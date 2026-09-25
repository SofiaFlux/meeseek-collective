# GitHub Issue Intake Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register maintainer-opened GitHub issues as durable workflow cases with triage Tasks, performing zero GitHub writes.

**Architecture:** New `internal/ghissue` package mirrors `internal/adoreview` (source → client → observe → CLI). The client is GET-only by construction. Case+Task creation becomes atomic via `workflowcase.EnsureAndMaterialize`, built on a new transaction-aware `execution.CreateTaskWithGuardInTx`; the ADO observer's miss path migrates to the same primitive. The `run-gh-intake` CLI opens the Box through `openGHIntakeBox`, which strips every writable component, so triage Tasks stay eligible-but-unclaimed.

**Tech Stack:** Go 1.27, `net/http` + `httptest` for transport tests, `testutil` store/clock for service tests, stdlib `flag` for CLI.

## Global Constraints

- No GitHub writes: the client exposes exactly `ListIssues` and `Name`; no POST/PATCH/DELETE method may exist on the type.
- Capability `github.issue.read` is always in FirstWork RequiredCapabilities; every effective work capability must be grant-listed (startup error otherwise).
- `IssueLister` returns parsed and unparseable items separately; unparseable items become exclusions, never silent drops.
- SQLite runs one connection (`db.SetMaxOpenConns(1)`): composing canonical writes requires ONE `store.WithTx` and the new `*InTx` primitive — never a nested `WithTx`.
- TDD: failing test first for every behavior, then minimal implementation.
- `GOCACHE=/tmp/summa42-full-go-cache` for all `go test`/`go vet`/`go build` invocations in this plan.
- Validation matrix from `CONTRIBUTING.md` before the final commit (Task 5).

---

### Task 1: Issue parsing, filter, snapshot, collector

**Files:**
- Create: `internal/ghissue/source.go`
- Create: `internal/ghissue/source_test.go`
- Create: `internal/ghissue/results.go` (Task 1; holds `ExcludedIssue` so `FilterIssues` compiles now — Task 4 adds `FailedIssue`/`ObserveResult` in observe.go).

**Interfaces:**
- Consumes: raw GitHub issue JSON objects (`[]any` per page) as decoded by `encoding/json`.
- Produces: `repositoryPattern`, `Issue` (+`ObjectID`/`RevisionID`), `Unparseable`, `IssueLister`, `classifyWireItem`, `ParseIssue`, `ClassifyTriage`, `FilterIssues`, `CanonicalSnapshot`, `CollectIssues`, reason/triage constants — all consumed by Tasks 2, 4, 5.
- `ExcludedIssue` is defined once in results.go (Task 1) and shared with observe.go (Task 4).

- [ ] **Step 1: Write the failing test** — create `internal/ghissue/source_test.go`:

```go
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
		"number":     func(m map[string]any) { delete(m, "number") },
		"title":      func(m map[string]any) { delete(m, "title") },
		"state":      func(m map[string]any) { delete(m, "state") },
		"user":       func(m map[string]any) { delete(m, "user") },
		"user.login": func(m map[string]any) { m["user"] = map[string]any{} },
		"updated_at": func(m map[string]any) { delete(m, "updated_at") },
		"html_url":   func(m map[string]any) { delete(m, "html_url") },
		"bad number":       func(m map[string]any) { m["number"] = "seven" },
		"zero number":      func(m map[string]any) { m["number"] = float64(0) },
		"negative number":  func(m map[string]any) { m["number"] = float64(-3) },
		"fractional number": func(m map[string]any) { m["number"] = 1.5 },
		"blank title":      func(m map[string]any) { m["title"] = "   " },
		"blank state":      func(m map[string]any) { m["state"] = "" },
		"blank url":        func(m map[string]any) { m["html_url"] = " " },
		"blank login":      func(m map[string]any) { m["user"] = map[string]any{"login": " "} },
		"bad update":       func(m map[string]any) { m["updated_at"] = "yesterday" },
		"bad author":       func(m map[string]any) { m["user"] = "maint" },
		"bad labels":       func(m map[string]any) { m["labels"] = "bug" },
		"bad label entry":  func(m map[string]any) { m["labels"] = []any{map[string]any{"name": 7}} },
		"bad assignees":    func(m map[string]any) { m["assignees"] = "maint" },
		"bad assignee entry": func(m map[string]any) { m["assignees"] = []any{"maint"} },
		"bad body":         func(m map[string]any) { m["body"] = 7 },
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -count=1`
Expected: FAIL — `undefined: ParseIssue`, `undefined: FilterIssues`, `undefined: CollectIssues`, `undefined: wireIssue` companions, etc.

- [ ] **Step 3: Write minimal implementation** — create `internal/ghissue/source.go`:

```go
// Package ghissue registers maintainer-opened GitHub issues as workflow cases.
// The package performs no GitHub writes: its client exposes GET operations only.
package ghissue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ReasonUnparseable     = "unparseable-issue"
	ReasonNotOpen         = "not-open"
	ReasonNotMaintainer   = "not-maintainer"
	ReasonAlreadyAssigned = "already-assigned"
	ReasonHeldByLabel     = "held-by-label"
	ReasonCaseNotActive   = "case-not-active"
	ReasonEnsureFailed    = "ensure-failed"

	TriageBug          = "bug"
	TriageFeature      = "feature"
	TriageUnclassified = "unclassified"
)
```

In the same step create `internal/ghissue/results.go` — `ExcludedIssue` is the shared result type used by both `FilterIssues` (this task) and `ObserveOnce` (Task 4), so it must exist for Task 1 to compile:

```go
package ghissue

// ExcludedIssue records one issue that did not become a case, with its reason
// token. It is defined once here and reused by observe.go.
type ExcludedIssue struct {
	Issue  Issue
	Reason string
}
```

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Issue is one parsed GitHub issue revision.
type Issue struct {
	Repository    string
	Number        int64
	Title         string
	Body          string
	State         string
	Author        string
	Assignees     []string
	Labels        []string
	URL           string
	UpdatedAt     time.Time
	Triage        string
	IsPullRequest bool
}

// ObjectID qualifies the issue with its repository: plain issue#n collides
// across repositories under the unique (mission, source, object, revision) key.
func (i Issue) ObjectID() string {
	return "github:" + i.Repository + "#" + strconv.FormatInt(i.Number, 10)
}

// RevisionID is the canonical UTC revision string.
func (i Issue) RevisionID() string {
	return i.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

// Unparseable is one wire item missing required fields.
type Unparseable struct {
	Raw any
}

// IssueLister is the read-only source consumed by the observer.
type IssueLister interface {
	ListIssues(ctx context.Context, cursor string) ([]any, []Unparseable, string, error)
	Name() string
}

func classifyWireItem(item any) error {
	object, ok := item.(map[string]any)
	if !ok {
		return errors.New("issue item is not an object")
	}
	if _, err := numberOf(object); err != nil {
		return err
	}
	for _, key := range []string{"title", "state", "updated_at", "html_url"} {
		if _, err := requiredString(object, key); err != nil {
			return err
		}
	}
	author, ok := object["user"].(map[string]any)
	if !ok {
		return errors.New("issue item has no user object")
	}
	if _, err := requiredString(author, "login"); err != nil {
		return err
	}
	if raw, present := object["body"]; present && raw != nil {
		if _, err := stringField(object, "body"); err != nil {
			return err
		}
	}
	for _, list := range []struct{ key, field string }{{"assignees", "login"}, {"labels", "name"}} {
		if _, err := nameList(object[list.key], list.field); err != nil {
			return err
		}
	}
	updated, err := requiredString(object, "updated_at")
	if err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, updated); err != nil {
		return fmt.Errorf("parse issue updated_at: %w", err)
	}
	return nil
}

func ParseIssue(item any, repository string) (Issue, error) {
	if err := classifyWireItem(item); err != nil {
		return Issue{}, err
	}
	object := item.(map[string]any)
	number, _ := numberOf(object)
	title, _ := requiredString(object, "title")
	state, _ := requiredString(object, "state")
	url, _ := requiredString(object, "html_url")
	updated, _ := requiredString(object, "updated_at")
	author, _ := requiredString(object["user"].(map[string]any), "login")
	body := ""
	if raw, ok := object["body"]; ok && raw != nil {
		value, err := stringField(object, "body")
		if err != nil {
			return Issue{}, err
		}
		body = value
	}
	assignees, err := nameList(object["assignees"], "login")
	if err != nil {
		return Issue{}, err
	}
	labels, err := nameList(object["labels"], "name")
	if err != nil {
		return Issue{}, err
	}
	updatedAt, err := time.Parse(time.RFC3339, updated)
	if err != nil {
		return Issue{}, fmt.Errorf("parse issue updated_at: %w", err)
	}
	issue := Issue{
		Repository: repository,
		Number:     number,
		Title:      title,
		Body:       body,
		State:      state,
		Author:     author,
		Assignees:  assignees,
		Labels:     labels,
		URL:        url,
		UpdatedAt:  updatedAt.UTC(),
	}
	issue.Triage = ClassifyTriage(labels, title)
	_, issue.IsPullRequest = object["pull_request"]
	return issue, nil
}

func ClassifyTriage(labels []string, title string) string {
	prefix := strings.ToLower(strings.TrimSpace(title))
	switch {
	case hasLabel(labels, "bug", "defect") || strings.HasPrefix(prefix, "[bug]"):
		return TriageBug
	case hasLabel(labels, "enhancement", "feature") ||
		strings.HasPrefix(prefix, "[feat]") || strings.HasPrefix(prefix, "[feature]"):
		return TriageFeature
	default:
		return TriageUnclassified
	}
}

func FilterIssues(issues []Issue, maintainers []string) ([]Issue, []ExcludedIssue, int) {
	allowed := make(map[string]bool, len(maintainers))
	for _, login := range maintainers {
		if trimmed := strings.ToLower(strings.TrimSpace(login)); trimmed != "" {
			allowed[trimmed] = true
		}
	}
	var kept []Issue
	var excluded []ExcludedIssue
	skipped := 0
	for _, issue := range issues {
		switch {
		case issue.IsPullRequest:
			skipped++
		case !strings.EqualFold(strings.TrimSpace(issue.State), "open"):
			excluded = append(excluded, ExcludedIssue{Issue: issue, Reason: ReasonNotOpen})
		case !allowed[strings.ToLower(strings.TrimSpace(issue.Author))]:
			excluded = append(excluded, ExcludedIssue{Issue: issue, Reason: ReasonNotMaintainer})
		case len(issue.Assignees) > 0:
			excluded = append(excluded, ExcludedIssue{Issue: issue, Reason: ReasonAlreadyAssigned})
		case hasLabel(issue.Labels, "box-hold", "wontfix"):
			excluded = append(excluded, ExcludedIssue{Issue: issue, Reason: ReasonHeldByLabel})
		default:
			kept = append(kept, issue)
		}
	}
	return kept, excluded, skipped
}

func CollectIssues(ctx context.Context, lister IssueLister) ([]Issue, []Unparseable, error) {
	if ctx == nil {
		return nil, nil, errors.New("collector context is required")
	}
	if lister == nil {
		return nil, nil, errors.New("issue lister is required")
	}
	repository := strings.TrimSpace(lister.Name())
	if !repositoryPattern.MatchString(repository) {
		return nil, nil, errors.New("issue lister name must be owner/name")
	}
	var (
		issues      []Issue
		unparseable []Unparseable
		seen        = map[string]struct{}{}
		cursor      string
	)
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		items, bad, next, err := lister.ListIssues(ctx, cursor)
		if err != nil {
			return nil, nil, err
		}
		unparseable = append(unparseable, bad...)
		for _, item := range items {
			issue, err := ParseIssue(item, repository)
			if err != nil {
				return nil, nil, fmt.Errorf("parse issue item: %w", err)
			}
			issues = append(issues, issue)
		}
		if next == "" {
			return issues, unparseable, nil
		}
		if _, repeated := seen[next]; repeated {
			return nil, nil, errors.New("GitHub issues pagination repeated a next cursor")
		}
		seen[next] = struct{}{}
		cursor = next
	}
}

