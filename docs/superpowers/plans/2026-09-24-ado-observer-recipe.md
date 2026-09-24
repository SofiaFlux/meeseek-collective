# ADO Observer and Review Work 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Discover reviewer-assigned ADO PRs each tick and materialize one durable review Task per PR revision with exclusions stated per PR.

**Architecture:** New `internal/adoreview` package: `source.go` lists/parses/filters PRs through an injected `PRCaller` seam; `observe.go` reuses-or-creates cases via a new `workflowcase.Find` lookup and materializes Work 1 Tasks. `Run` ticks `ObserveOnce`; `run-observer` CLI mirrors `runWorker` wiring.

**Tech Stack:** Go 1.27, existing `workflowcase`/`execution`/`evidence` services, `testutil` store/clock, stdlib `flag`.

## Global Constraints

- No `operations.Service` dispatches; the observer only reads ADO and writes evidence/cases/Tasks.
- Never claim unattended lease: acceptance is `ELIGIBLE` with valid envelope, not leased execution.
- Full capability names (`ado.pr.*`); `ado.pr.org_active` never takes an `action` key.
- TDD: failing test first for every behavior, then minimal implementation.
- `go test` package suite before each task commit; full `go test ./... -count=1` and `go vet ./...` in Task 3 before the final commit.

---

## File structure

- Create `internal/adoreview/source.go`: `Reviewer`, `PullRequest`, `PRCaller`, `ListPRs`, `FilterPRs`, exclusion reasons. Pure ADO-shape logic, no storage.
- Create `internal/adoreview/source_test.go`: fake caller, parse/filter/paging tests.
- Modify `internal/workflowcase/service.go`: add `Find`; add `find_test.go` or extend `service_test.go` (follow the existing test file pattern).
- Create `internal/adoreview/observe.go`: `Config`, `ExcludedPR`, `FailedPR`, `ObserveResult`, `ObserveOnce`, `Run`.
- Create `internal/adoreview/observe_test.go`: full stack via `testutil` + fake caller.
- Modify `cmd/summa42-box/main.go`: `run-observer` dispatch, flags, wiring (control-plane `run()` untouched).

### Task 1: PR source — list, parse, filter

**Files:**
- Create: `internal/adoreview/source.go`
- Create: `internal/adoreview/source_test.go`

**Interfaces:**
- Consumes: `adomcp` capability names (no import — caller injected).
- Produces: `PRCaller`, `PullRequest`, `Reviewer`, `Unparseable`, `ExcludedPR`, `ListPRs`, `ParsePR`, `FilterPRs`, `Reason*` constants used by Task 2.

- [ ] **Step 1: Write the failing tests.** In `source_test.go` (`package adoreview`):

```go
type fakeCaller struct {
    calls []fakeCall
    pages []any
    err   error
}

type fakeCall struct {
    capability string
    request    any
}

func (f *fakeCaller) Call(_ context.Context, capability string, request any) (any, error) {
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
```

`ListPRs` returns parsed PRs plus unparseable raw items so the observer can
exclude them visibly:

```go
type Unparseable struct {
    Raw any
}
```

Tests:

```go
func TestListPRsUsesOrgScopeByDefault(t *testing.T) {
    caller := &fakeCaller{pages: []any{map[string]any{"prs": []any{}}}}
    prs, bad, err := ListPRs(context.Background(), caller, "", "")
    if err != nil {
        t.Fatal(err)
    }
    if len(prs) != 0 || len(bad) != 0 {
        t.Fatalf("prs = %v bad = %v, want both empty", prs, bad)
    }
    if len(caller.calls) != 1 || caller.calls[0].capability != "ado.pr.org_active" {
        t.Fatalf("calls = %v, want one ado.pr.org_active", caller.calls)
    }
    if args, ok := caller.calls[0].request.(map[string]any); ok {
        if _, hasAction := args["action"]; hasAction {
            t.Fatalf("org_active request carries action: %#v", caller.calls[0].request)
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
    if len(caller.calls) != 2 {
        t.Fatalf("calls = %d, want 2 pages", len(caller.calls))
    }
    for _, call := range caller.calls {
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
        if want[e.PR.Number] != e.Reason {
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
```

