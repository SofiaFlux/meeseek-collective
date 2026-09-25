package adoreview

import (
	"context"
	"sync"
	"testing"
)

type fakeCaller struct {
	mu    sync.Mutex
	calls []fakeCall
	pages []any
	err   error
}

type fakeCall struct {
	capability string
	request    any
}

func (f *fakeCaller) Call(_ context.Context, capability string, request any) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{capability, request})
	if f.err != nil {
		return nil, f.err
	}
	if len(f.pages) == 0 {
		return map[string]any{"prs": []any{}}, nil
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

// Calls snapshots the recorded calls so a poll goroutine and the test never share the slice.
func (f *fakeCaller) Calls() []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeCall(nil), f.calls...)
}

func (f *fakeCaller) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeCaller) SetPages(pages []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages = pages
}

func (f *fakeCaller) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func TestListPRsUsesOrgScopeByDefault(t *testing.T) {
	caller := &fakeCaller{pages: []any{map[string]any{"prs": []any{}}}}
	prs, bad, err := ListPRs(context.Background(), caller, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 0 || len(bad) != 0 {
		t.Fatalf("prs = %v bad = %v, want both empty", prs, bad)
	}
	calls := caller.Calls()
	if len(calls) != 1 || calls[0].capability != "ado.pr.org_active" {
		t.Fatalf("calls = %v, want one ado.pr.org_active", calls)
	}
	if req, ok := calls[0].request.(map[string]any); ok {
		if _, hasAction := req["action"]; hasAction {
			t.Fatalf("org_active request carries action: %#v", calls[0].request)
		}
	}
}

func TestListPRsPagesProjectScope(t *testing.T) {
	page := func(n int, token string) map[string]any {
		return map[string]any{
			"prs": []any{map[string]any{
				"repository": "shop", "number": float64(n), "sourceCommit": "abc", "targetCommit": "def",
			}},
			"continuationToken": token,
		}
	}
	caller := &fakeCaller{pages: []any{page(1, "t1"), page(2, ""), map[string]any{"prs": []any{}}}}
	prs, bad, err := ListPRs(context.Background(), caller, "proj", "shop")
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("bad = %v, want empty", bad)
	}
	if len(prs) != 2 || prs[0].Number != 1 || prs[1].Number != 2 {
		t.Fatalf("prs = %+v, want numbers 1,2", prs)
	}
	calls := caller.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2 pages", len(calls))
	}
	for _, call := range calls {
		if call.capability != "ado.pr.list" {
			t.Fatalf("call = %v, want ado.pr.list", call)
		}
		args := call.request.(map[string]any)
		if args["action"] != "list" {
			t.Fatalf("action = %#v, want list", args)
		}
	}
}

func TestFilterPRsExcludesWithReasons(t *testing.T) {
	prs := []PullRequest{
		{Repository: "shop", Number: 1, SourceCommit: "a", TargetCommit: "b", IsDraft: true, AuthorID: "u9", Reviewers: []Reviewer{{ID: "me"}}},
		{Repository: "shop", Number: 2, SourceCommit: "a", TargetCommit: "b", AuthorID: "me", Reviewers: []Reviewer{{ID: "me"}}},
		{Repository: "shop", Number: 3, SourceCommit: "a", TargetCommit: "b", AuthorID: "u9", Reviewers: []Reviewer{{ID: "g-eng", IsGroup: true}}},
		{Repository: "shop", Number: 4, SourceCommit: "a", TargetCommit: "b", AuthorID: "u9"},
		{Repository: "shop", Number: 5, SourceCommit: "a", TargetCommit: "b", AuthorID: "u9", Reviewers: []Reviewer{{ID: "me"}}},
	}
	kept, excluded := FilterPRs(prs, "me")
	if len(kept) != 1 || kept[0].Number != 5 {
		t.Fatalf("kept = %+v, want PR 5", kept)
	}
	want := map[int]string{1: "draft", 2: "self-authored", 3: "group-only-assignment", 4: "cannot-confirm-direct-assignment"}
	if len(excluded) != len(want) {
		t.Fatalf("excluded = %+v, want %d", excluded, len(want))
	}
	for _, e := range excluded {
		if want[int(e.PR.Number)] != e.Reason {
			t.Fatalf("excluded %+v, want %v", e, want)
		}
	}
}

func TestParsePRRequiresIdentityAndCommits(t *testing.T) {
	if _, err := ParsePR(map[string]any{"repository": "shop"}); err == nil {
		t.Fatal("accepted PR without number and commits")
	}
	pr, err := ParsePR(map[string]any{
		"repository": "shop", "number": float64(7), "title": "Fix",
		"sourceCommit": "abc", "targetCommit": "def",
		"author": "u1", "reviewers": []any{map[string]any{"id": "me"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Repository != "shop" || pr.Number != 7 || pr.SourceCommit != "abc" || pr.TargetCommit != "def" {
		t.Fatalf("pr = %+v", pr)
	}
}

func TestListPRsCollectsUnparseable(t *testing.T) {
	caller := &fakeCaller{pages: []any{map[string]any{"prs": []any{
		map[string]any{"repository": "shop", "number": float64(1), "sourceCommit": "a", "targetCommit": "b"},
		map[string]any{"repository": "shop"},
	}}}}
	prs, bad, err := ListPRs(context.Background(), caller, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || len(bad) != 1 {
		t.Fatalf("prs = %v bad = %v, want 1 and 1", prs, bad)
	}
}