func CanonicalSnapshot(issue Issue) ([]byte, error) {
	return json.Marshal(struct {
		Repo      string   `json:"repo"`
		Issue     int64    `json:"issue"`
		Title     string   `json:"title"`
		Body      string   `json:"body"`
		Author    string   `json:"author"`
		Assignees []string `json:"assignees"`
		Labels    []string `json:"labels"`
		URL       string   `json:"url"`
		UpdatedAt string   `json:"updatedAt"`
		Triage    string   `json:"triage"`
	}{
		Repo:      issue.Repository,
		Issue:     issue.Number,
		Title:     issue.Title,
		Body:      issue.Body,
		Author:    issue.Author,
		Assignees: nonNil(issue.Assignees),
		Labels:    nonNil(issue.Labels),
		URL:       issue.URL,
		UpdatedAt: issue.RevisionID(),
		Triage:    issue.Triage,
	})
}

func numberOf(object map[string]any) (int64, error) {
	var number int64
	switch value := object["number"].(type) {
	case float64:
		number = int64(value)
		if float64(number) != value {
			return 0, errors.New("issue number is not an integer")
		}
	case int64:
		number = value
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, errors.New("issue number is not an integer")
		}
		number = parsed
	default:
		return 0, errors.New("issue number is not numeric")
	}
	if number <= 0 {
		return 0, errors.New("issue number is not positive")
	}
	return number, nil
}

// requiredString reads a field that must be present and non-blank.
func requiredString(object map[string]any, key string) (string, error) {
	value, ok := object[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("issue %q is not a non-blank string", key)
	}
	return value, nil
}

// stringField reads a field that must be a string but may be blank; blank
// values are dropped by normalizeTokens.
func stringField(object map[string]any, key string) (string, error) {
	value, ok := object[key].(string)
	if !ok {
		return "", fmt.Errorf("issue %q is not a string", key)
	}
	return value, nil
}

func nameList(value any, key string) ([]string, error) {
	if value == nil {
		return []string{}, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("issue %s list is not an array", key)
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("issue %s list entry is not an object", key)
		}
		name, err := stringField(object, key)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return normalizeTokens(names), nil
}

func normalizeTokens(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func hasLabel(labels []string, wanted ...string) bool {
	for _, label := range labels {
		normalized := strings.ToLower(strings.TrimSpace(label))
		for _, want := range wanted {
			if normalized == want {
				return true
			}
		}
	}
	return false
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `gofmt -w internal/ghissue` then `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -count=1`
Expected: PASS. `internal/ghissue/results.go` (created in Step 1) already defines `ExcludedIssue`, so no compile shim and no cross-task dependency is needed.

- [ ] **Step 5: Commit**

```bash
git add internal/ghissue/source.go internal/ghissue/source_test.go internal/ghissue/results.go
git commit -m "feat: parse and filter GitHub issue candidates"
```

### Task 2: Read-only GitHub client

**Files:**
- Create: `internal/ghissue/client.go`
- Create: `internal/ghissue/client_test.go`

**Interfaces:**
- Consumes: `repositoryPattern`, `classifyWireItem`, `Unparseable` from Task 1; token file at `SUMMA42_GITHUB_TOKEN_FILE` (read by the CLI, passed as `Config.TokenFile`).
- Produces: `Config`, `New`, `(*Client).ListIssues`, `(*Client).Name` — the only exported client surface; `*Client` satisfies `IssueLister` (asserted in tests).

- [ ] **Step 1: Write the failing test** — create `internal/ghissue/client_test.go`:

```go
package ghissue

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func tokenFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validIssueJSON = `{"number":7,"title":"t","body":"","state":"open","user":{"login":"maint"},"assignees":[],"labels":[],"updated_at":"2026-09-24T10:00:00Z","html_url":"https://github.com/o/r/issues/7"}`

func TestListIssuesRequestsOpenIssuesPageWithBearerToken(t *testing.T) {
	var seen *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		_, _ = w.Write([]byte("[" + validIssueJSON + "]"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit\n")})
	if err != nil {
		t.Fatal(err)
	}
	items, bad, next, err := client.ListIssues(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(bad) != 0 || next != "" {
		t.Fatalf("items=%d bad=%d next=%q", len(items), len(bad), next)
	}
	if seen.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", seen.Method)
	}
	if seen.URL.Path != "/repos/o/r/issues" {
		t.Fatalf("path = %q", seen.URL.Path)
	}
	if seen.URL.Query().Get("state") != "open" || seen.URL.Query().Get("per_page") != "100" {
		t.Fatalf("query = %q", seen.URL.RawQuery)
	}
	if seen.Header.Get("Authorization") != "Bearer sekrit" {
		t.Fatalf("authorization = %q", seen.Header.Get("Authorization"))
	}
	if seen.Header.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("accept = %q", seen.Header.Get("Accept"))
	}
	if client.Name() != "o/r" {
		t.Fatalf("name = %q", client.Name())
	}
	var lister IssueLister = client
	if lister.Name() != "o/r" {
		t.Fatal("client does not satisfy IssueLister")
	}
}

func TestListIssuesFollowsLinkPagination(t *testing.T) {
	var paths []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		w.Header().Set("Link", `<`+server.URL+`/repos/o/r/issues?state=open&per_page=100&page=2>; rel="next"`)
		_, _ = w.Write([]byte("[" + validIssueJSON + "]"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, next, err := client.ListIssues(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "page=2") {
		t.Fatalf("next = %q, want page-2 cursor", next)
	}
	if _, _, last, err := client.ListIssues(context.Background(), next); err != nil || last != "" {
		t.Fatalf("last=%q err=%v", last, err)
	}
	if len(paths) != 2 || !strings.Contains(paths[1], "page=2") {
		t.Fatalf("paths = %v", paths)
	}
}

func TestListIssuesSplitsUnparseableItems(t *testing.T) {
	malformedLabels := `{"number":8,"title":"t","state":"open","user":{"login":"maint"},` +
		`"updated_at":"2026-09-24T10:00:00Z","html_url":"https://github.com/o/r/issues/8","labels":"bug"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[" + validIssueJSON + `,{"number":8},` + malformedLabels + `]`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	items, bad, _, err := client.ListIssues(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(bad) != 2 {
		t.Fatalf("items=%d bad=%d", len(items), len(bad))
	}
	for _, entry := range bad {
		if entry.Raw == nil {
			t.Fatal("unparseable entry lost its raw item")
		}
	}
}

func TestListIssuesRejectsNonArrayBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("null"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := client.ListIssues(context.Background(), ""); err == nil {
		t.Fatal("expected error for null body")
	}
}

func TestListIssuesRejectsRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/repos/o/r/issues", http.StatusFound)
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := client.ListIssues(context.Background(), ""); err == nil {
		t.Fatal("expected error for redirect response")
	}
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	token := tokenFile(t, "sekrit")
	cases := []Config{
		{BaseURL: "https://api.github.com", Repository: "o", TokenFile: token},
		{BaseURL: "https://api.github.com", Repository: "../x", TokenFile: token},
		{BaseURL: "https://api.github.com", Repository: "o/r/extra", TokenFile: token},
		{BaseURL: "https://user:pass@api.github.com", Repository: "o/r", TokenFile: token},
		{BaseURL: "http://api.github.com", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://api.github.com", Repository: "o/r", TokenFile: " "},
		{BaseURL: "https://evil.example", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://api.github.com/v3", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://api.github.com?x=1", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://api.github.com#frag", Repository: "o/r", TokenFile: token},
		{BaseURL: "https://ghe.example", Repository: "o/r", TokenFile: token},
	}
	for i, cfg := range cases {
		if _, err := New(cfg); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestListIssuesRejectsUnsafeTokenFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	loose := filepath.Join(t.TempDir(), "loose")
	if err := os.WriteFile(loose, []byte("sekrit"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"empty":              tokenFile(t, "  \n"),
		"loose permissions":  loose,
		"missing":            filepath.Join(t.TempDir(), "absent"),
		"directory":          t.TempDir(),
	}
	for name, path := range paths {
		client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: path})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := client.ListIssues(context.Background(), ""); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestListIssuesSanitizesHTTPFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError, http.StatusBadGateway} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"token sekrit rejected"}`))
		}))
		client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, err = client.ListIssues(context.Background(), "")
		server.Close()
		if err == nil {
			t.Fatalf("status %d: expected error", status)
		}
		if !strings.Contains(err.Error(), strconv.Itoa(status)) {
			t.Fatalf("status %d: err = %v, want status in message", status, err)
		}
		if strings.Contains(err.Error(), "sekrit") {
			t.Fatalf("status %d leaked token: %v", status, err)
		}
	}
}

func TestListIssuesEnforcesResponseCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit"), MaxResponseBytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := client.ListIssues(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "128") {
		t.Fatalf("err = %v, want response cap error", err)
	}
}

func TestListIssuesRejectsUnsafeLinkHeaders(t *testing.T) {
	links := map[string]func(origin string) string{
		"malformed":        func(string) string { return `<>; rel="next"` },
		"cross origin":     func(string) string { return `<http://other.example/repos/o/r/issues?page=2>; rel="next"` },
		"unquoted rel":     func(origin string) string { return `<` + origin + `/repos/o/r/issues?page=2>; rel=next` },
		"empty section":    func(string) string { return `,` },
		"relative target":  func(string) string { return `</repos/o/r/issues?page=2>; rel="next"` },
		"userinfo target":  func(origin string) string { return `<http://user:pass@` + strings.TrimPrefix(origin, "http://") + `/repos/o/r/issues?page=2>; rel="next"` },
		"fragment target":  func(origin string) string { return `<` + origin + `/repos/o/r/issues?page=2#frag>; rel="next"` },
		"foreign resource": func(origin string) string { return `<` + origin + `/repos/o/other/issues?page=2>; rel="next"` },
	}
	for name, build := range links {
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Link", build(server.URL))
			_, _ = w.Write([]byte("[]"))
		}))
		client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, err = client.ListIssues(context.Background(), "")
		server.Close()
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestListIssuesFollowsQuotedNextLinkAndIgnoresPrev(t *testing.T) {
	var nextCalls int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "page=2") {
			nextCalls++
			_, _ = w.Write([]byte("[" + validIssueJSON + "]"))
			return
		}
		w.Header().Set("Link", `<`+server.URL+`/repos/o/r/issues?page=2>; rel="next", <`+server.URL+`/repos/o/r/issues?page=9>; rel="prev"`)
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	_, _, cursor, err := client.ListIssues(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cursor, "page=2") {
		t.Fatalf("cursor = %q, want page 2", cursor)
	}
	if _, _, _, err := client.ListIssues(context.Background(), cursor); err != nil {
		t.Fatal(err)
	}
	if nextCalls != 1 {
		t.Fatalf("next page fetches = %d, want 1", nextCalls)
	}
}

func TestListIssuesPropagatesContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit")})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := client.ListIssues(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestListIssuesHonorsClientTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer func() {
		close(release)
		server.Close()
	}()
	client, err := New(Config{BaseURL: server.URL, Repository: "o/r", TokenFile: tokenFile(t, "sekrit"), Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := client.ListIssues(context.Background(), ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestClientExposesOnlyReadOperations(t *testing.T) {
	var methods []string
	for _, name := range []string{"POST", "PATCH", "PUT", "DELETE", "Create", "Update", "Delete", "Comment", "Merge", "ListIssues", "Name"} {
		if _, ok := reflect.TypeOf(&Client{}).MethodByName(name); ok {
			methods = append(methods, name)
		}
	}
	if !reflect.DeepEqual(methods, []string{"ListIssues", "Name"}) {
		t.Fatalf("methods = %v, want only ListIssues and Name", methods)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -count=1`
Expected: FAIL — `undefined: New`, `undefined: Config`, `undefined: Client`.

- [ ] **Step 3: Write minimal implementation** — create `internal/ghissue/client.go`:

```go
package ghissue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	defaultAPIBaseURL = "https://api.github.com"
	defaultAPIHost    = "api.github.com"
	defaultTimeout    = 20 * time.Second
	defaultMaxBytes   = 1 << 20
	userAgent         = "summa42-ghissue/1"
)

// rejectRedirect keeps bearer credentials pinned to the configured origin:
// any 3xx is surfaced as a non-200 response instead of being followed.
func rejectRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// Config configures the read-only client. Zero Timeout or MaxResponseBytes
// falls back to package defaults.
type Config struct {
	BaseURL          string
	Repository       string
	TokenFile        string
	Timeout          time.Duration
	MaxResponseBytes int
}

// Client performs GET requests only; no write method exists on the type.
type Client struct {
	baseURL          *url.URL
	repository       string
	tokenFile        string
	http             *http.Client
	maxResponseBytes int
}

func New(cfg Config) (*Client, error) {
	repository := strings.TrimSpace(cfg.Repository)
	if !repositoryPattern.MatchString(repository) {
		return nil, errors.New("GitHub issues repository must be owner/name")
	}
	for _, part := range strings.Split(repository, "/") {
		if part == "." || part == ".." {
			return nil, errors.New("GitHub issues repository must be owner/name")
		}
	}
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = defaultAPIBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("GitHub API base URL must be an absolute HTTP(S) URL without user info")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, errors.New("GitHub API base URL must not carry a path, query, or fragment")
	}
	parsed.Path = ""
	host := strings.TrimSpace(parsed.Hostname())
	loopback := strings.EqualFold(host, "localhost") || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
	if parsed.Scheme != "https" {
		if parsed.Scheme != "http" || !loopback {
			return nil, errors.New("GitHub API base URL must use HTTPS except for loopback test endpoints")
		}
	} else if !loopback && !strings.EqualFold(host, defaultAPIHost) {
		return nil, errors.New("GitHub API base URL host must be api.github.com")
	}
	tokenFile := strings.TrimSpace(cfg.TokenFile)
	if tokenFile == "" {
		return nil, errors.New("GitHub issues token file path is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	limit := cfg.MaxResponseBytes
	if limit <= 0 {
		limit = defaultMaxBytes
	}
	return &Client{
		baseURL:          parsed,
		repository:       repository,
		tokenFile:        tokenFile,
		http:             &http.Client{Timeout: timeout, CheckRedirect: rejectRedirect},
		maxResponseBytes: limit,
	}, nil
}

// Name returns the configured repository, used to qualify object IDs.
func (c *Client) Name() string {
	return c.repository
}

func (c *Client) ListIssues(ctx context.Context, cursor string) ([]any, []Unparseable, string, error) {
	if ctx == nil {
		return nil, nil, "", errors.New("GitHub issues context is required")
	}
	target, err := c.pageURL(cursor)
	if err != nil {
		return nil, nil, "", err
	}
	token, err := readTokenFile(c.tokenFile)
	if err != nil {
		return nil, nil, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, "", fmt.Errorf("build GitHub issues request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", userAgent)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, nil, "", fmt.Errorf("list GitHub issues: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, nil, "", fmt.Errorf("list GitHub issues: unexpected status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(c.maxResponseBytes)+1))
	if err != nil {
		return nil, nil, "", fmt.Errorf("read GitHub issues response: %w", err)
	}
	if len(body) > c.maxResponseBytes {
		return nil, nil, "", fmt.Errorf("GitHub issues response exceeds %d bytes", c.maxResponseBytes)
	}
	var raw []any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, "", fmt.Errorf("decode GitHub issues response: %w", err)
	}
	if raw == nil {
		return nil, nil, "", errors.New("decode GitHub issues response: expected a JSON array")
	}
	next, err := c.nextCursor(response.Header.Get("Link"))
	if err != nil {
		return nil, nil, "", err
	}
	items := make([]any, 0, len(raw))
	var unparseable []Unparseable
	for _, item := range raw {
		if err := classifyWireItem(item); err != nil {
			unparseable = append(unparseable, Unparseable{Raw: item})
			continue
		}
		items = append(items, item)
	}
	return items, unparseable, next, nil
}

func (c *Client) pageURL(cursor string) (string, error) {
	if strings.TrimSpace(cursor) == "" {
		return c.baseURL.String() + "/repos/" + c.repository + "/issues?state=open&per_page=100", nil
	}
	parsed, err := url.Parse(cursor)
	if err != nil || !parsed.IsAbs() {
		return "", errors.New("GitHub issues pagination cursor is not an absolute URL")
	}
	if parsed.Scheme != c.baseURL.Scheme || parsed.Host != c.baseURL.Host {
		return "", errors.New("GitHub issues pagination cursor is cross-origin")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("GitHub issues pagination cursor is malformed")
	}
	if parsed.Path != "/repos/"+c.repository+"/issues" {
		return "", errors.New("GitHub issues pagination cursor targets another resource")
	}
	return parsed.String(), nil
}

func (c *Client) nextCursor(link string) (string, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return "", nil
	}
	next := ""
	for _, section := range strings.Split(link, ",") {
		section = strings.TrimSpace(section)
		if section == "" {
			return "", errors.New("GitHub issues Link header has an empty section")
		}
		open := strings.Index(section, "<")
		closing := strings.Index(section, ">")
		if open != 0 || closing <= open+1 {
			return "", errors.New("GitHub issues Link header is malformed")
		}
		target := section[open+1 : closing]
		params := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(section[closing+1:]), ";"))
		isNext := false
		if params != "" {
			for _, param := range strings.Split(params, ";") {
				key, value, found := strings.Cut(strings.TrimSpace(param), "=")
				if !found || len(value) < 2 || !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) {
					return "", errors.New("GitHub issues Link header parameter is malformed")
				}
				if strings.TrimSpace(key) == "rel" && value == `"next"` {
					isNext = true
				}
			}
		}
		if !isNext {
			continue
		}
		parsed, err := url.Parse(target)
		if err != nil || !parsed.IsAbs() {
			return "", errors.New("GitHub issues next link is malformed")
		}
		if parsed.Scheme != c.baseURL.Scheme || parsed.Host != c.baseURL.Host {
			return "", errors.New("GitHub issues next link is cross-origin")
		}
		if parsed.User != nil || parsed.Fragment != "" {
			return "", errors.New("GitHub issues next link is malformed")
		}
		if parsed.Path != "/repos/"+c.repository+"/issues" {
			return "", errors.New("GitHub issues next link targets another resource")
		}
		if next != "" {
			return "", errors.New("GitHub issues Link header repeats rel=next")
		}
		next = parsed.String()
	}
	return next, nil
}

func readTokenFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat GitHub issues credential file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("GitHub issues credential path must be a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("GitHub issues credential file must not be group/world accessible")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read GitHub issues credential file: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errors.New("GitHub issues credential file is empty")
	}
	return token, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -count=1 -v`
Expected: PASS (all Task 1 + Task 2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/ghissue/client.go internal/ghissue/client_test.go
git commit -m "feat: add read-only GitHub issues client"
```

### Task 3: Atomic case + Task creation and ADO migration

**Files:**
- Modify: `internal/execution/service.go` (`insertTask` split; new `CreateTaskWithGuardInTx`)
- Create: `internal/execution/in_tx_test.go`
- Modify: `internal/workflowcase/service.go` (`Ensure` split into `prepareObservation` + `ensureTx`)
- Modify: `internal/workflowcase/materialize.go` (extract `templateForCase` + `activeWorkGuard`)
- Create: `internal/workflowcase/ensure_materialize.go`
- Create: `internal/workflowcase/ensure_materialize_test.go`
- Modify: `internal/adoreview/observe.go` (miss path uses `EnsureAndMaterialize`)
- Create: `internal/adoreview/atomic_miss_test.go`

**Interfaces:**
- Consumes: `store.WithTx`, `purposes.ValidatePurposeTx`, existing `insertTask` SQL (no schema change).
- Produces: `execution.Service.CreateTaskWithGuardInTx(ctx, tx, request, guard)`; `workflowcase.Service.EnsureAndMaterialize(ctx, executionSvc, observation, template) (Case, domain.Task, error)` — consumed by Task 4 and the ADO observer.

- [ ] **Step 1: Write the failing execution test** — create `internal/execution/in_tx_test.go`:

```go
package execution_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

func intakeTaskFixture(t *testing.T) (context.Context, *state.Store, *execution.Service, domain.ID) {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	svc := execution.New(store, clk, purposes)
	envelope := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, ?, ?)`,
		envelope, 100, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	return ctx, store, svc, envelope
}

func intakeTaskRequest(envelope domain.ID) execution.TaskRequest {
	return execution.TaskRequest{
		Purpose:            domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: "owner-intake"},
		Objective:          "Triage GitHub issue o/r#7",
		AcceptanceCriteria: []string{"triage decision recorded for 2026-09-24T10:00:00Z"},
		ResourceEnvelopeID: envelope,
		IdempotencyKey:     "work-1",
	}
}

func TestCreateTaskWithGuardInTxRollsBackWithEnclosingTransaction(t *testing.T) {
	ctx, store, svc, envelope := intakeTaskFixture(t)
	failure := errors.New("rollback")
	err := store.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := svc.CreateTaskWithGuardInTx(ctx, tx, intakeTaskRequest(envelope), nil); err != nil {
			t.Fatal(err)
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want rollback sentinel", err)
	}
	if _, found, err := svc.FindByIdempotencyKey(ctx, "work-1"); err != nil || found {
		t.Fatalf("task persisted after rollback: found=%v err=%v", found, err)
	}
}

func TestCreateTaskWithGuardInTxPersistsOnCommitAndReplays(t *testing.T) {
	ctx, store, svc, envelope := intakeTaskFixture(t)
	create := func() domain.Task {
		t.Helper()
		var task domain.Task
		if err := store.WithTx(ctx, func(tx *sql.Tx) error {
			created, err := svc.CreateTaskWithGuardInTx(ctx, tx, intakeTaskRequest(envelope), nil)
			if err != nil {
				return err
			}
			task = created
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return task
	}
	first := create()
	second := create()
	if first.ID != second.ID {
		t.Fatalf("replay created a new task: %s vs %s", first.ID, second.ID)
	}
	if _, found, err := svc.FindByIdempotencyKey(ctx, "work-1"); err != nil || !found {
		t.Fatalf("committed task not found: found=%v err=%v", found, err)
	}
}

func TestCreateTaskWithGuardInTxAppliesGuard(t *testing.T) {
	ctx, store, svc, envelope := intakeTaskFixture(t)
	failure := errors.New("guard rejected")
	err := store.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := svc.CreateTaskWithGuardInTx(ctx, tx, intakeTaskRequest(envelope), func(context.Context, *sql.Tx) error {
			return failure
		})
		return err
	})
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want guard failure", err)
	}
	if _, found, err := svc.FindByIdempotencyKey(ctx, "work-1"); err != nil || found {
		t.Fatalf("guarded task persisted: found=%v err=%v", found, err)
	}
}
```

- [ ] **Step 2: Run execution test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/execution -run 'InTx' -count=1`
Expected: FAIL — `svc.CreateTaskWithGuardInTx undefined`.

- [ ] **Step 3: Implement the transaction-aware primitive** — in `internal/execution/service.go`:

Replace the current `insertTask` (lines 444-544) with the following three functions. The `insertTaskTx` body is elided (`// ...`) in the block below: that block is a shape reference, not a copy-paste target — the mechanical edit described in the paragraph after it is the authoritative instruction for that body.