`context` import required.

```go
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
```

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -count=1`. Expected: FAIL (undefined `ListPRs`, `PullRequest`, `FilterPRs`, `ParsePR`, `Unparseable`).
- [ ] **Step 3: Implement `source.go`.**

```go
// Package adoreview observes reviewer-assigned Azure DevOps pull requests and
// turns new revisions into durable review Tasks. It performs no external writes.
package adoreview

import (
    "context"
    "errors"
    "fmt"
    "strings"
)

const (
    ReasonDraft                 = "draft"
    ReasonSelfAuthored          = "self-authored"
    ReasonGroupOnly             = "group-only-assignment"
    ReasonUnconfirmedAssignment = "cannot-confirm-direct-assignment"
    ReasonUnparseable           = "unparseable-pr"
)

type PRCaller interface {
    Call(ctx context.Context, capability string, request any) (any, error)
}

type Reviewer struct {
    ID      string
    IsGroup bool
}

type PullRequest struct {
    Repository   string
    Number       int64
    Title        string
    IsDraft      bool
    AuthorID     string
    Reviewers    []Reviewer
    SourceCommit string
    TargetCommit string
}

func (p PullRequest) ObjectID() string { return p.Repository + "#" + fmt.Sprint(p.Number) }
func (p PullRequest) RevisionID() string { return p.SourceCommit + ":" + p.TargetCommit }

func ListPRs(ctx context.Context, caller PRCaller, project, repository string) ([]PullRequest, []Unparseable, error) {
    if caller == nil {
        return nil, nil, errors.New("PR caller is required")
    }
    var out []PullRequest
    var bad []Unparseable
    token := ""
    for {
        items, next, err := listPage(ctx, caller, project, repository, token)
        if err != nil {
            return nil, nil, err
        }
        for _, item := range items {
            pr, err := ParsePR(item)
            if err != nil {
                bad = append(bad, Unparseable{Raw: item})
                continue
            }
            out = append(out, pr)
        }
        if len(items) == 0 || next == "" {
            return out, bad, nil
        }
        token = next
    }
}

func listPage(ctx context.Context, caller PRCaller, project, repository, token string) ([]any, string, error) {
    var capability string
    args := map[string]any{}
    if project != "" || repository != "" {
        capability = "ado.pr.list"
        args["action"] = "list"
        if project != "" {
            args["project"] = project
        }
        if repository != "" {
            args["repository"] = repository
        }
    } else {
        capability = "ado.pr.org_active"
    }
    if token != "" {
        args["continuationToken"] = token
    }
    var request any
    if len(args) > 0 {
        request = args
    }
    raw, err := caller.Call(ctx, capability, request)
    if err != nil {
        return nil, "", err
    }
    return pageItems(raw), pageToken(raw), nil
}

func pageItems(raw any) []any {
    m, ok := raw.(map[string]any)
    if !ok {
        return nil
    }
    for _, key := range []string{"prs", "value"} {
        if items, ok := m[key].([]any); ok {
            return items
        }
    }
    return nil
}

func pageToken(raw any) string {
    m, ok := raw.(map[string]any)
    if !ok {
        return ""
    }
    for _, key := range []string{"continuationToken", "nextPageToken"} {
        if token, ok := m[key].(string); ok && strings.TrimSpace(token) != "" {
            return token
        }
    }
    return ""
}

