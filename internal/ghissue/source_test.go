package ghissue

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func wireIssue(mutate ...func(map[string]any)) map[string]any {
	item := map[string]any{
		"number":     float64(7),
		"title":      "App crashes on save",
		"body":       "steps",
		"state":      "open",
		"user":       map[string]any{"login": "Maint"},
		"assignees":  []any{},
		"labels":     []any{},
		"updated_at": "2026-09-24T10:00:00+00:00",
		"html_url":   "https://github.com/o/r/issues/7",
	}
	for _, apply := range mutate {
		apply(item)
	}
	return item
}

func TestParseIssueNormalizesWireFields(t *testing.T) {
	issue, err := ParseIssue(wireIssue(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Repository != "o/r" || issue.Number != 7 {
		t.Fatalf("identity = %+v", issue)
	}
	if issue.RevisionID() != "2026-09-24T10:00:00Z" {
		t.Fatalf("revision = %q, want Z form", issue.RevisionID())
	}
	if issue.ObjectID() != "github:o/r#7" {
		t.Fatalf("objectID = %q", issue.ObjectID())
	}
	if issue.Assignees == nil || issue.Labels == nil {
		t.Fatalf("lists not normalized: %+v", issue)
	}
	if issue.Triage != TriageUnclassified {
		t.Fatalf("triage = %q", issue.Triage)
	}
	if issue.IsPullRequest {
		t.Fatal("plain issue flagged as pull request")
	}
}

func TestParseIssueDefaultsOptionalFields(t *testing.T) {
	issue, err := ParseIssue(wireIssue(func(m map[string]any) {
		delete(m, "body")
		delete(m, "assignees")
		delete(m, "labels")
	}), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Body != "" || issue.Assignees == nil || issue.Labels == nil {
		t.Fatalf("defaults = %q %v %v", issue.Body, issue.Assignees, issue.Labels)
	}
}

func TestParseIssueRejectsMissingRequiredFields(t *testing.T) {
	cases := map[string]func(map[string]any){
		"number":            func(m map[string]any) { delete(m, "number") },
		"title":             func(m map[string]any) { delete(m, "title") },
		"state":             func(m map[string]any) { delete(m, "state") },
		"user":              func(m map[string]any) { delete(m, "user") },
		"user.login":        func(m map[string]any) { m["user"] = map[string]any{} },
		"updated_at":        func(m map[string]any) { delete(m, "updated_at") },
		"html_url":          func(m map[string]any) { delete(m, "html_url") },
		"bad number":        func(m map[string]any) { m["number"] = "seven" },
		"zero number":       func(m map[string]any) { m["number"] = float64(0) },
		"negative number":   func(m map[string]any) { m["number"] = float64(-3) },
		"fractional number": func(m map[string]any) { m["number"] = 1.5 },
		"blank title":       func(m map[string]any) { m["title"] = "   " },
		"blank state":       func(m map[string]any) { m["state"] = "" },
		"blank url":         func(m map[string]any) { m["html_url"] = " " },
		"blank login":       func(m map[string]any) { m["user"] = map[string]any{"login": " "} },
		"bad update":        func(m map[string]any) { m["updated_at"] = "yesterday" },
		"bad author":        func(m map[string]any) { m["user"] = "maint" },
		"bad labels":        func(m map[string]any) { m["labels"] = "bug" },
		"bad label entry":   func(m map[string]any) { m["labels"] = []any{map[string]any{"name": 7}} },
		"bad assignees":     func(m map[string]any) { m["assignees"] = "maint" },
		"bad assignee entry": func(m map[string]any) {
			m["assignees"] = []any{"maint"}
		},
		"bad body": func(m map[string]any) { m["body"] = 7 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseIssue(wireIssue(mutate), "o/r"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseIssueFlagsPullRequests(t *testing.T) {
	issue, err := ParseIssue(wireIssue(func(m map[string]any) {
		m["pull_request"] = map[string]any{"url": "https://api.github.com/repos/o/r/pulls/7"}
	}), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if !issue.IsPullRequest {
		t.Fatal("pull request flag not set")
	}
}

func TestNormalizeTokensDeduplicatesAndSortsCaseInsensitively(t *testing.T) {
	got := normalizeTokens([]string{"Beta", "beta", " alpha ", ""})
	if !reflect.DeepEqual(got, []string{"alpha", "Beta"}) {
		t.Fatalf("names = %v", got)
	}
}

func TestClassifyTriage(t *testing.T) {
	cases := []struct {
		labels []string
		title  string
		want   string
	}{
		{[]string{"Bug"}, "x", TriageBug},
		{[]string{" DEFECT "}, "x", TriageBug},
		{nil, "[bug] crash", TriageBug},
		{[]string{"enhancement"}, "x", TriageFeature},
		{[]string{"feature"}, "x", TriageFeature},
		{nil, "[Feat] add", TriageFeature},
		{nil, "[feature] add", TriageFeature},
		{[]string{"enhancement", "bug"}, "x", TriageBug},
		{[]string{"docs"}, "polish", TriageUnclassified},
	}
	for _, tc := range cases {
		if got := ClassifyTriage(tc.labels, tc.title); got != tc.want {
			t.Fatalf("labels=%v title=%q got %q want %q", tc.labels, tc.title, got, tc.want)
		}
	}
}

func TestFilterIssuesReasonPrecedence(t *testing.T) {
	issue := func(mutate ...func(map[string]any)) Issue {
		parsed, err := ParseIssue(wireIssue(mutate...), "o/r")
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	pullRequest := issue(func(m map[string]any) { m["pull_request"] = map[string]any{"url": "x"} })
	if kept, excluded, skipped := FilterIssues([]Issue{pullRequest}, []string{"maint"}); len(kept) != 0 || len(excluded) != 0 || skipped != 1 {
		t.Fatalf("pull request: kept=%d excluded=%d skipped=%d", len(kept), len(excluded), skipped)
	}
	cases := []struct {
		issue  Issue
		reason string
	}{
		{issue(func(m map[string]any) { m["state"] = "closed" }), ReasonNotOpen},
		{issue(func(m map[string]any) { m["user"] = map[string]any{"login": "stranger"} }), ReasonNotMaintainer},
		{issue(func(m map[string]any) { m["assignees"] = []any{map[string]any{"login": "x"}} }), ReasonAlreadyAssigned},
		{issue(func(m map[string]any) { m["labels"] = []any{map[string]any{"name": "WontFix"}} }), ReasonHeldByLabel},
		{issue(func(m map[string]any) { m["labels"] = []any{map[string]any{"name": "box-hold"}} }), ReasonHeldByLabel},
	}
	for _, tc := range cases {
		kept, excluded, skipped := FilterIssues([]Issue{tc.issue}, []string{"MAINT"})
		if len(kept) != 0 || skipped != 0 || len(excluded) != 1 || excluded[0].Reason != tc.reason {
			t.Fatalf("issue %d: kept=%d excluded=%+v want %q", tc.issue.Number, len(kept), excluded, tc.reason)
		}
	}
	closedStranger := issue(func(m map[string]any) {
		m["state"] = "closed"
		m["user"] = map[string]any{"login": "stranger"}
	})
	if _, excluded, _ := FilterIssues([]Issue{closedStranger}, []string{"maint"}); len(excluded) != 1 || excluded[0].Reason != ReasonNotOpen {
		t.Fatalf("precedence = %+v, want not-open first", excluded)
	}
	kept, excluded, _ := FilterIssues([]Issue{issue()}, []string{"maint"})
	if len(kept) != 1 || len(excluded) != 0 {
		t.Fatalf("candidate: kept=%+v excluded=%+v", kept, excluded)
	}
}

func TestCanonicalSnapshotKeys(t *testing.T) {
	issue, err := ParseIssue(wireIssue(func(m map[string]any) {
		m["labels"] = []any{
			map[string]any{"name": "Bug"},
			map[string]any{"name": "bug"},
			map[string]any{"name": "Alpha"},
		}
	}), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := CanonicalSnapshot(issue)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(snapshot, &decoded); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	sortStrings(keys)
	want := []string{"assignees", "author", "body", "issue", "labels", "repo", "title", "triage", "updatedAt", "url"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	if decoded["updatedAt"] != "2026-09-24T10:00:00Z" {
		t.Fatalf("updatedAt = %v", decoded["updatedAt"])
	}
	if !reflect.DeepEqual(decoded["labels"], []any{"Alpha", "Bug"}) {
		t.Fatalf("labels = %v", decoded["labels"])
	}
	if !reflect.DeepEqual(decoded["assignees"], []any{}) {
		t.Fatalf("assignees = %v", decoded["assignees"])
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

type stubPage struct {
	items []any
	bad   []Unparseable
	next  string
	err   error
}

type stubLister struct {
	name  string
	pages []stubPage
	calls []string
}

func (l *stubLister) ListIssues(_ context.Context, cursor string) ([]any, []Unparseable, string, error) {
	l.calls = append(l.calls, cursor)
	if len(l.pages) == 0 {
		return nil, nil, "", errors.New("unexpected page request")
	}
	page := l.pages[0]
	l.pages = l.pages[1:]
	return page.items, page.bad, page.next, page.err
}

func (l *stubLister) Name() string { return l.name }

func TestCollectIssuesWalksPagesAndKeepsUnparseable(t *testing.T) {
	lister := &stubLister{name: "o/r", pages: []stubPage{
		{
			items: []any{wireIssue()},
			bad:   []Unparseable{{Raw: map[string]any{"number": float64(9)}}},
			next:  "cursor-2",
		},
		{items: []any{wireIssue(func(m map[string]any) { m["number"] = float64(8) })}},
	}}
	issues, bad, err := CollectIssues(context.Background(), lister)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || len(bad) != 1 || bad[0].Raw == nil {
		t.Fatalf("issues=%d bad=%d, want 2 and 1", len(issues), len(bad))
	}
	if issues[0].ObjectID() != "github:o/r#7" || issues[1].ObjectID() != "github:o/r#8" {
		t.Fatalf("identity = %q %q", issues[0].ObjectID(), issues[1].ObjectID())
	}
	if !reflect.DeepEqual(lister.calls, []string{"", "cursor-2"}) {
		t.Fatalf("calls = %v", lister.calls)
	}
}

func TestCollectIssuesRejectsRepeatedCursor(t *testing.T) {
	lister := &stubLister{name: "o/r", pages: []stubPage{
		{items: []any{wireIssue()}, next: "same"},
		{items: []any{wireIssue()}, next: "same"},
	}}
	if _, _, err := CollectIssues(context.Background(), lister); err == nil {
		t.Fatal("expected repeated cursor error")
	}
}

func TestCollectIssuesPropagatesListerError(t *testing.T) {
	sentinel := errors.New("boom")
	lister := &stubLister{name: "o/r", pages: []stubPage{{err: sentinel}}}
	if _, _, err := CollectIssues(context.Background(), lister); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}

func TestCollectIssuesRequiresRepositoryName(t *testing.T) {
	lister := &stubLister{name: "not-a-repo"}
	if _, _, err := CollectIssues(context.Background(), lister); err == nil {
		t.Fatal("expected repository name error")
	}
}