```go
func (s *Service) insertTask(ctx context.Context, parentID domain.ID, request TaskRequest, guard TaskGuard) (domain.Task, error) {
	var task domain.Task
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		task, err = s.insertTaskTx(ctx, tx, parentID, request, guard)
		return err
	})
	if err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

// CreateTaskWithGuardInTx inserts or replays a Task inside an enclosing
// transaction so canonical writes that must commit together share one
// transaction. SQLite runs a single connection, so callers must never open a
// nested transaction.
func (s *Service) CreateTaskWithGuardInTx(ctx context.Context, tx *sql.Tx, request TaskRequest, guard TaskGuard) (domain.Task, error) {
	if err := s.configured(); err != nil {
		return domain.Task{}, err
	}
	if tx == nil {
		return domain.Task{}, errors.New("SQL transaction is required")
	}
	normalized, err := normalizeRootRequest(request)
	if err != nil {
		return domain.Task{}, err
	}
	return s.insertTaskTx(ctx, tx, domain.ID(""), normalized, guard)
}

func (s *Service) insertTaskTx(ctx context.Context, tx *sql.Tx, parentID domain.ID, request TaskRequest, guard TaskGuard) (domain.Task, error) {
	request, err := normalizeTaskIntent(request)
	if err != nil {
		return domain.Task{}, err
	}
	// ... the previous insertTask body from `var requestHash string` through
	// the replay/validate logic, with the `s.store.WithTx(ctx, func(tx *sql.Tx) error {`
	// wrapper removed and every one-value return rewritten as shown below.
}
```

Implementation instruction: mechanically edit — keep every line from `var requestHash string` (old line 450) through the insert/replay logic, de-indent by one tab, drop the `s.store.WithTx` wrapper, and rewrite the two return sites that used to return only `error`:

```go
	if inserted == 1 {
		if err := s.purpose.ValidatePurposeTx(ctx, tx, task.Purpose); err != nil {
			return domain.Task{}, err
		}
		return task, nil
	}
```

and the replay tail, which currently ends with `return err`:

```go
	task, err = loadTask(ctx, tx, existingID)
	if err != nil {
		return domain.Task{}, err
	}
	return task, nil
```

`insertTask` keeps its own `WithTx` wrapper (shown above) and returns whatever `insertTaskTx` returns, so the existing `CreateChildTask` call path and all replay behavior stay identical.

- [ ] **Step 4: Run execution tests to verify they pass**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/execution -count=1`
Expected: PASS (new + pre-existing tests, including idempotency replay).

- [ ] **Step 5: Write the failing workflowcase test** — create `internal/workflowcase/ensure_materialize_test.go` (package `workflowcase`, matching `service_test.go`):

```go
package workflowcase

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
	"github.com/SofiaFlux/summa42/internal/workflow"
)

func ensureFixture(t *testing.T) (context.Context, *state.Store, *Service, *execution.Service, domain.ID, domain.ID) {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	cases := New(store, clk, purposes)
	mission, err := purposes.CreateMission(ctx, "triage incoming issues")
	if err != nil {
		t.Fatal(err)
	}
	envelope := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, ?, ?)`,
		envelope, 100, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	return ctx, store, cases, execSvc, mission, envelope
}

func githubObservation(mission domain.ID) Observation {
	return Observation{
		MissionID: mission, Source: "github", ObjectID: "github:o/r#7",
		RevisionID: "2026-09-24T10:00:00Z", EvidenceID: "evidence-1",
		FirstWork: workflow.WorkProposal{Kind: "github.issue.triage", RequiredCapabilities: []string{"github.issue.read"}, AuthorityCeiling: []string{"github.issue.read"}},
		Grant:     workflow.Grant{Capabilities: []string{"github.issue.read"}}, MaxSteps: 3, RemainingBudget: 10,
	}
}

func triageTemplate(envelope domain.ID) execution.TaskRequest {
	return execution.TaskRequest{
		Objective:          "Triage GitHub issue o/r#7",
		PayloadJSON:        json.RawMessage(`{"issue":7}`),
		AcceptanceCriteria: []string{"triage decision recorded for 2026-09-24T10:00:00Z"},
		ResourceEnvelopeID: envelope,
	}
}

func TestEnsureAndMaterializeCreatesCaseAndTask(t *testing.T) {
	ctx, _, cases, execSvc, mission, envelope := ensureFixture(t)
	created, task, err := cases.EnsureAndMaterialize(ctx, execSvc, githubObservation(mission), triageTemplate(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || task.ID == "" {
		t.Fatalf("created=%+v task=%+v", created, task)
	}
	if task.IdempotencyKey != string(created.CurrentWorkID) {
		t.Fatalf("idempotency key = %q, want work %q", task.IdempotencyKey, created.CurrentWorkID)
	}
	if task.TaskClass != "github.issue.triage" {
		t.Fatalf("task class = %q", task.TaskClass)
	}
	if len(task.RequiredCapabilities) != 1 || task.RequiredCapabilities[0] != "github.issue.read" {
		t.Fatalf("capabilities = %v", task.RequiredCapabilities)
	}
	if task.RequiredEnforcement != domain.EnforcementUnenforced {
		t.Fatalf("required enforcement = %q, want %q", task.RequiredEnforcement, domain.EnforcementUnenforced)
	}
	stored, err := cases.Get(ctx, created.ID)
	if err != nil || stored.ObjectID != "github:o/r#7" {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	fetched, err := execSvc.Task(ctx, task.ID)
	if err != nil || fetched.Objective != "Triage GitHub issue o/r#7" {
		t.Fatalf("fetched=%+v err=%v", fetched, err)
	}
}

func TestEnsureAndMaterializeRollsBackCaseWhenTaskInsertFails(t *testing.T) {
	ctx, store, cases, execSvc, mission, envelope := ensureFixture(t)
	template := triageTemplate(envelope)
	template.RequiredEnforcement = domain.EnforcementLevel("BOGUS")
	if _, _, err := cases.EnsureAndMaterialize(ctx, execSvc, githubObservation(mission), template); err == nil {
		t.Fatal("expected task creation error")
	}
	if _, found, err := cases.Find(ctx, mission, "github", "github:o/r#7", "2026-09-24T10:00:00Z"); err != nil || found {
		t.Fatalf("case persisted after task failure: found=%v err=%v", found, err)
	}
	var tasks int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 {
		t.Fatalf("tasks = %d, want 0 after rollback", tasks)
	}
}

func TestEnsureAndMaterializeRejectsMissingServices(t *testing.T) {
	ctx, _, cases, execSvc, mission, envelope := ensureFixture(t)
	if _, _, err := cases.EnsureAndMaterialize(ctx, nil, githubObservation(mission), triageTemplate(envelope)); err == nil {
		t.Fatal("expected error for nil execution service")
	}
	if _, _, err := cases.EnsureAndMaterialize(ctx, execSvc, githubObservation(mission), execution.TaskRequest{}); err == nil {
		t.Fatal("expected error for empty template")
	}
}
```

Fixture note: the store type is `*state.Store` from `github.com/SofiaFlux/summa42/internal/state/sqlite` (imported aliased as `state`); `New` in `ensureFixture` is `workflowcase.New` (same package, unqualified).

- [ ] **Step 6: Run workflowcase test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/workflowcase -run 'EnsureAndMaterialize' -count=1`
Expected: FAIL — `cases.EnsureAndMaterialize undefined`.

- [ ] **Step 7: Implement `EnsureAndMaterialize`**

`internal/workflowcase/service.go` — refactor `Ensure` (lines 433-507) into:

```go
type preparedObservation struct {
	observation Observation
	workID      domain.ID
	requestJSON string
	grantJSON   string
	workJSON    string
	now         string
}

func (s *Service) prepareObservation(observation Observation) (preparedObservation, error) {
	var prepared preparedObservation
	if strings.TrimSpace(string(observation.MissionID)) == "" ||
		strings.TrimSpace(observation.Source) == "" ||
		strings.TrimSpace(observation.ObjectID) == "" ||
		strings.TrimSpace(observation.RevisionID) == "" ||
		strings.TrimSpace(observation.EvidenceID) == "" ||
		strings.TrimSpace(observation.FirstWork.Kind) == "" ||
		observation.MaxSteps <= 0 || observation.RemainingBudget <= 0 {
		return prepared, errors.New("observation identity, evidence, first work, and positive limits are required")
	}
	decision, err := workflow.Decide(workflow.Input{
		Assessment: workflow.Assessment{
			Verdict: workflow.Continue, EvidenceIDs: []string{observation.EvidenceID}, Next: &observation.FirstWork,
		},
		Grant:          observation.Grant,
		Limits:         workflow.Limits{MaxSteps: observation.MaxSteps, RemainingBudget: observation.RemainingBudget},
		CompletedSteps: 0,
	})
	if err != nil {
		return prepared, fmt.Errorf("invalid first work: %w", err)
	}
	if decision.Outcome != workflow.OutcomeContinue {
		return prepared, fmt.Errorf("first work decision is %s", decision.Outcome)
	}
	requestJSON, err := json.Marshal(observation)
	if err != nil {
		return prepared, fmt.Errorf("encode observation: %w", err)
	}
	grantJSON, err := json.Marshal(observation.Grant)
	if err != nil {
		return prepared, fmt.Errorf("encode grant: %w", err)
	}
	workJSON, err := json.Marshal(observation.FirstWork)
	if err != nil {
		return prepared, fmt.Errorf("encode first work: %w", err)
	}
	prepared.observation = observation
	prepared.workID = domain.NewID("work")
	prepared.requestJSON = string(requestJSON)
	prepared.grantJSON = string(grantJSON)
	prepared.workJSON = string(workJSON)
	prepared.now = s.clock.Now().UTC().Format(time.RFC3339Nano)
	return prepared, nil
}

func (s *Service) ensureTx(ctx context.Context, tx *sql.Tx, prepared preparedObservation) (Case, error) {
	observation := prepared.observation
	if err := s.purposes.ValidatePurposeTx(ctx, tx, domain.PurposeRef{Kind: domain.PurposeMission, ID: observation.MissionID}); err != nil {
		return Case{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_cases (
		case_id, mission_id, source, object_id, revision_id, observation_evidence_id,
		initial_request_json, grant_json, state, current_work_id, next_work_json,
		completed_steps, max_steps, remaining_budget, progress_signature, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(mission_id, source, object_id, revision_id) DO NOTHING`,
		domain.NewID("case"), observation.MissionID, observation.Source, observation.ObjectID,
		observation.RevisionID, observation.EvidenceID, prepared.requestJSON, prepared.grantJSON, Active,
		prepared.workID, prepared.workJSON, 0, observation.MaxSteps, observation.RemainingBudget, "", prepared.now, prepared.now,
	); err != nil {
		return Case{}, err
	}
	row := tx.QueryRowContext(ctx, `SELECT `+caseColumns+` FROM workflow_cases WHERE mission_id = ? AND source = ? AND object_id = ? AND revision_id = ?`,
		observation.MissionID, observation.Source, observation.ObjectID, observation.RevisionID)
	result, storedRequest, err := scanCase(row)
	if err != nil {
		return Case{}, err
	}
	if storedRequest != prepared.requestJSON {
		return Case{}, errors.New("observation revision already exists with a different initial request")
	}
	return result, nil
}

func (s *Service) Ensure(ctx context.Context, observation Observation) (Case, error) {
	if s == nil || s.store == nil || s.clock == nil || s.purposes == nil {
		return Case{}, errors.New("workflow case service is not configured")
	}
	prepared, err := s.prepareObservation(observation)
	if err != nil {
		return Case{}, err
	}
	var result Case
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = s.ensureTx(ctx, tx, prepared)
		return err
	}); err != nil {
		return Case{}, fmt.Errorf("ensure workflow case: %w", err)
	}
	return result, nil
}
```

`internal/workflowcase/materialize.go` — replace `materializeCurrentCase` (lines 30-55) with:

```go
func templateForCase(c Case, template execution.TaskRequest) (execution.TaskRequest, error) {
	if strings.TrimSpace(template.Objective) == "" ||
		!hasNonblankCriterion(template.AcceptanceCriteria) ||
		strings.TrimSpace(string(template.ResourceEnvelopeID)) == "" {
		return execution.TaskRequest{}, errors.New("task template requires objective, acceptance criteria, and resource envelope")
	}
	template.Purpose = domain.PurposeRef{Kind: domain.PurposeMission, ID: c.MissionID}
	template.TaskClass = c.NextWork.Kind
	template.RequiredCapabilities = append([]string(nil), c.NextWork.RequiredCapabilities...)
	template.AuthorityCeiling = append([]string(nil), c.NextWork.AuthorityCeiling...)
	template.IdempotencyKey = string(c.CurrentWorkID)
	return template, nil
}

func activeWorkGuard(c Case) execution.TaskGuard {
	return func(ctx context.Context, tx *sql.Tx) error {
		var active int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM workflow_cases
			WHERE case_id = ? AND mission_id = ? AND state = ? AND current_work_id = ?`,
			c.ID, c.MissionID, Active, c.CurrentWorkID).Scan(&active)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("work is not the active work for this case")
		}
		return err
	}
}

func (s *Service) materializeCurrentCase(ctx context.Context, executionSvc *execution.Service, c Case, template execution.TaskRequest) (domain.Task, error) {
	prepared, err := templateForCase(c, template)
	if err != nil {
		return domain.Task{}, err
	}
	task, err := executionSvc.CreateTaskWithGuard(ctx, prepared, activeWorkGuard(c))
	if err != nil {
		return domain.Task{}, fmt.Errorf("materialize workflow work: %w", err)
	}
	return task, nil
}
```

New file `internal/workflowcase/ensure_materialize.go`:

```go
package workflowcase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
)