func ParsePR(item any) (PullRequest, error) {
    m, ok := item.(map[string]any)
    if !ok {
        return PullRequest{}, errors.New("PR must be an object")
    }
    var pr PullRequest
    repo, _ := m["repository"].(string)
    if strings.TrimSpace(repo) == "" {
        return PullRequest{}, errors.New("PR repository is required")
    }
    pr.Repository = repo
    switch number := m["number"].(type) {
    case float64:
        pr.Number = int64(number)
    case int64:
        pr.Number = number
    case int:
        pr.Number = int64(number)
    case json.Number:
        n, err := number.Int64()
        if err != nil {
            return PullRequest{}, fmt.Errorf("PR number is invalid: %v", m["number"])
        }
        pr.Number = n
    default:
        return PullRequest{}, errors.New("PR number is required")
    }
    if pr.Number <= 0 {
        return PullRequest{}, errors.New("PR number is required")
    }
    source, _ := m["sourceCommit"].(string)
    if strings.TrimSpace(source) == "" {
        return PullRequest{}, errors.New("PR sourceCommit is required")
    }
    target, _ := m["targetCommit"].(string)
    if strings.TrimSpace(target) == "" {
        return PullRequest{}, errors.New("PR targetCommit is required")
    }
    pr.SourceCommit, pr.TargetCommit = source, target
    pr.Title, _ = m["title"].(string)
    if draft, ok := m["isDraft"].(bool); ok {
        pr.IsDraft = draft
    } else if draft, ok := m["draft"].(bool); ok {
        pr.IsDraft = draft
    }
    switch author := m["author"].(type) {
    case string:
        pr.AuthorID = author
    case map[string]any:
        pr.AuthorID, _ = author["id"].(string)
    }
    if reviewers, ok := m["reviewers"].([]any); ok {
        for _, r := range reviewers {
            switch reviewer := r.(type) {
            case string:
                pr.Reviewers = append(pr.Reviewers, Reviewer{ID: reviewer})
            case map[string]any:
                id, _ := reviewer["id"].(string)
                isGroup, _ := reviewer["isGroup"].(bool)
                if !isGroup {
                    isGroup, _ = reviewer["group"].(bool)
                }
                if strings.TrimSpace(id) != "" {
                    pr.Reviewers = append(pr.Reviewers, Reviewer{ID: id, IsGroup: isGroup})
                }
            }
        }
    }
    return pr, nil
}

func FilterPRs(prs []PullRequest, reviewerID string) (kept []PullRequest, excluded []ExcludedPR) {
    for _, pr := range prs {
        switch {
        case pr.IsDraft:
            excluded = append(excluded, ExcludedPR{pr, ReasonDraft})
        case pr.AuthorID != "" && pr.AuthorID == reviewerID:
            excluded = append(excluded, ExcludedPR{pr, ReasonSelfAuthored})
        case hasDirectReviewer(pr, reviewerID):
            kept = append(kept, pr)
        case hasGroupReviewer(pr, reviewerID):
            excluded = append(excluded, ExcludedPR{pr, ReasonGroupOnly})
        default:
            excluded = append(excluded, ExcludedPR{pr, ReasonUnconfirmedAssignment})
        }
    }
    return kept, excluded
}

func hasDirectReviewer(pr PullRequest, reviewerID string) bool {
    for _, r := range pr.Reviewers {
        if !r.IsGroup && r.ID == reviewerID {
            return true
        }
    }
    return false
}

func hasGroupReviewer(pr PullRequest, _ string) bool {
    for _, r := range pr.Reviewers {
        if r.IsGroup {
            return true
        }
    }
    return false
}

type ExcludedPR struct {
    PR     PullRequest
    Reason string
}
```

`encoding/json` and `strings` imports required in `source.go`.

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git add internal/adoreview/source.go internal/adoreview/source_test.go && git commit -m "feat: add ADO PR listing, parsing and filter"`.

### Task 2: Find, ObserveOnce, materialize

**Files:**
- Modify: `internal/workflowcase/service.go` (add `Find`)
- Modify: `internal/workflowcase/service_test.go` or create `find_test.go` (follow existing pattern; check imports in the file first)
- Create: `internal/adoreview/observe.go`
- Create: `internal/adoreview/observe_test.go`

**Interfaces:**
- Consumes: `ListPRs`, `FilterPRs`, `ParsePR`, `PRCaller`, `PullRequest` from Task 1; `workflowcase.Ensure/Observation/Case`, `MaterializeTask`, `execution.TaskRequest`, `evidence.Put/Metadata`.
- Produces: `Config`, `ObserveResult`, `ObserveOnce`, `workflowcase.Find` used by Task 3.

- [ ] **Step 1: Write the failing tests.** `workflowcase` Find test (in the existing test package/style):

```go
func TestFindReturnsCaseByIdentity(t *testing.T) {
    // build stack like existing service tests: testutil.OpenStore, testutil.NewClock, purpose.New, workflowcase.New
    // create mission via purposes.CreateMission, then svc.Ensure with a minimal valid Observation
    // (Source "ado", ObjectID "shop#1", RevisionID "a:b", EvidenceID "ev-1",
    //  FirstWork workflow.WorkProposal{Kind: "ado.pr.review", RequiredCapabilities: []string{"read"}, AuthorityCeiling: []string{"read"}},
    //  Grant workflow.Grant{Capabilities: []string{"read"}}, MaxSteps 3, RemainingBudget 10)
    // found, ok, err := svc.Find(ctx, missionID, "ado", "shop#1", "a:b")
    // assert ok, found.ID == created.ID
    // missing, ok, err := svc.Find(ctx, missionID, "ado", "shop#1", "other")
    // assert !ok and missing.ID == ""
}
```

Check the existing service test setup names before writing (store/clock/purpose constructor calls mirror scheduler tests). `ObserveOnce` tests in `observe_test.go` (`package adoreview`):

```go
func TestObserveOnceMaterializesNewRevision(t *testing.T) {
    // stack: testutil store/clock, purpose.New, execution.New, resources not needed,
    // evidence.New(store, t.TempDir(), clk), verification not needed,
    // cases := workflowcase.New(store, clk, purposes)
    // mission via purposes.CreateMission(ctx, "ado review")
    // envelope via INSERT INTO resource_envelopes (copy scheduler test pattern, limit 100)
    // fake caller page: one PR {repository shop, number 1, sourceCommit a, targetCommit b, author u9, reviewers [{id me}]}
    // cfg := adoreview.Config{MissionID: mission, ReviewerID: "me",
    //   Grant: workflow.Grant{Capabilities: []string{"read"}},
    //   ResourceEnvelopeID: envelope, MaxSteps: 3, RemainingBudget: 10}
    // result, err := adoreview.ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg)
    // assert 1 ensured, 1 materialized, 0 excluded, 0 failed
    // task, err := execSvc.Task(ctx, result.Materialized[0]); assert Objective == "Review ADO PR shop#1"
    // assert task.ResourceEnvelopeID == envelope
}
func TestObserveOnceReusesCaseWithoutNewEvidence(t *testing.T) {
    // same fixture; run ObserveOnce twice
    // second result: same case ID in Ensured, 1 materialized (same Task ID — idempotency key = work ID),
    // and evidence_objects row count for kind ado.pr.snapshot unchanged (query count before/after)
}
func TestObserveOnceExcludesAndRepairsPartialCase(t *testing.T) {
    // caller returns one draft PR + one good PR
    // first run with a caller whose good-PR materialization... (partial case needs Ensure-without-Task:
    //  call cases.Ensure directly with the good PR observation shape, then run ObserveOnce)
    // assert draft excluded with reason draft; good PR materialized (repaired) with no new case
}
```