// EnsureAndMaterialize creates the workflow case and its first Task in one
// transaction: any Task failure rolls the case back, so no partial state
// survives.
func (s *Service) EnsureAndMaterialize(ctx context.Context, executionSvc *execution.Service, observation Observation, template execution.TaskRequest) (Case, domain.Task, error) {
	if s == nil || s.store == nil || s.clock == nil || s.purposes == nil {
		return Case{}, domain.Task{}, errors.New("workflow case service is not configured")
	}
	if executionSvc == nil {
		return Case{}, domain.Task{}, errors.New("execution service is required")
	}
	prepared, err := s.prepareObservation(observation)
	if err != nil {
		return Case{}, domain.Task{}, err
	}
	var (
		result Case
		task   domain.Task
	)
	if err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		created, err := s.ensureTx(ctx, tx, prepared)
		if err != nil {
			return err
		}
		taskTemplate, err := templateForCase(created, template)
		if err != nil {
			return err
		}
		createdTask, err := executionSvc.CreateTaskWithGuardInTx(ctx, tx, taskTemplate, activeWorkGuard(created))
		if err != nil {
			return fmt.Errorf("materialize workflow work: %w", err)
		}
		result, task = created, createdTask
		return nil
	}); err != nil {
		return Case{}, domain.Task{}, fmt.Errorf("ensure and materialize workflow case: %w", err)
	}
	return result, task, nil
}
```

- [ ] **Step 8: Run workflowcase tests to verify they pass**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/workflowcase -count=1`
Expected: PASS (new tests + existing `Ensure`/`MaterializeTask` tests unchanged).

- [ ] **Step 9: Write the failing ADO migration test** — create `internal/adoreview/atomic_miss_test.go` (package `adoreview`, reusing `setupObserve`, `fakeCaller`, `goodPRItem` from the package's tests):

```go
package adoreview

import (
	"testing"
)

// The miss path now commits case + Task in one transaction; the task is keyed
// by the case's work ID, so the linkage must hold for every fresh observation.
func TestObserveOnceMissPathLinksTaskToCaseWork(t *testing.T) {
	ctx, store, cases, execSvc, evidenceStore, cfg := setupObserve(t)
	caller := &fakeCaller{pages: []any{map[string]any{"prs": []any{goodPRItem()}}}}
	result, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ensured) != 1 || len(result.Materialized) != 1 || len(result.Failed) != 0 {
		t.Fatalf("result = %+v", result)
	}
	var workID string
	if err := store.DB().QueryRowContext(ctx,
		`SELECT current_work_id FROM workflow_cases WHERE case_id = ?`, result.Ensured[0]).Scan(&workID); err != nil {
		t.Fatal(err)
	}
	task, err := execSvc.Task(ctx, result.Materialized[0])
	if err != nil {
		t.Fatal(err)
	}
	if task.IdempotencyKey != workID {
		t.Fatalf("task idempotency key = %q, want case work %q", task.IdempotencyKey, workID)
	}
	if task.TaskClass != "ado.pr.review" {
		t.Fatalf("task class = %q", task.TaskClass)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM workflow_cases WHERE case_id = ?`, result.Ensured[0]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("case rows = %d, want 1", count)
	}
}
```

Note on scope: an ADO-side rollback test is not writable without weakening `Config.validate()` (every task-insert failure reachable from a valid Config is already covered by the workflowcase rollback test); the pre-existing suite — including `TestObserveOnceExcludesAndRepairsPartialCase` (hit-path repair of legacy partial cases) and `TestObserveOnceEnsureFailureYieldsExclusion` — must stay green unchanged.

- [ ] **Step 10: Migrate the ADO miss path** — in `internal/adoreview/observe.go`, inside `observeOne`, replace the miss branch (old lines 138-164: `canonical, err := json.Marshal(canonicalEvidence(pr))` … `result.Materialized = append(result.Materialized, task.ID); return nil`) with:

```go
	canonical, err := json.Marshal(canonicalEvidence(pr))
	if err != nil {
		return err
	}
	object, err := evidenceStore.Put(ctx, strings.NewReader(string(canonical)), evidence.Metadata{MediaType: "application/json", Kind: "ado.pr.snapshot"})
	if err != nil {
		return err
	}
	workCaps := cfg.workCapabilities()
	payload, err := prPayload(pr)
	if err != nil {
		return err
	}
	created, task, err := cases.EnsureAndMaterialize(ctx, execSvc, workflowcase.Observation{
		MissionID: cfg.MissionID, Source: "ado", ObjectID: pr.ObjectID(), RevisionID: pr.RevisionID(),
		EvidenceID: string(object.ID),
		FirstWork:  workflow.WorkProposal{Kind: "ado.pr.review", RequiredCapabilities: workCaps, AuthorityCeiling: append([]string(nil), cfg.Grant.Capabilities...)},
		Grant:      cfg.Grant, MaxSteps: cfg.MaxSteps, RemainingBudget: cfg.RemainingBudget,
	}, execution.TaskRequest{
		Objective:          fmt.Sprintf("Review ADO PR %s", pr.ObjectID()),
		PayloadJSON:        payload,
		AcceptanceCriteria: []string{"review evidence recorded for " + pr.RevisionID()},
		ResourceEnvelopeID: cfg.ResourceEnvelopeID,
	})
	if err != nil {
		result.Excluded = append(result.Excluded, ExcludedPR{PR: pr, Reason: ReasonEnsureFailed})
		return nil
	}
	result.Ensured = append(result.Ensured, created.ID)
	result.Materialized = append(result.Materialized, task.ID)
	return nil
```

Add the payload helper next to the existing `materialize` function in the same file (and make `materialize` reuse it):

```go
func prPayload(pr PullRequest) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]any{
		"repo": pr.Repository, "pr": pr.Number, "sourceCommit": pr.SourceCommit, "targetCommit": pr.TargetCommit,
	})
	if err != nil {
		return nil, fmt.Errorf("encode ADO review payload: %w", err)
	}
	return payload, nil
}
```

The hit path of `observeOne` and `MaterializeTask` stay unchanged (legacy partial-case repair keeps working).

`materialize` must keep compiling after the change: it currently inlines `json.Marshal` (see `internal/adoreview/observe.go:182-195`). Replace that inline marshal with a call to the new helper so both paths share one payload builder:

```go
	payload, err := prPayload(pr)
	if err != nil {
		return domain.Task{}, err
	}
```

- [ ] **Step 11: Run ADO + affected suites**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview ./internal/workflowcase ./internal/execution -count=1`
Expected: PASS, including `TestObserveOnceExcludesAndRepairsPartialCase` and `TestObserveOnceMaterializeFailureAfterAssessmentSurfacesFailedAndReusesCase`.

- [ ] **Step 12: Commit**

```bash
git add internal/execution/service.go internal/execution/in_tx_test.go internal/workflowcase/service.go internal/workflowcase/materialize.go internal/workflowcase/ensure_materialize.go internal/workflowcase/ensure_materialize_test.go internal/adoreview/observe.go internal/adoreview/atomic_miss_test.go
git commit -m "feat: create workflow case and task atomically"
```

### Task 4: Observer (ObserveOnce/Run)

**Files:**
- Create: `internal/ghissue/observe.go`
- Create: `internal/ghissue/observe_test.go`

**Interfaces:**
- Consumes: `IssueLister`, `CollectIssues`, `FilterIssues`, `CanonicalSnapshot`, `ExcludedIssue` (Tasks 1-3); `cases.Find`, `evidenceStore.Put`, `cases.EnsureAndMaterialize`, `cases.MaterializeTask`; `workflowcase.Active`; `workflow.Grant`.
- Produces: `ObserveConfig`, `ObserveResult`, `FailedIssue`, `ObserveOnce`, `Run` — consumed by Task 5.

- [ ] **Step 1: Write the failing test** — create `internal/ghissue/observe_test.go`:

```go
package ghissue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
	"github.com/SofiaFlux/summa42/internal/workflow"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
)

type observeFixture struct {
	ctx           context.Context
	store         *state.Store
	cases         *workflowcase.Service
	execSvc       *execution.Service
	evidenceStore *evidence.Store
	purposes      *purpose.Service
	cfg           ObserveConfig
}

func setupObserve(t *testing.T) observeFixture {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	cases := workflowcase.New(store, clk, purposes)
	mission, err := purposes.CreateMission(ctx, "triage incoming issues")
	if err != nil {
		t.Fatal(err)
	}
	envelope := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, ?, ?)`,
		envelope, 100, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	return observeFixture{
		ctx: ctx, store: store, cases: cases, execSvc: execSvc, evidenceStore: evidenceStore, purposes: purposes,
		cfg: ObserveConfig{
			MissionID: mission, Repository: "o/r", Maintainers: []string{"maint"},
			Grant:              workflow.Grant{Capabilities: []string{"github.issue.read"}},
			ResourceEnvelopeID: envelope, MaxSteps: 3, RemainingBudget: 10,
		},
	}
}