For the partial-case subtest the test must construct the same revision the observer computes (`sourceCommit:targetCommit`) — use commits "a"/"b" and FirstWork Kind "ado.pr.review" so `Find` hits.

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/workflowcase ./internal/adoreview -count=1`. Expected: FAIL (undefined `Find`, `ObserveOnce`, `Config`).
- [ ] **Step 3: Implement `Find`.** In `service.go`:

```go
func (s *Service) Find(ctx context.Context, missionID domain.ID, source, objectID, revisionID string) (Case, bool, error) {
    if s == nil || s.store == nil {
        return Case{}, false, errors.New("workflow case service is not configured")
    }
    row := s.store.DB().QueryRowContext(ctx, `SELECT case_id, mission_id, source, object_id, revision_id,
        observation_evidence_id, state, current_work_id, next_work_json, grant_json,
        completed_steps, max_steps, remaining_budget, progress_signature, initial_request_json
        FROM workflow_cases WHERE mission_id = ? AND source = ? AND object_id = ? AND revision_id = ?`,
        missionID, source, objectID, revisionID)
    c, _, err := scanCase(row)
    if errors.Is(err, sql.ErrNoRows) {
        return Case{}, false, nil
    }
    if err != nil {
        return Case{}, false, fmt.Errorf("find workflow case: %w", err)
    }
    return c, true, nil
}
```

- [ ] **Step 4: Implement `observe.go`.**

```go
package adoreview

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "strings"

    "github.com/SofiaFlux/summa42/internal/domain"
    "github.com/SofiaFlux/summa42/internal/evidence"
    "github.com/SofiaFlux/summa42/internal/execution"
    "github.com/SofiaFlux/summa42/internal/workflow"
    "github.com/SofiaFlux/summa42/internal/workflowcase"
)

type Config struct {
    MissionID          domain.ID
    ReviewerID         string
    Grant              workflow.Grant
    WorkCapabilities   []string
    ResourceEnvelopeID domain.ID
    MaxSteps           int
    RemainingBudget    int64
    Project            string
    Repository         string
}

type ExcludedPR struct {
    PR     PullRequest
    Reason string
}

type FailedPR struct {
    PR  PullRequest
    Err string
}

type ObserveResult struct {
    Ensured      []domain.ID
    Materialized []domain.ID
    Excluded     []ExcludedPR
    Failed       []FailedPR
}

func (c Config) validate() error {
    if strings.TrimSpace(string(c.MissionID)) == "" || strings.TrimSpace(c.ReviewerID) == "" {
        return errors.New("mission and reviewer are required")
    }
    if strings.TrimSpace(string(c.ResourceEnvelopeID)) == "" {
        return errors.New("resource envelope is required")
    }
    if c.MaxSteps <= 0 || c.RemainingBudget <= 0 {
        return errors.New("positive max steps and remaining budget are required")
    }
    return nil
}

func (c Config) workCapabilities() []string {
    if len(c.WorkCapabilities) > 0 {
        return append([]string(nil), c.WorkCapabilities...)
    }
    return append([]string(nil), c.Grant.Capabilities...)
}

func ObserveOnce(ctx context.Context, caller PRCaller, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg Config) (ObserveResult, error) {
    var result ObserveResult
    if err := cfg.validate(); err != nil {
        return result, err
    }
    if caller == nil || cases == nil || execSvc == nil || evidenceStore == nil {
        return result, errors.New("caller, case, execution and evidence services are required")
    }
    prs, bad, err := ListPRs(ctx, caller, cfg.Project, cfg.Repository)
    if err != nil {
        return result, err
    }
    for _, raw := range bad {
        _ = raw
        result.Excluded = append(result.Excluded, ExcludedPR{Reason: ReasonUnparseable})
    }
    kept, excluded := FilterPRs(prs, cfg.ReviewerID)
    result.Excluded = append(result.Excluded, excluded...)
    for _, pr := range kept {
        if err := observeOne(ctx, cases, execSvc, evidenceStore, cfg, pr, &result); err != nil {
            result.Failed = append(result.Failed, FailedPR{PR: pr, Err: err.Error()})
        }
    }
    return result, nil
}