func snapshotCount(t *testing.T, store *state.Store, ctx context.Context) int {
	t.Helper()
	var n int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM evidence_objects WHERE kind = 'github.issue.snapshot'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestObserveOnceRegistersMaintainerIssueAndClassifiesOthers(t *testing.T) {
	f := setupObserve(t)
	lister := &stubLister{name: "o/r", pages: []stubPage{{
		items: []any{
			wireIssue(),
			wireIssue(func(m map[string]any) { m["pull_request"] = map[string]any{"url": "x"} }),
			wireIssue(func(m map[string]any) { m["user"] = map[string]any{"login": "stranger"} }),
			wireIssue(func(m map[string]any) { m["number"] = float64(8); m["labels"] = []any{map[string]any{"name": "wontfix"}} }),
		},
		bad: []Unparseable{{Raw: map[string]any{"number": float64(99)}}},
	}}}
	result, err := ObserveOnce(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ensured) != 1 || len(result.Materialized) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.PullRequestsSkipped != 1 {
		t.Fatalf("skipped = %d, want 1", result.PullRequestsSkipped)
	}
	reasons := map[string]int{}
	for _, excluded := range result.Excluded {
		reasons[excluded.Reason]++
	}
	if reasons[ReasonUnparseable] != 1 || reasons[ReasonNotMaintainer] != 1 || reasons[ReasonHeldByLabel] != 1 {
		t.Fatalf("reasons = %v", reasons)
	}
	task, err := f.execSvc.Task(f.ctx, result.Materialized[0])
	if err != nil {
		t.Fatal(err)
	}
	if task.Objective != "Triage GitHub issue o/r#7" {
		t.Fatalf("objective = %q", task.Objective)
	}
	if task.State != domain.TaskEligible {
		t.Fatalf("state = %q, want ELIGIBLE (never leased)", task.State)
	}
	if task.TaskClass != "github.issue.triage" {
		t.Fatalf("class = %q", task.TaskClass)
	}
	stored, err := f.cases.Get(f.ctx, result.Ensured[0])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(task.PayloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["issueSnapshot"] != stored.ObservationEvidenceID {
		t.Fatalf("issueSnapshot = %v, want %s", payload["issueSnapshot"], stored.ObservationEvidenceID)
	}
	if payload["repo"] != "o/r" || payload["revision"] != "2026-09-24T10:00:00Z" || payload["triage"] != TriageUnclassified {
		t.Fatalf("payload = %v", payload)
	}
	if _, ok := payload["body"]; ok {
		t.Fatal("payload inlines body; the executor must resolve it through evidence")
	}
	if len(task.AcceptanceCriteria) != 1 || task.AcceptanceCriteria[0] != "triage decision recorded for 2026-09-24T10:00:00Z" {
		t.Fatalf("criteria = %v", task.AcceptanceCriteria)
	}
	_, snapshot, err := f.evidenceStore.Get(f.ctx, domain.ID(stored.ObservationEvidenceID))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(snapshot, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["updatedAt"] != stored.RevisionID {
		t.Fatalf("snapshot revision = %v, want %s", decoded["updatedAt"], stored.RevisionID)
	}
}

func TestObserveOnceRepollIsNoopAndNormalizesOffsets(t *testing.T) {
	f := setupObserve(t)
	page := func(updated string) *stubLister {
		return &stubLister{name: "o/r", pages: []stubPage{{items: []any{
			wireIssue(func(m map[string]any) { m["updated_at"] = updated }),
		}}}}
	}
	first, err := ObserveOnce(f.ctx, page("2026-09-24T10:00:00+00:00"), f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotCount(t, f.store, f.ctx)
	sameInstant, err := ObserveOnce(f.ctx, page("2026-09-24T11:00:00+01:00"), f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sameInstant.Ensured[0] != first.Ensured[0] || sameInstant.Materialized[0] != first.Materialized[0] {
		t.Fatalf("offset-equivalent revision created new work: %+v vs %+v", sameInstant, first)
	}
	if after := snapshotCount(t, f.store, f.ctx); after != before {
		t.Fatalf("evidence snapshots grew %d -> %d", before, after)
	}
	next, err := ObserveOnce(f.ctx, page("2026-09-24T13:00:00+02:00"), f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if next.Ensured[0] == first.Ensured[0] {
		t.Fatal("edited issue revision did not open a new case")
	}
}

func TestObserveOnceKeepsRepositoriesApart(t *testing.T) {
	f := setupObserve(t)
	firstCfg := f.cfg
	firstCfg.Repository = "owner-a/repo"
	secondCfg := f.cfg
	secondCfg.Repository = "owner-b/repo"
	first, err := ObserveOnce(f.ctx,
		&stubLister{name: "owner-a/repo", pages: []stubPage{{items: []any{wireIssue()}}}},
		f.cases, f.execSvc, f.evidenceStore, firstCfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ObserveOnce(f.ctx,
		&stubLister{name: "owner-b/repo", pages: []stubPage{{items: []any{wireIssue()}}}},
		f.cases, f.execSvc, f.evidenceStore, secondCfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.Ensured[0] == second.Ensured[0] {
		t.Fatal("cross-repository issue #7 collided")
	}
	stored, err := f.cases.Get(f.ctx, second.Ensured[0])
	if err != nil || stored.ObjectID != "github:owner-b/repo#7" {
		t.Fatalf("stored = %+v err = %v", stored, err)
	}
}

func TestObserveOnceRejectsGrantViolations(t *testing.T) {
	f := setupObserve(t)
	lister := &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	cases := map[string]func(ObserveConfig) ObserveConfig{
		"grant without read capability": func(c ObserveConfig) ObserveConfig {
			c.Grant = workflow.Grant{Capabilities: []string{"other.cap"}}
			return c
		},
		"empty grant": func(c ObserveConfig) ObserveConfig {
			c.Grant = workflow.Grant{}
			return c
		},
		"work capability outside grant": func(c ObserveConfig) ObserveConfig {
			c.WorkCapabilities = []string{"extra.cap"}
			return c
		},
		"missing maintainers": func(c ObserveConfig) ObserveConfig {
			c.Maintainers = nil
			return c
		},
		"blank maintainers": func(c ObserveConfig) ObserveConfig {
			c.Maintainers = []string{"  "}
			return c
		},
		"lister repository mismatch": func(c ObserveConfig) ObserveConfig {
			c.Repository = "o/other"
			return c
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ObserveOnce(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, mutate(f.cfg)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if count := snapshotCount(t, f.store, f.ctx); count != 0 {
		t.Fatalf("snapshots = %d, want 0 (validation precedes any poll)", count)
	}
}

func TestObserveOnceNormalizesPaddedGrantTokens(t *testing.T) {
	f := setupObserve(t)
	lister := &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	cfg := f.cfg
	cfg.Grant = workflow.Grant{Capabilities: []string{" github.issue.read ", "github.issue.read"}}
	cfg.Maintainers = []string{" Maint "}
	cfg.WorkCapabilities = []string{" "}
	result, err := ObserveOnce(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Ensured) != 1 {
		t.Fatalf("ensured = %d, want 1", len(result.Ensured))
	}
	task, err := f.execSvc.Task(f.ctx, result.Materialized[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(task.RequiredCapabilities) != 1 || task.RequiredCapabilities[0] != readCapability {
		t.Fatalf("required capabilities = %v", task.RequiredCapabilities)
	}
}

func TestObserveOnceSkipsInactiveCases(t *testing.T) {
	f := setupObserve(t)
	lister := func() *stubLister {
		return &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	}
	first, err := ObserveOnce(f.ctx, lister(), f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"BLOCKED", "READY_FOR_VERIFICATION", "CLOSED"} {
		if _, err := f.store.DB().ExecContext(f.ctx,
			`UPDATE workflow_cases SET state = ? WHERE case_id = ?`, state, first.Ensured[0]); err != nil {
			t.Fatal(err)
		}
		result, err := ObserveOnce(f.ctx, lister(), f.cases, f.execSvc, f.evidenceStore, f.cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Materialized) != 0 || len(result.Ensured) != 0 {
			t.Fatalf("state %s: result = %+v", state, result)
		}
		if len(result.Excluded) != 1 || result.Excluded[0].Reason != ReasonCaseNotActive {
			t.Fatalf("state %s: excluded = %+v", state, result.Excluded)
		}
	}
}

func TestObserveOnceExcludesEnsureFailureWithoutCase(t *testing.T) {
	f := setupObserve(t)
	if err := f.purposes.DeactivateMission(f.ctx, f.cfg.MissionID); err != nil {
		t.Fatal(err)
	}
	lister := &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	result, err := ObserveOnce(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, f.cfg)
	if err != nil {
		t.Fatalf("ObserveOnce = %v, want nil (ensure failure is an exclusion)", err)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].Reason != ReasonEnsureFailed {
		t.Fatalf("excluded = %+v", result.Excluded)
	}
	if len(result.Failed) != 0 || len(result.Ensured) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunReturnsNilOnCancellation(t *testing.T) {
	f := setupObserve(t)
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	lister := &stubLister{name: "o/r", pages: []stubPage{{items: []any{wireIssue()}}}}
	if err := Run(ctx, lister, f.cases, f.execSvc, f.evidenceStore, f.cfg, time.Millisecond); err != nil {
		t.Fatalf("Run = %v, want nil on cancelled context", err)
	}
}

func TestRunPropagatesTickError(t *testing.T) {
	f := setupObserve(t)
	lister := &stubLister{name: "not-a-repo", pages: nil}
	if err := Run(f.ctx, lister, f.cases, f.execSvc, f.evidenceStore, f.cfg, time.Millisecond); err == nil {
		t.Fatal("expected tick error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -run 'ObserveOnce|Run' -count=1`
Expected: FAIL — `undefined: ObserveConfig`, `undefined: ObserveOnce`, `undefined: Run`.

- [ ] **Step 3: Write minimal implementation** — create `internal/ghissue/observe.go`:

```go
package ghissue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/workflow"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
)

const readCapability = "github.issue.read"

// ObserveConfig configures issue intake. Config in client.go is the transport
// configuration; this is the observer's.
type ObserveConfig struct {
	MissionID          domain.ID
	Repository         string
	Maintainers        []string
	Grant              workflow.Grant
	WorkCapabilities   []string
	ResourceEnvelopeID domain.ID
	MaxSteps           int
	RemainingBudget    int64
}

// normalized trims and deduplicates every string collection so validation,
// FirstWork.AuthorityCeiling and workflow.Decide all compare the same tokens.
func (c ObserveConfig) normalized() ObserveConfig {
	normalized := ObserveConfig{
		MissionID:          domain.ID(strings.TrimSpace(string(c.MissionID))),
		Repository:         strings.TrimSpace(c.Repository),
		Maintainers:        normalizeTokens(c.Maintainers),
		Grant:              workflow.Grant{Capabilities: normalizeTokens(c.Grant.Capabilities), Actions: normalizeTokens(c.Grant.Actions)},
		WorkCapabilities:   normalizeTokens(c.WorkCapabilities),
		ResourceEnvelopeID: domain.ID(strings.TrimSpace(string(c.ResourceEnvelopeID))),
		MaxSteps:           c.MaxSteps,
		RemainingBudget:    c.RemainingBudget,
	}
	return normalized
}

func (c ObserveConfig) validate() error {
	if strings.TrimSpace(string(c.MissionID)) == "" {
		return errors.New("mission is required")
	}
	if strings.TrimSpace(c.Repository) == "" {
		return errors.New("repository is required")
	}
	if len(c.Maintainers) == 0 {
		return errors.New("at least one maintainer login is required")
	}
	if strings.TrimSpace(string(c.ResourceEnvelopeID)) == "" {
		return errors.New("resource envelope is required")
	}
	if c.MaxSteps <= 0 || c.RemainingBudget <= 0 {
		return errors.New("positive max steps and remaining budget are required")
	}
	granted := make(map[string]struct{}, len(c.Grant.Capabilities))
	for _, capability := range c.Grant.Capabilities {
		if trimmed := strings.TrimSpace(capability); trimmed != "" {
			granted[trimmed] = struct{}{}
		}
	}
	if len(granted) == 0 {
		return errors.New("grant capabilities are required")
	}
	if _, ok := granted[readCapability]; !ok {
		return fmt.Errorf("grant must include %s", readCapability)
	}
	for _, capability := range c.effectiveCapabilities() {
		if _, ok := granted[capability]; !ok {
			return fmt.Errorf("work capability %q exceeds the configured grant", capability)
		}
	}
	return nil
}

// effectiveCapabilities always includes github.issue.read; extra work
// capabilities are additive only.
func (c ObserveConfig) effectiveCapabilities() []string {
	seen := make(map[string]struct{}, len(c.WorkCapabilities)+1)
	out := make([]string, 0, len(c.WorkCapabilities)+1)
	for _, capability := range append([]string{readCapability}, c.WorkCapabilities...) {
		trimmed := strings.TrimSpace(capability)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// FailedIssue records one issue whose durable Task creation failed on the hit path.
type FailedIssue struct {
	Issue Issue
	Err   string
}

// ObserveResult summarizes one tick.
type ObserveResult struct {
	Ensured             []domain.ID
	Materialized        []domain.ID
	Excluded            []ExcludedIssue
	Failed              []FailedIssue
	PullRequestsSkipped int
}

func ObserveOnce(ctx context.Context, lister IssueLister, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg ObserveConfig) (ObserveResult, error) {
	var result ObserveResult
	if ctx == nil {
		return result, errors.New("observer context is required")
	}
	if cases == nil || execSvc == nil || evidenceStore == nil {
		return result, errors.New("workflow case, execution and evidence services are required")
	}
	if lister == nil {
		return result, errors.New("issue lister is required")
	}
	cfg = cfg.normalized()
	if err := cfg.validate(); err != nil {
		return result, err
	}
	if name := strings.TrimSpace(lister.Name()); !strings.EqualFold(name, cfg.Repository) {
		return result, fmt.Errorf("issue lister repository %q does not match configured repository %q", name, cfg.Repository)
	}
	issues, unparseable, err := CollectIssues(ctx, lister)
	if err != nil {
		return result, err
	}
	for range unparseable {
		result.Excluded = append(result.Excluded, ExcludedIssue{Reason: ReasonUnparseable})
	}
	kept, excluded, skipped := FilterIssues(issues, cfg.Maintainers)
	result.PullRequestsSkipped = skipped
	result.Excluded = append(result.Excluded, excluded...)
	for _, issue := range kept {
		if err := observeIssue(ctx, cases, execSvc, evidenceStore, cfg, issue, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func observeIssue(ctx context.Context, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg ObserveConfig, issue Issue, result *ObserveResult) error {
	existing, found, err := cases.Find(ctx, cfg.MissionID, "github", issue.ObjectID(), issue.RevisionID())
	if err != nil {
		return err
	}
	if found {
		if existing.State != workflowcase.Active {
			result.Excluded = append(result.Excluded, ExcludedIssue{Issue: issue, Reason: ReasonCaseNotActive})
			return nil
		}
		template, err := taskTemplate(cfg, issue, existing.ObservationEvidenceID)
		if err != nil {
			return err
		}
		task, err := cases.MaterializeTask(ctx, execSvc, existing.ID, existing.CurrentWorkID, template)
		if err != nil {
			result.Failed = append(result.Failed, FailedIssue{Issue: issue, Err: err.Error()})
			return nil
		}
		result.Ensured = append(result.Ensured, existing.ID)
		result.Materialized = append(result.Materialized, task.ID)
		return nil
	}
	snapshot, err := CanonicalSnapshot(issue)
	if err != nil {
		return err
	}
	object, err := evidenceStore.Put(ctx, bytes.NewReader(snapshot), evidence.Metadata{MediaType: "application/json", Kind: "github.issue.snapshot"})
	if err != nil {
		return err
	}
	template, err := taskTemplate(cfg, issue, string(object.ID))
	if err != nil {
		return err
	}
	created, task, err := cases.EnsureAndMaterialize(ctx, execSvc, workflowcase.Observation{
		MissionID: cfg.MissionID, Source: "github", ObjectID: issue.ObjectID(), RevisionID: issue.RevisionID(),
		EvidenceID: string(object.ID),
		FirstWork: workflow.WorkProposal{
			Kind: "github.issue.triage", RequiredCapabilities: cfg.effectiveCapabilities(),
			AuthorityCeiling: append([]string(nil), cfg.Grant.Capabilities...),
		},
		Grant: cfg.Grant, MaxSteps: cfg.MaxSteps, RemainingBudget: cfg.RemainingBudget,
	}, template)
	if err != nil {
		result.Excluded = append(result.Excluded, ExcludedIssue{Issue: issue, Reason: ReasonEnsureFailed})
		return nil
	}
	result.Ensured = append(result.Ensured, created.ID)
	result.Materialized = append(result.Materialized, task.ID)
	return nil
}

func taskTemplate(cfg ObserveConfig, issue Issue, snapshotID string) (execution.TaskRequest, error) {
	payload, err := json.Marshal(map[string]any{
		"repo": issue.Repository, "issue": issue.Number, "revision": issue.RevisionID(),
		"title": issue.Title, "url": issue.URL, "author": issue.Author,
		"labels": nonNil(issue.Labels), "triage": issue.Triage, "issueSnapshot": snapshotID,
	})
	if err != nil {
		return execution.TaskRequest{}, fmt.Errorf("encode task payload: %w", err)
	}
	return execution.TaskRequest{
		Objective:          fmt.Sprintf("Triage GitHub issue %s#%d", issue.Repository, issue.Number),
		PayloadJSON:        payload,
		AcceptanceCriteria: []string{"triage decision recorded for " + issue.RevisionID()},
		ResourceEnvelopeID: cfg.ResourceEnvelopeID,
	}, nil
}

func Run(ctx context.Context, lister IssueLister, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg ObserveConfig, interval time.Duration) error {
	if ctx == nil {
		return errors.New("observer context is required")
	}
	if interval <= 0 {
		return errors.New("observer requires a positive poll interval")
	}
	if err := ctx.Err(); err != nil {
		return nil
	}
	if _, err := ObserveOnce(ctx, lister, cases, execSvc, evidenceStore, cfg); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := ObserveOnce(ctx, lister, cases, execSvc, evidenceStore, cfg); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -count=1 -v`
Expected: PASS (Tasks 1-4 suites).

- [ ] **Step 5: Commit**

```bash
git add internal/ghissue/observe.go internal/ghissue/observe_test.go
git commit -m "feat: observe maintainer issues into triage tasks"
```

### Task 5: run-gh-intake CLI, read-only composition, eligibility, full validation

**Files:**
- Modify: `cmd/summa42-box/main.go` (dispatch + `parseGHIntakeFlags` + `openGHIntakeBox` + `runGHIntake`)
- Create: `cmd/summa42-box/gh_intake_test.go`
- Create: `internal/scheduler/ghissue_intake_test.go`

**Interfaces:**
- Consumes: Task 4 `ghissue.Run/ObserveConfig/New/Config`; existing `stringSlice` flag type (`cmd/summa42-box/main.go:637`), `loadStartupMaterial`, `summa42runtime.Open`; `publishCapacityExecutor` fake (cmd test, `publish_env_test.go:86`).
- Produces: runnable `run-gh-intake`; guarantees no executor/capability provider/feedback sink exists in its Box.

- [ ] **Step 1: Write the failing CLI tests** — create `cmd/summa42-box/gh_intake_test.go`:

```go
package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/capabilities"
	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/fieldfeedback"
	"github.com/SofiaFlux/summa42/internal/localconfig"
	"github.com/SofiaFlux/summa42/internal/policy"
	summa42runtime "github.com/SofiaFlux/summa42/internal/runtime"
)

type intakePolicy struct{}

func (intakePolicy) Evaluate(_ context.Context, in policy.PolicyInput) (domain.PolicyDecision, error) {
	return domain.PolicyDecision{
		ID: domain.NewID("policy-decision"), Outcome: domain.PolicyAllow,
		PolicySetID: "policy-test", PolicySetHash: "hash", PolicyCapabilitiesHash: "caps",
		InputDigest: "input", EvaluatedAt: in.Now,
	}, nil
}

type intakeCapabilityProvider struct{}

func (intakeCapabilityProvider) Name() string { return "ado" }
func (intakeCapabilityProvider) Call(context.Context, string, any) (any, error) {
	return nil, nil
}

type intakeSink struct{}

func (intakeSink) Create(context.Context, fieldfeedback.IssuePayload) (string, int64, error) {
	return "issue-1", 0, nil
}
func (intakeSink) FindByMarker(context.Context, string) (string, bool, error) { return "", false, nil }

func validGHIntakeArgs() []string {
	return []string{
		"--mission", "mission-1", "--repo", "o/r", "--maintainer", "maint",
		"--grant-capability", "github.issue.read", "--envelope", "envelope-1",
		"--max-steps", "3", "--remaining-budget", "10", "--poll-interval", "1s",
	}
}

func withoutFlag(args []string, flag, value string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) && args[i+1] == value {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func replaceFlag(args []string, flag, old, value string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) && args[i+1] == old {
			out = append(out, flag, value)
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func TestParseGHIntakeFlagsAcceptsValidInput(t *testing.T) {
	t.Setenv("SUMMA42_GITHUB_REPOSITORY", "env/repo")
	cfg, interval, err := parseGHIntakeFlags(validGHIntakeArgs())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MissionID != "mission-1" || cfg.Repository != "o/r" {
		t.Fatalf("mission=%q repo=%q (--repo must win over env)", cfg.MissionID, cfg.Repository)
	}
	if len(cfg.Maintainers) != 1 || cfg.Maintainers[0] != "maint" {
		t.Fatalf("maintainers = %v", cfg.Maintainers)
	}
	if len(cfg.Grant.Capabilities) != 1 || cfg.Grant.Capabilities[0] != "github.issue.read" {
		t.Fatalf("grant = %v", cfg.Grant.Capabilities)
	}
	if cfg.ResourceEnvelopeID != "envelope-1" || cfg.MaxSteps != 3 || cfg.RemainingBudget != 10 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if interval != time.Second {
		t.Fatalf("interval = %v", interval)
	}
}

func TestParseGHIntakeFlagsFallsBackToRepositoryEnv(t *testing.T) {
	t.Setenv("SUMMA42_GITHUB_REPOSITORY", "env/repo")
	args := withoutFlag(validGHIntakeArgs(), "--repo", "o/r")
	cfg, _, err := parseGHIntakeFlags(args)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repository != "env/repo" {
		t.Fatalf("repo = %q, want env fallback", cfg.Repository)
	}
}

func TestParseGHIntakeFlagsRejectsInvalidInput(t *testing.T) {
	valid := validGHIntakeArgs()
	cases := map[string][]string{
		"missing mission":               withoutFlag(valid, "--mission", "mission-1"),
		"missing repository":            withoutFlag(valid, "--repo", "o/r"),
		"missing maintainer":            withoutFlag(valid, "--maintainer", "maint"),
		"missing envelope":              withoutFlag(valid, "--envelope", "envelope-1"),
		"empty grant":                   withoutFlag(valid, "--grant-capability", "github.issue.read"),
		"grant without read capability": replaceFlag(valid, "--grant-capability", "github.issue.read", "other.cap"),
		"non-positive max steps":        replaceFlag(valid, "--max-steps", "3", "0"),
		"non-positive budget":           replaceFlag(valid, "--remaining-budget", "10", "0"),
		"non-positive interval":         replaceFlag(valid, "--poll-interval", "1s", "0s"),
		"work capability outside grant": append(append([]string{}, valid...), "--work-capability", "extra.cap"),
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SUMMA42_GITHUB_REPOSITORY", "")
			if _, _, err := parseGHIntakeFlags(args); err == nil {
				t.Fatal("expected startup error")
			}
		})
	}
}

func TestOpenGHIntakeBoxStripsWritableComposition(t *testing.T) {
	root := t.TempDir()
	box, err := openGHIntakeBox(context.Background(), summa42runtime.Config{
		StatePath: filepath.Join(root, "state.db"), EvidencePath: filepath.Join(root, "evidence"),
		CollectiveID: "collective-1", OwnerPrincipalID: "owner-1", PolicyEngine: intakePolicy{},
		FieldFeedback: localconfig.FieldFeedbackConfig{
			Enabled: true, Mode: localconfig.FeedbackModeAutoIfAllowed, Provider: "github",
			Destination: "o/r", MaintenanceEnvelopeID: "envelope-1", RequiredEnforcement: domain.EnforcementEnforced,
		},
		FeedbackSink:        intakeSink{},
		OperationProviders:  nil,
		CapabilityProviders: []capabilities.Provider{intakeCapabilityProvider{}},
		Executors:           map[string]executors.Executor{"ado-publish": publishCapacityExecutor{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	if len(box.Executors) != 0 {
		t.Fatalf("executors = %v, want none", box.Executors)
	}
	if _, err := box.Capabilities.AssessProvider(context.Background(), "ado"); err == nil {
		t.Fatal("capability provider registered in read-only intake Box")
	}
}
```

The import block above is complete; `publishCapacityExecutor` is the existing fake from `cmd/summa42-box/publish_env_test.go:86` (same package, reused as-is).

- [ ] **Step 2: Run CLI test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -run 'GHIntake' -count=1`
Expected: FAIL — `undefined: parseGHIntakeFlags`, `undefined: openGHIntakeBox`.

- [ ] **Step 3: Write the failing scheduler test** — create `internal/scheduler/ghissue_intake_test.go`:

```go
package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	"github.com/SofiaFlux/summa42/internal/resources"
	"github.com/SofiaFlux/summa42/internal/scheduler"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

// An executor registry without github.issue.read can never lease triage work:
// the task stays ELIGIBLE but unclaimed.
func TestNextNeverClaimsGitHubIssueTriageWithoutReadCapability(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	resourceSvc := resources.New(store, clk)
	svc := scheduler.New(store, clk, purposes, execSvc, resourceSvc, time.Minute)

	envelope := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, ?, ?)`,
		envelope, 100, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	task, err := execSvc.CreateTask(ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: "owner-intake"},
		TaskClass:            "github.issue.triage",
		Objective:            "Triage GitHub issue o/r#7",
		AcceptanceCriteria:   []string{"triage decision recorded for 2026-09-24T10:00:00Z"},
		RequiredCapabilities: []string{"github.issue.read"},
		RequiredEnforcement:  domain.EnforcementEnforced,
		AuthorityCeiling:     []string{"github.issue.read"},
		ResourceEnvelopeID:   envelope,
		IdempotencyKey:       "work-intake",
	})
	if err != nil {
		t.Fatal(err)
	}
	unrelated := scheduler.CapacitySnapshot{Capabilities: map[string]scheduler.CapabilityCapacity{
		"shell": {Accessible: true, Enforcement: domain.EnforcementEnforced},
	}}
	if candidate, err := svc.Next(ctx, unrelated); err != nil {
		t.Fatal(err)
	} else if candidate != nil {
		t.Fatalf("candidate %s leased without github.issue.read", candidate.Task.ID)
	}
	partial := scheduler.CapacitySnapshot{Capabilities: map[string]scheduler.CapabilityCapacity{
		"github.issue.read": {Accessible: true, Enforcement: domain.EnforcementPartial},
	}}
	if candidate, err := svc.Next(ctx, partial); err != nil {
		t.Fatal(err)
	} else if candidate != nil {
		t.Fatalf("candidate %s leased on PARTIAL enforcement", candidate.Task.ID)
	}
	stored, err := execSvc.Task(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != domain.TaskEligible {
		t.Fatalf("state = %q, want ELIGIBLE (eligible but unclaimed)", stored.State)
	}
}
```

- [ ] **Step 4: Run scheduler test** — expected PASS immediately (it documents existing `scheduler.Next` behavior):

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/scheduler -run 'GitHubIssueTriage' -count=1`
Expected: PASS. If it fails, stop and report: the eligibility guarantee is a spec assumption that must be re-verified before continuing.

- [ ] **Step 5: Implement the CLI** — in `cmd/summa42-box/main.go`:

Add the dispatch block right after the `run-observer` block (old lines 446-452):

```go
	if len(os.Args) > 1 && os.Args[1] == "run-gh-intake" {
		if err := runGHIntake(ctx, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
```

Add these functions after `runObserver` (old line 765):

```go
func parseGHIntakeFlags(args []string) (ghissue.ObserveConfig, time.Duration, error) {
	var cfg ghissue.ObserveConfig
	var mission, envelope, repository string
	var maintainers, grantCaps, workCaps stringSlice
	var maxSteps int
	var remainingBudget int64
	var pollInterval time.Duration

	flags := flag.NewFlagSet("run-gh-intake", flag.ContinueOnError)
	flags.StringVar(&mission, "mission", "", "mission ID for GitHub issue cases")
	flags.StringVar(&repository, "repo", "", "GitHub repository as owner/name")
	flags.Var(&maintainers, "maintainer", "maintainer login eligible to open issues (repeatable)")
	flags.Var(&grantCaps, "grant-capability", "capability granted to issue cases (repeatable; must include github.issue.read)")
	flags.Var(&workCaps, "work-capability", "additional work capability (repeatable)")
	flags.StringVar(&envelope, "envelope", "", "resource envelope ID for triage tasks")
	flags.IntVar(&maxSteps, "max-steps", 3, "maximum workflow steps per issue case")
	flags.Int64Var(&remainingBudget, "remaining-budget", 10, "remaining workflow budget per issue case")
	flags.DurationVar(&pollInterval, "poll-interval", 30*time.Second, "interval between intake polls")
	if err := flags.Parse(args); err != nil {
		return ghissue.ObserveConfig{}, 0, err
	}
	if strings.TrimSpace(mission) == "" {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake requires --mission")
	}
	if strings.TrimSpace(repository) == "" {
		repository = strings.TrimSpace(os.Getenv("SUMMA42_GITHUB_REPOSITORY"))
	}
	if strings.TrimSpace(repository) == "" {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake requires --repo or SUMMA42_GITHUB_REPOSITORY")
	}
	if flags.NArg() != 0 {
		return ghissue.ObserveConfig{}, 0, fmt.Errorf("run-gh-intake takes no positional arguments, got %q", flags.Args())
	}
	logins := make([]string, 0, len(maintainers))
	for _, login := range maintainers {
		if strings.TrimSpace(login) == "" {
			return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake --maintainer must not be blank")
		}
		logins = append(logins, login)
	}
	if len(logins) == 0 {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake requires at least one --maintainer")
	}
	if strings.ContainsAny(repository, " \t") || !strings.Contains(repository, "/") {
		return ghissue.ObserveConfig{}, 0, fmt.Errorf("run-gh-intake repository %q must be owner/name", repository)
	}
	if strings.TrimSpace(envelope) == "" {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake requires --envelope")
	}
	if len(grantCaps) == 0 {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake requires --grant-capability github.issue.read")
	}
	granted := make(map[string]struct{}, len(grantCaps))
	for _, capability := range grantCaps {
		if trimmed := strings.TrimSpace(capability); trimmed != "" {
			granted[trimmed] = struct{}{}
		}
	}
	if _, ok := granted["github.issue.read"]; !ok {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake grant must include github.issue.read")
	}
	for _, capability := range workCaps {
		if _, ok := granted[strings.TrimSpace(capability)]; !ok {
			return ghissue.ObserveConfig{}, 0, fmt.Errorf("work capability %q is not listed in the grant", strings.TrimSpace(capability))
		}
	}
	if maxSteps <= 0 {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake requires a positive --max-steps")
	}
	if remainingBudget <= 0 {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake requires a positive --remaining-budget")
	}
	if pollInterval <= 0 {
		return ghissue.ObserveConfig{}, 0, errors.New("run-gh-intake requires a positive --poll-interval")
	}
	cfg.MissionID = domain.ID(strings.TrimSpace(mission))
	cfg.Repository = strings.TrimSpace(repository)
	cfg.Maintainers = append([]string(nil), logins...)
	cfg.Grant = workflow.Grant{Capabilities: append([]string(nil), grantCaps...)}
	cfg.WorkCapabilities = append([]string(nil), workCaps...)
	cfg.ResourceEnvelopeID = domain.ID(strings.TrimSpace(envelope))
	cfg.MaxSteps = maxSteps
	cfg.RemainingBudget = remainingBudget
	return cfg, pollInterval, nil
}

// openGHIntakeBox opens the runtime with every writable component removed:
// intake only reads GitHub, so no executor, capability provider, feedback sink,
// or operation provider may exist in this composition.
func openGHIntakeBox(ctx context.Context, cfg summa42runtime.Config) (*summa42runtime.Box, error) {
	if ctx == nil {
		return nil, errors.New("Box context is required")
	}
	cfg.FieldFeedback = localconfig.FieldFeedbackConfig{Enabled: false, Mode: localconfig.FeedbackModeLocalOnly}
	cfg.FeedbackSink = nil
	cfg.OperationProviders = nil
	cfg.CapabilityProviders = nil
	cfg.Executors = nil
	return summa42runtime.Open(ctx, cfg)
}

func runGHIntake(ctx context.Context, args []string) error {
	if ctx == nil {
		return errors.New("Box context is required")
	}
	observerCfg, pollInterval, err := parseGHIntakeFlags(args)
	if err != nil {
		return err
	}
	home, err := localconfig.ResolveHome("")
	if err != nil {
		return err
	}
	cfg, err := localconfig.Load(home)
	if err != nil {
		return fmt.Errorf("load initialized Collective: %w", err)
	}
	material, err := loadStartupMaterial(ctx, cfg)
	if err != nil {
		return err
	}
	client, err := ghissue.New(ghissue.Config{
		Repository: observerCfg.Repository,
		TokenFile:  strings.TrimSpace(os.Getenv("SUMMA42_GITHUB_TOKEN_FILE")),
	})
	if err != nil {
		return fmt.Errorf("construct read-only GitHub issues client: %w", err)
	}
	box, err := openGHIntakeBox(ctx, summa42runtime.Config{
		StatePath:        cfg.DatabasePath,
		EvidencePath:     cfg.EvidencePath,
		CollectiveID:     cfg.CollectiveID,
		OwnerPrincipalID: cfg.OwnerPrincipalID,
		PolicyEngine:     material.policyEngine,
	})
	if err != nil {
		return fmt.Errorf("open read-only Box runtime: %w", err)
	}
	defer box.Close()
	cases := workflowcase.New(box.Store, box.Clock, box.Purpose)
	return ghissue.Run(ctx, client, cases, box.Execution, box.Evidence, observerCfg, pollInterval)
}
```

Add `"github.com/SofiaFlux/summa42/internal/ghissue"` to the import block of `cmd/summa42-box/main.go` in sorted position (after `internal/fieldfeedback`, before `internal/localconfig`), then run `gofmt -w cmd/summa42-box/main.go`.

- [ ] **Step 6: Run focused tests to verify they pass**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box ./internal/scheduler ./internal/ghissue -count=1 -v`
Expected: PASS.

- [ ] **Step 7: Run the full validation matrix (CONTRIBUTING.md)**

Run each; all must pass:
```bash
GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1
GOCACHE=/tmp/summa42-full-go-cache go test -race ./... -count=1
GOCACHE=/tmp/summa42-full-go-cache go vet ./...
GOCACHE=/tmp/summa42-full-go-cache go build ./cmd/summa42 ./cmd/summa42-box
python3 docs/superpowers/research/spikes/2026-09-14-sqlite-persistence-spike.py
```
Expected: all exit 0. If the sqlite spike cannot run in this environment, report it explicitly instead of skipping silently.

On Linux with Docker available, also run the OCI bypass/capability acceptance profile from `CONTRIBUTING.md` — that gate must not be replaced with a unit-test-only approximation. If Docker is unavailable, report the skipped profile explicitly in the final report.

- [ ] **Step 8: Commit**

```bash
git status --porcelain
git add cmd/summa42-box/main.go cmd/summa42-box/gh_intake_test.go internal/scheduler/ghissue_intake_test.go
git commit -m "feat: add run-gh-intake command"
```

(Stage only the files `git status` shows as modified/created; never `git add .`.)

## Self-review

- Spec coverage: client contract (env, GET-only surface, Link errors, HTTPS/size/timeout/sanitization, unparseable split) → Task 2; parse/filter/snapshot/classifier/payload/collector → Task 1 and Task 4; atomic case+Task with rollback, ADO migration → Task 3; Find-first flow, state skip, error matrix, Run semantics, cross-repository identity, UTC normalization → Task 4; CLI flags/startup validation/repo precedence/read-only composition/eligibility → Task 5. Every acceptance test listed in the spec has a named test.
- Critical constraints addressed: single `Config` name (client `Config`, observer `ObserveConfig` — spec amended to match); no nested SQLite transactions (one `store.WithTx` + new `CreateTaskWithGuardInTx`); `ensure-failed` is an exclusion, never `Failed`; ADO miss path migrated; `ExcludedIssue` defined once in `results.go`; `Issue.IsPullRequest` carries the wire flag through filtering; pagination state lives in the collector, keeping the client stateless.
- No placeholders: every step names exact files, complete code, and commands. The single "reality note" (Task 3 fixture store type) names the exact type and import to use.
- Type consistency: all cross-task signatures (`ObserveConfig`, `Config`, `ListIssues`, `Name`, `IssueLister`, `Issue`, `ExcludedIssue`, `FailedIssue`, `ObserveResult`, `ObserveOnce`, `Run`, `EnsureAndMaterialize`, `CreateTaskWithGuardInTx`) are identical everywhere they appear; reason tokens, snapshot keys, payload keys, objective and acceptance strings match the spec verbatim.