func observeOne(ctx context.Context, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg Config, pr PullRequest, result *ObserveResult) error {
    existing, found, err := cases.Find(ctx, cfg.MissionID, "ado", pr.ObjectID(), pr.RevisionID())
    if err != nil {
        return err
    }
    if found {
        task, err := materialize(ctx, cases, execSvc, cfg, existing, pr)
        if err != nil {
            return err
        }
        result.Ensured = append(result.Ensured, existing.ID)
        result.Materialized = append(result.Materialized, task.ID)
        return nil
    }
    canonical, err := json.Marshal(pr)
    if err != nil {
        return err
    }
    object, err := evidenceStore.Put(ctx, strings.NewReader(string(canonical)), evidence.Metadata{MediaType: "application/json", Kind: "ado.pr.snapshot"})
    if err != nil {
        return err
    }
    workCaps := cfg.workCapabilities()
    created, err := cases.Ensure(ctx, workflowcase.Observation{
        MissionID: cfg.MissionID, Source: "ado", ObjectID: pr.ObjectID(), RevisionID: pr.RevisionID(),
        EvidenceID: string(object.ID),
        FirstWork: workflow.WorkProposal{Kind: "ado.pr.review", RequiredCapabilities: workCaps, AuthorityCeiling: append([]string(nil), cfg.Grant.Capabilities...)},
        Grant: cfg.Grant, MaxSteps: cfg.MaxSteps, RemainingBudget: cfg.RemainingBudget,
    })
    if err != nil {
        return err
    }
    task, err := materialize(ctx, cases, execSvc, cfg, created, pr)
    if err != nil {
        return err
    }
    result.Ensured = append(result.Ensured, created.ID)
    result.Materialized = append(result.Materialized, task.ID)
    return nil
}

func materialize(ctx context.Context, cases *workflowcase.Service, execSvc *execution.Service, cfg Config, c workflowcase.Case, pr PullRequest) (domain.Task, error) {
    payload, err := json.Marshal(map[string]any{
        "repo": pr.Repository, "pr": pr.Number, "sourceCommit": pr.SourceCommit, "targetCommit": pr.TargetCommit,
    })
    if err != nil {
        return domain.Task{}, err
    }
    return cases.MaterializeTask(ctx, execSvc, c.ID, c.CurrentWorkID, execution.TaskRequest{
        Objective:          fmt.Sprintf("Review ADO PR %s", pr.ObjectID()),
        PayloadJSON:        payload,
        AcceptanceCriteria: []string{"review evidence recorded for " + pr.RevisionID()},
        ResourceEnvelopeID: cfg.ResourceEnvelopeID,
    })
}
```

`_ = raw` is a placeholder-ish wart — replace with a comment: unparseable items carry no identity, so only the reason is recorded. Write it as:

```go
    for range bad {
        // Unparseable items carry no repo/number identity; only the reason is recorded.
        result.Excluded = append(result.Excluded, ExcludedPR{Reason: ReasonUnparseable})
    }
```

- [ ] **Step 5: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview ./internal/workflowcase -count=1`. Expected: PASS.
- [ ] **Step 6: Commit.** `git add internal/adoreview/observe.go internal/adoreview/observe_test.go internal/workflowcase/service.go internal/workflowcase/*test*.go && git commit -m "feat: observe ADO PRs into review Tasks"`. (Stage only the touched workflowcase files — check `git status` first; never `git add .`.)

### Task 3: Run loop, run-observer CLI, full verification

**Files:**
- Modify: `internal/adoreview/observe.go` (add `Run`)
- Modify: `internal/adoreview/observe_test.go` (Run tests)
- Modify: `cmd/summa42-box/main.go` (dispatch, flags, wiring; `run()` untouched)

**Interfaces:**
- Consumes: `ObserveOnce`, `Config` from Task 2; `runWorker` wiring pattern in `main.go` (read it first: `os.Args` dispatch, `flag` set, `summa42runtime.Open`, provider assess, skip control server).
- Produces: runnable `run-observer`; nothing depends on it yet.

- [ ] **Step 1: Write the failing tests.** Append Run tests mirroring the worker-loop Run semantics:

```go
func TestRunStopsOnCancelWithoutNewPoll(t *testing.T) {
    // minimal stack (no PRs needed): caller returns empty page
    // ctx cancelled before Run → returns nil, zero caller calls
}
func TestRunPollsThenStops(t *testing.T) {
    // one-PR fixture from Task 2; Run with millisecond interval in goroutine
    // wait until task ELIGIBLE (poll execSvc.Task), cancel, expect nil return
    // caller called at least once; second tick reuses case (same case ID)
}
```

Write full bodies following the Task 2 fixture pattern (do not abbreviate: construct stack, caller, cfg explicitly in each test).

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -run 'TestRun(StopsOnCancelWithoutNewPoll|PollsThenStops)$' -count=1`. Expected: FAIL (undefined `Run`).
- [ ] **Step 3: Implement `Run`.** In `observe.go`:

```go
func (w *Observer) Run(...)
```

No — no Observer struct in this design; `Run` is a plain function mirroring worker semantics:

```go
func Run(ctx context.Context, caller PRCaller, cases *workflowcase.Service, execSvc *execution.Service, evidenceStore *evidence.Store, cfg Config, interval time.Duration) error {
    if interval <= 0 {
        return errors.New("observer requires a positive poll interval")
    }
    if err := ctx.Err(); err != nil {
        return nil
    }
    if _, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg); err != nil {
        return err
    }
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            if err := ctx.Err(); err != nil {
                return nil
            }
            if _, err := ObserveOnce(ctx, caller, cases, execSvc, evidenceStore, cfg); err != nil {
                return err
            }
        }
    }
}
```

Add the `time` import.

- [ ] **Step 4: Wire the CLI.** In `main.go`, mirror the `run-worker` dispatch: `if len(os.Args) > 1 && os.Args[1] == "run-observer"` → `runObserver(ctx, os.Args[2:])`. Implement `parseObserverFlags` with a `flag.FlagSet` (`--mission`, `--reviewer-id`, `--grant-capability`/`--grant-action`/`--work-capability` via a repeatable `stringSlice` flag type, `--envelope`, `--max-steps`, `--remaining-budget`, `--project`, `--repository`, `--poll-interval` default 5 minutes) with the same validation style as `parseWorkerFlags` (required non-empty, positive numbers/interval). `runObserver` mirrors `runWorker`: load config, build provider via `buildADOProviderFromEnv`, open runtime, construct `workflowcase.New` from the box store/clock/purpose services (confirm exact Box field names in `internal/runtime/box.go` before coding: `Store`, `Clock`, `Purpose`, `Execution`, `Evidence`), call `adoreview.Run`. Refuse empty grant capabilities with an explicit error (no silent allow-all).
- [ ] **Step 5: Write flag tests** in `cmd/summa42-box/observer_flags_test.go` (`package main`): defaults, missing required `--mission`, non-positive `--poll-interval`. Mirror `worker_flags_test.go` style (read it first).
- [ ] **Step 6: Run everything.** In order, all must pass:
  - `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview ./internal/workflowcase ./cmd/... -count=1`
  - `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1`
  - `go vet ./...`, `git diff --check`, `gofmt -l` on touched files
- [ ] **Step 7: Commit.** `git add internal/adoreview/observe.go internal/adoreview/observe_test.go cmd/summa42-box/main.go cmd/summa42-box/observer_flags_test.go && git commit -m "feat: add observer run loop and run-observer command"`.

## Self-review

- Spec coverage: data flow (Find-first, evidence metadata, Ensure/Materialize, result quads) → Task 2; template contract (5 overwritten fields, Decide gates, ProposedActions nil) → Task 2 `observeOne`/`materialize`; flags table → Task 3; parser/paging (required fields, canonical blob example, token loop) → Task 1; error handling (tick abort, exclusion vs Failed, no operations) → Tasks 2–3; testing section → all tasks; acceptance (reuse/zero new rows, ELIGIBLE, no effects) → Task 2–3 tests.
- No placeholders: exact files, code, commands, expected outputs; CLI wiring points at exact Box fields to confirm.
- Type consistency: `PullRequest`/`PRCaller`/`ListPRs`/`FilterPRs`/`ParsePR`/`Config`/`ObserveResult`/`ObserveOnce`/`Run`/`Find` signatures identical across tasks; `ExcludedPR`/`FailedPR`/`Unparseable` shapes fixed in Task 1.
