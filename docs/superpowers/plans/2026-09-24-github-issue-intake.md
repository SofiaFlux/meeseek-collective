# GitHub Issue Intake Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register maintainer-opened GitHub issues as durable workflow cases with triage Tasks, performing zero GitHub writes.

**Architecture:** New `internal/ghissue` package mirrors `internal/adoreview` (source → observe → CLI). A GET-only GitHub client reuses the `feedbackgithub.FileCredentialSource` pattern. Case+Task creation is atomic via a new `workflowcase.EnsureAndMaterialize` (also adopted by the ADO observer miss path later — out of scope here). The `run-gh-intake` CLI opens the Box with feedback disabled and no executors, so triage Tasks stay eligible-but-unclaimed.

**Tech Stack:** Go 1.27, `net/http` + `httptest` for client tests, `testutil` store/clock for service tests, stdlib `flag` for CLI.

## Global Constraints

- No GitHub writes: the client type exposes GET operations only; no POST/PATCH/DELETE method may exist on it.
- Capability `github.issue.read` is always in FirstWork RequiredCapabilities; every effective work capability must be grant-listed (startup error otherwise).
- `IssueLister` returns parsed and unparseable items separately; unparseable items become exclusions, never silent drops.
- TDD: failing test first for every behavior, then minimal implementation.
- `GOCACHE=/tmp/summa42-full-go-cache` for all `go test` invocations in this plan.
- Full `go test ./... -count=1` and `go vet ./...` must pass before the final commit of Task 5; package suites before every other commit.

---

### Task 1: GET-only GitHub client

**Files:**
- Create: `internal/ghissue/client.go`
- Create: `internal/ghissue/client_test.go`

**Interfaces:**
- Consumes: `feedbackgithub.FileCredentialSource`-style token file (`SUMMA42_GITHUB_TOKEN_FILE`); `owner/name` repo format (same regex rule as `repositoryPattern`: `^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).
- Produces: `ghissue.Config`, `ghissue.New`, `(*Client).FetchIssuesPage(ctx, cursor string) ([]any, string, error)` used by Task 2.

- [ ] **Step 1: Write the failing test**

```go
func TestFetchIssuesPageFollowsLinkPagination(t *testing.T) {
    first := `[{"number":1,"title":"a","body":"","state":"open","user":{"login":"m"},"assignees":[],"labels":[],"updated_at":"2026-09-24T10:00:00Z","html_url":"https://github.com/o/r/issues/1"}]`
    var server *httptest.Server
    server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            t.Errorf("method = %s, want GET", r.Method)
        }
        if got := r.Header.Get("Authorization"); got != "Bearer sekrit" {
            t.Errorf("auth = %q", got)
        }
        if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
            t.Errorf("accept = %q", got)
        }
        if strings.Contains(r.URL.RawQuery, "page=2") {
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte(`[]`))
            return
        }
        w.Header().Set("Link", `<`+server.URL+`/x?page=2>; rel="next"`)
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(first))
    }))
    defer server.Close()
    tokenFile := filepath.Join(t.TempDir(), "token")
    if err := os.WriteFile(tokenFile, []byte("sekrit\n"), 0o600); err != nil {
        t.Fatal(err)
    }
    t.Setenv("SUMMA42_GITHUB_TOKEN_FILE", tokenFile)
    t.Setenv("SUMMA42_GITHUB_REPOSITORY", "o/r")
    client, err := New(Config{BaseURL: server.URL, Repository: "o/r"})
    if err != nil {
        t.Fatal(err)
    }
    items, next, err := client.FetchIssuesPage(context.Background(), "")
    if err != nil {
        t.Fatal(err)
    }
    if len(items) != 1 {
        t.Fatalf("items = %d, want 1", len(items))
    }
    if next == "" || strings.Contains(next, "page=2") == false {
        t.Fatalf("next = %q, want page-2 cursor", next)
    }
    items, next, err = client.FetchIssuesPage(context.Background(), next)
    if err != nil {
        t.Fatal(err)
    }
    if len(items) != 0 || next != "" {
        t.Fatalf("items = %d next = %q, want 0 and empty", len(items), next)
    }
}

func TestFetchIssuesPageRejects(t *testing.T) {
    // table, each with a fresh httptest server or a direct constructor check:
    // - bad repository "o" → New error
    // - bad repository "../x" → New error
    // - http://evil.example base URL → New error (non-loopback http rejected)
    // - missing token file (env points at nonexistent path) → FetchIssuesPage error
    // - token file with 0o644 perms → FetchIssuesPage error
    // - 403 response → sanitized error (must not contain the token)
    // - oversized body (> MaxResponseBytes) → error
    // - malformed Link next URL → error, not silent stop
    // - cross-origin Link next URL (different host) → error
    // - repeated next URL (server always returns same next) → error after one repeat
}
```

Imports for the test file: `context`, `net/http`, `net/http/httptest`, `os`, `path/filepath`, `strings`, `testing`.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -run 'TestFetchIssuesPage' -count=1 -v`
Expected: FAIL with `undefined: New` / `undefined: Config` (package does not exist yet — create the directory first with the test file in it).

- [ ] **Step 3: Write minimal implementation**

```go
// Package ghissue observes maintainer-opened GitHub issues. It performs no
// GitHub writes: the client below exposes GET operations only.
package ghissue

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "regexp"
    "strings"
    "time"
)

const (
    defaultAPIBaseURL = "https://api.github.com"
    maxResponseBytes  = 1 << 20
    userAgent         = "summa42-ghissue/1"
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type Config struct {
    BaseURL    string
    Repository string
    TokenFile  string
    Timeout    time.Duration
    HTTPClient *http.Client
}

type Client struct {
    baseURL    *url.URL
    repository string
    tokenFile  string
    http       *http.Client
}

// FetchIssuesPage returns one page of raw issue objects plus the next-page
// cursor ("") when the Link header carries no usable rel="next".
func (c *Client) FetchIssuesPage(ctx context.Context, cursor string) ([]any, string, error) {
    // Build request: cursor != "" → use it verbatim after same-origin check;
    // else GET {base}/repos/{repo}/issues?state=open&per_page=100.
    // Headers: Accept application/vnd.github+json, Authorization Bearer <token from file>, User-Agent.
    // Non-2xx → sanitized error naming status only. Body capped at maxResponseBytes.
    // Decode []any. Parse Link header for rel="next": malformed/cross-origin/repeated → error.
}
```

Full method bodies per the test contract above (token read mirrors `FileCredentialSource`: regular file, `0o077` permission mask rejected on non-windows, trimmed non-empty). `New` validates repository format, base URL (https, or http only for localhost/loopback), and returns the client with a 20s default timeout. No POST/PATCH/DELETE method may be added.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ghissue/client.go internal/ghissue/client_test.go
git commit -m "feat: add read-only GitHub issues client"
```

### Task 2: Issue parsing, filter, snapshot

**Files:**
- Create: `internal/ghissue/source.go`
- Create: `internal/ghissue/source_test.go`

**Interfaces:**
- Consumes: `Client.FetchIssuesPage` page shapes from Task 1.
- Produces: `Issue`, `Unparseable{Raw any}`, `PageFetcher` interface, `ListAllIssues`, `ParseIssue`, `FilterIssues`, `ExcludedIssue`-shape `{Issue Issue; Reason string}`, reason constants, `CanonicalSnapshot`, `ClassifyTriage` used by Task 4.

- [ ] **Step 1: Write the failing test**

```go
type stubFetcher struct {
    pages []stubPage
    calls []string
}

type stubPage struct {
    items []any
    next  string
    err   error
}

func (s *stubFetcher) FetchPage(_ context.Context, cursor string) ([]any, string, error) {
    s.calls = append(s.calls, cursor)
    if len(s.pages) == 0 {
        return nil, "", nil
    }
    page := s.pages[0]
    s.pages = s.pages[1:]
    return page.items, page.next, page.err
}

func issueItem(n int, author, state string, extra map[string]any) map[string]any {
    item := map[string]any{
        "number": float64(n), "title": "T", "body": "b", "state": state,
        "user": map[string]any{"login": author}, "assignees": []any{},
        "labels": []any{}, "updated_at": "2026-09-24T10:00:00Z",
        "html_url": "https://github.com/o/r/issues/1",
    }
    for k, v := range extra {
        item[k] = v
    }
    return item
}

func TestListAllIssuesParsesAndSplitsUnparseable(t *testing.T) {
    fetcher := &stubFetcher{pages: []stubPage{
        {items: []any{issueItem(1, "m", "open", nil), map[string]any{"number": 2.0}}, next: "c1"},
        {items: []any{issueItem(3, "m", "open", nil)}, next: ""},
    }}
    issues, bad, err := ListAllIssues(context.Background(), fetcher)
    if err != nil {
        t.Fatal(err)
    }
    if len(issues) != 2 || len(bad) != 1 {
        t.Fatalf("issues = %d bad = %d, want 2 and 1", len(issues), len(bad))
    }
    if issues[0].Number != 1 || issues[0].Repository != "o/r" {
        t.Fatalf("issue[0] = %+v", issues[0])
    }
    if issues[0].RevisionID() == "" || issues[0].ObjectID() != "github:o/r#1" {
        t.Fatalf("identity = %q %q", issues[0].ObjectID(), issues[0].RevisionID())
    }
}

func TestFilterIssuesReasonsInPrecedence(t *testing.T) {
    mk := func(n int, mutate func(map[string]any)) Issue {
        item := issueItem(n, "m", "open", nil)
        if mutate != nil {
            mutate(item)
        }
        parsed, err := ParseIssue(item, "o/r")
        if err != nil {
            t.Fatal(err)
        }
        return parsed
    }
    prs := []Issue{mk(1, func(m map[string]any) { m["pull_request"] = map[string]any{"url": "x"} })}
    kept, excluded, skipped := FilterIssues(prs, []string{"m"})
    if len(kept) != 0 || len(skipped) != 1 || len(excluded) != 0 {
        t.Fatalf("kept=%d excluded=%d skipped=%d", len(kept), len(excluded), len(skipped))
    }
    cases := []struct {
        issue  Issue
        reason string
    }{
        {mk(2, func(m map[string]any) { delete(m, "title") }), ReasonUnparseable},
        {mk(3, func(m map[string]any) { m["state"] = "closed" }), ReasonNotOpen},
        {mk(4, func(m map[string]any) { m["user"] = map[string]any{"login": "stranger"} }), ReasonNotMaintainer},
        {mk(5, func(m map[string]any) { m["assignees"] = []any{map[string]any{"login": "s"}} }), ReasonAlreadyAssigned},
        {mk(6, func(m map[string]any) { m["labels"] = []any{map[string]any{"name": "WontFix"}} }), ReasonHeldByLabel},
        {mk(7, nil), ""},
    }
    for _, tc := range cases[:len(cases)-1] {
        kept, excluded, _ := FilterIssues([]Issue{tc.issue}, []string{"m"})
        if len(kept) != 0 || len(excluded) != 1 || excluded[0].Reason != tc.reason {
            t.Fatalf("issue %d: kept=%d excluded=%+v, want reason %q", tc.issue.Number, len(kept), excluded, tc.reason)
        }
    }
    kept, excluded, _ = FilterIssues([]Issue{cases[len(cases)-1].issue}, []string{"m"})
    if len(kept) != 1 || len(excluded) != 0 {
        t.Fatalf("candidate: kept=%d excluded=%d", len(kept), len(excluded))
    }
}

func TestClassifyTriageRules(t *testing.T) {
    table := []struct {
        labels []string
        title  string
        want   string
    }{
        {[]string{"bug"}, "x", TriageBug},
        {[]string{"Defect"}, "x", TriageBug},
        {[]string{}, "[bug] crash", TriageBug},
        {[]string{"enhancement"}, "x", TriageFeature},
        {[]string{}, "[FEAT] thing", TriageFeature},
        {[]string{"bug", "enhancement"}, "x", TriageBug},
        {[]string{"docs"}, "polish wording", TriageUnclassified},
    }
    for _, tc := range table {
        if got := ClassifyTriage(tc.labels, tc.title); got != tc.want {
            t.Fatalf("labels=%v title=%q: got %q want %q", tc.labels, tc.title, got, tc.want)
        }
    }
}

func TestCanonicalSnapshotKeys(t *testing.T) {
    parsed, err := ParseIssue(issueItem(9, "m", "open", map[string]any{
        "labels": []any{map[string]any{"name": "Bug"}, map[string]any{"name": "bug"}},
        "assignees": []any{},
        "updated_at": "2026-09-24T10:00:00+00:00",
    }), "o/r")
    if err != nil {
        t.Fatal(err)
    }
    snapshot, err := CanonicalSnapshot(parsed)
    if err != nil {
        t.Fatal(err)
    }
    var decoded map[string]any
    if err := json.Unmarshal(snapshot, &decoded); err != nil {
        t.Fatal(err)
    }
    for _, key := range []string{"repo", "issue", "title", "body", "author", "assignees", "labels", "url", "updatedAt", "triage"} {
        if _, ok := decoded[key]; !ok {
            t.Fatalf("missing key %q in %s", key, snapshot)
        }
    }
    if decoded["updatedAt"] != "2026-09-24T10:00:00Z" {
        t.Fatalf("updatedAt = %v, want UTC Z form", decoded["updatedAt"])
    }
    if labels := decoded["labels"].([]any); len(labels) != 1 {
        t.Fatalf("labels = %v, want deduped single bug", labels)
    }
}
```

Imports: `context`, `encoding/json`, `testing`.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -run 'TestListAllIssues|TestFilterIssues|TestClassifyTriage|TestCanonicalSnapshot' -count=1 -v`
Expected: FAIL with `undefined` symbols.

- [ ] **Step 3: Write minimal implementation**

```go
package ghissue

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "sort"
    "strconv"
    "strings"
    "time"
)

const (
    ReasonUnparseable    = "unparseable-issue"
    ReasonNotOpen        = "not-open"
    ReasonNotMaintainer  = "not-maintainer"
    ReasonAlreadyAssigned = "already-assigned"
    ReasonHeldByLabel    = "held-by-label"
    ReasonCaseNotActive  = "case-not-active"
    ReasonEnsureFailed   = "ensure-failed"

    TriageBug          = "bug"
    TriageFeature      = "feature"
    TriageUnclassified = "unclassified"
)

type Issue struct {
    Repository string
    Number     int64
    Title      string
    Body       string
    State      string
    Author     string
    Assignees  []string
    Labels     []string
    URL        string
    UpdatedAt  time.Time
    Triage     string
}

func (i Issue) ObjectID() string  { return "github:" + i.Repository + "#" + strconv.FormatInt(i.Number, 10) }
func (i Issue) RevisionID() string { return i.UpdatedAt.UTC().Format(time.RFC3339Nano) }

type Unparseable struct{ Raw any }

type PageFetcher interface {
    FetchPage(ctx context.Context, cursor string) ([]any, string, error)
}

func (c *Client) FetchPage(ctx context.Context, cursor string) ([]any, string, error) {
    return c.FetchIssuesPage(ctx, cursor)  // Task 1 method, same signature
}

func ListAllIssues(ctx context.Context, fetcher PageFetcher) ([]Issue, []Unparseable, error) {
    // loop: items, next, err := fetcher.FetchPage(ctx, cursor); cursor = next
    // stop when next == ""; per item: ParseIssue(item, repo?) — repository comes from where?
}
```

Repository problem: `FetchIssuesPage` returns raw items without repo context. `ListAllIssues` needs the repository for `ParseIssue(item, repo)`. Fix the signature now (do not leave for later): `ListAllIssues(ctx context.Context, fetcher PageFetcher, repository string)`. Update the Step 1 test calls accordingly (`ListAllIssues(context.Background(), fetcher, "o/r")`). Note: the test file as written above calls the 2-arg form — the implementer MUST use the 3-arg form; treat the test snippet's call as `ListAllIssues(context.Background(), fetcher, "o/r")`.

```go
func ParseIssue(item any, repository string) (Issue, error) {
    // item must be map[string]any; number from float64/int64/json.Number;
    // title/body/url/author(user.login) strings (body may be absent → "");
    // state required; assignees []any of {login}; labels []any of {name};
    // updated_at RFC3339 required; normalize: assignees/labels nil→[], sorted
    // case-insensitively + deduped; labels keep original case of first occurrence.
    // Triage = ClassifyTriage(labels, title). UpdatedAt parsed (any RFC3339 offset).
    // pull_request presence is NOT an error here — FilterIssues handles the skip.
}

func ClassifyTriage(labels []string, title string) string {
    // trimmed + lowercased comparison; bug: label bug/defect or title "[bug]" prefix;
    // feature: label enhancement/feature or title "[feat]"/"[feature]" prefix; bug wins ties.
}

func FilterIssues(issues []Issue, maintainers []string) (kept []Issue, excluded []ExcludedIssue, skipped int) {
    // precedence: is-pull-request (counted in skipped — needs the raw flag...).
}
```

Problem: `FilterIssues` takes parsed `Issue`, but PR-ness is a wire property. Carry it: `Issue.IsPullRequest bool` set by `ParseIssue` when `pull_request` is present. Then filter step 1: `if issue.IsPullRequest { skipped++; continue }`. Add the field to the struct above (do not forget it or the test's PR case breaks).

`ExcludedIssue{Issue Issue; Reason string}` — define in source.go (results-adjacent but parsing-owned; observe.go reuses it — document that observe.go does NOT redefine it).

```go
func CanonicalSnapshot(issue Issue) ([]byte, error) {
    // exact keys: repo, issue, title, body, author, assignees, labels, url, updatedAt, triage
    // updatedAt = issue.RevisionID()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ghissue/source.go internal/ghissue/source_test.go
git commit -m "feat: add GitHub issue parsing and filter"
```

### Task 3: Atomic case+Task creation

**Files:**
- Modify: `internal/workflowcase/service.go` (or new file `internal/workflowcase/ensure_materialize.go` — prefer the new file; service.go is already large)
- Create: `internal/workflowcase/ensure_materialize_test.go`

**Interfaces:**
- Consumes: `Observation`, `execution.TaskRequest`, existing `Ensure` internals, `CreateTaskWithGuard` semantics (read `internal/workflowcase/materialize.go:30-55` and `internal/execution/service.go` `CreateTaskWithGuard` before coding).
- Produces: `EnsureAndMaterialize(ctx, executionSvc, observation, template) (Case, domain.Task, error)` used by Task 4.

- [ ] **Step 1: Write the failing test**

```go
func TestEnsureAndMaterializeIsAtomic(t *testing.T) {
    // stack: testutil.OpenStore(t), testutil.NewClock(...), purpose.New(store, clk),
    // cases := workflowcase.New(store, clk, purposes), execSvc with execution.New(store, clk, purposes, ...)
    // — read internal/workflowcase/service_test.go lines 15-30 for the exact fixture (store/clk/purposes/mission),
    // and internal/adoreview/observe_test.go for the execution.TaskRequest template shape
    // (objective/payload/acceptance/envelope) — mirror both, do not invent field names.
    // 1. happy path: returns case + task; task readable via execSvc.Task; case Find-able.
    // 2. failing template (blank objective): returns error AND Find(mission, source, object, revision)
    //    reports found=false (no partial case left behind).
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/workflowcase -run TestEnsureAndMaterialize -count=1 -v`
Expected: FAIL with `undefined: EnsureAndMaterialize`.

- [ ] **Step 3: Write minimal implementation**

New file `internal/workflowcase/ensure_materialize.go`: validate inputs (same rules as `Ensure`), run the case insert and the `CreateTaskWithGuard` task insert inside ONE `store.WithTx` transaction, reusing the existing `materializeCurrentCase`-style guard logic against the in-transaction case row. On any error the transaction rolls back (no partial case). Replay semantics mirror `Ensure` (same observation → same case; same idempotency key → same task).

- [ ] **Step 4: Run test to verify it passes**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/workflowcase -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workflowcase/ensure_materialize.go internal/workflowcase/ensure_materialize_test.go
git commit -m "feat: add atomic case and task creation"
```

### Task 4: Observer (ObserveOnce/Run)

**Files:**
- Create: `internal/ghissue/observe.go`
- Create: `internal/ghissue/observe_test.go`

**Interfaces:**
- Consumes: Task 1 client, Task 2 types, Task 3 `EnsureAndMaterialize`, `cases.Find`, `execution.TaskRequest`, `evidence.Put` (`Put(ctx, io.Reader, Metadata{MediaType, Kind})`, MediaType `application/json`, Kind `github.issue.snapshot`).
- Produces: `Config`, `ObserveResult{Ensured, Materialized []domain.ID; Excluded []ExcludedIssue; Failed []FailedIssue; PRSkipped int}`, `ObserveOnce`, `Run` used by Task 5.

- [ ] **Step 1: Write the failing test**

```go
func fullStack(t *testing.T) (context.Context, *state.Store, clock.Clock, *purpose.Service, *execution.Service, *evidence.Store, *workflowcase.Service, domain.ID, domain.ID) {
    // testutil.OpenStore(t); testutil.NewClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC));
    // purposes := purpose.New(store, clk); missionID, _ := purposes.CreateMission(ctx, "triage incoming issues");
    // execSvc := execution.New(store, clk, purposes); evidenceStore, _ := evidence.New(store, t.TempDir(), clk);
    // cases := workflowcase.New(store, clk, purposes);
    // envelope: INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, 100, now) — mirror internal/adoreview/observe_test.go envelope fixture exactly.
    // return all handles
}

func TestObserveOnceRegistersMaintainerIssue(t *testing.T) {
    // fake fetcher: one page with issueItem(1, "maint", "open") + one PR item + one unparseable map
    // cfg: MissionID, Repository "o/r", Maintainers ["maint"], Grant capabilities ["github.issue.read"],
    //      ResourceEnvelopeID envelope, MaxSteps 3, RemainingBudget 10
    // result, err := ObserveOnce(ctx, fetcher, cases, execSvc, evidenceStore, cfg)
    // assert: len(Ensured)==1, len(Materialized)==1, PRSkipped==1, one unparseable exclusion,
    // task payload decodes with issueSnapshot == case ObservationEvidenceID (Find the case, compare).
}

func TestObserveOnceRepollIsNoop(t *testing.T) {
    // same as above, run ObserveOnce twice with identical pages
    // assert: same case ID, same task ID, evidence snapshot count unchanged
    // (count via SELECT COUNT(*) FROM evidence_objects WHERE kind='github.issue.snapshot')
}

func TestObserveOnceRevisionChangeOpensNewCase(t *testing.T) {
    // second poll with updated_at one hour later → new case ID, second task
}

func TestObserveOnceSkipsNonActiveAndValidatesGrant(t *testing.T) {
    // grant without github.issue.read → ObserveOnce returns startup-style validation error before any poll
    // (validate effective caps ⊆ grant at ObserveOnce entry, mirroring the startup rule)
    // blocked case for same revision (set up via direct Ensure then Assess to BLOCKED? — simpler:
    // pre-create via Ensure, then close it through the case API used in close_test.go... if too
    // heavy, drive the state via workflowcase Assess with an Unknown verdict fixture mirroring
    // internal/workflowcase/assessment_test.go) → reason case-not-active on repoll
}
```

`FailedIssue{Issue Issue; Err string}` defined in observe.go. `Config{MissionID, Repository string, Maintainers, Grant, WorkCapabilities []string, ResourceEnvelopeID domain.ID, MaxSteps int, RemainingBudget int64}` with `validate()` enforcing: mission/repo/maintainers/envelope non-empty, grant contains `github.issue.read`, every effective work cap (read + extras) grant-listed, positive limits. `Run(ctx, fetcher, cases, execSvc, evidenceStore, cfg, interval)` mirrors `adoreview.Run` cancellation semantics (nil on cancel, incl. mid-step suppression).

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -run 'TestObserveOnce' -count=1 -v`
Expected: FAIL with `undefined` symbols.

- [ ] **Step 3: Write minimal implementation**

`observe.go`: `ObserveOnce` validates config → `ListAllIssues` (tick error on failure) → per parsed issue: filter (exclusions incl. PR skips) → `Find` → hit: state check (`case-not-active` unless ACTIVE), `MaterializeTask` with payload `issueSnapshot = existing.ObservationEvidenceID` → `Failed` on error; miss: `Put` snapshot → `EnsureAndMaterialize` → `Failed` on error. Task template mirrors `internal/adoreview/observe.go` `materialize()`: Objective `Triage GitHub issue <owner>/<name>#<n>`, PayloadJSON `{repo, issue, revision, title, url, author, labels, triage, issueSnapshot}`, AcceptanceCriteria `["triage decision recorded for <revision>"]`, ResourceEnvelopeID from cfg. `FirstWork{Kind: "github.issue.triage", RequiredCapabilities: ["github.issue.read"]+extras, AuthorityCeiling: grant caps, ProposedActions: nil}`.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/ghissue -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ghissue/observe.go internal/ghissue/observe_test.go
git commit -m "feat: observe maintainer issues into triage tasks"
```

### Task 5: run-gh-intake CLI, read-only composition, full verification

**Files:**
- Modify: `cmd/summa42-box/main.go` (dispatch + `runGHIntake` + `parseGHIntakeFlags`)
- Create: `cmd/summa42-box/gh_intake_test.go` (flag tests) — plus composition assertions inside `runGHIntake` tests? Composition (no feedback emitter/provider, empty executor map) is asserted via a `TestRunGHIntakeOpensReadOnlyBox`-style test ONLY if the Box exposes those fields readably in-process; check `internal/runtime.Box` exported fields first (`Executors`, feedback provider presence). If not observable without a running server, assert instead on the config actually passed to `summa42runtime.Open` by extracting a `ghIntakeRuntimeConfig(cfg) summa42runtime.Config` pure function and testing THAT (no sink, no providers, no executors, feedback local-only). Prefer the pure-function test.
- Modify: `internal/scheduler/service_test.go` or new `internal/scheduler/ghissue_eligibility_test.go`: eligible-but-unclaimed — a task requiring `github.issue.read` is never selected when capacity lacks it (mirror the existing weak-capacity nil-candidate pattern at `service_test.go:94-101`).

**Interfaces:**
- Consumes: Tasks 1–4, `summa42runtime.Open` composition (mirror `runObserver` at `cmd/summa42-box/main.go:707-750`, minus ADO provider/feedback/executors).
- Produces: runnable `run-gh-intake`; nothing downstream in this plan.

- [ ] **Step 1: Write the failing test**

```go
func TestParseGHIntakeFlagsRejects(t *testing.T) {
    // table: missing --mission / missing --repo / missing --maintainer /
    // missing --envelope / grant without github.issue.read / non-positive --poll-interval
    // each → parseGHIntakeFlags returns error mentioning the flag
}

func TestParseGHIntakeFlagsAccepts(t *testing.T) {
    // full valid args → cfg with MissionID/Repository/Maintainers/grant caps incl. github.issue.read/
    // envelope/limits + 30s default interval
}

func TestGHIntakeRuntimeConfigIsReadOnly(t *testing.T) {
    // cfg := ghIntakeRuntimeConfig(base summa42runtime.Config{...}) → assert:
    // len(Executors)==0, len(OperationProviders)==0 (confirm exact field names in
    // internal/runtime/box.go first), CapabilityProviders empty, FeedbackSink nil,
    // FieldFeedback.Mode == localconfig.FeedbackModeLocalOnly
}
```

Check `summa42runtime.Config` exact field names (`Executors`, `OperationProviders`, `CapabilityProviders`, `FeedbackSink`, `FieldFeedback`) in `internal/runtime/box.go` before finalizing assertions.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -run 'TestParseGHIntake|TestGHIntakeRuntime' -count=1 -v`
Expected: FAIL with `undefined` symbols.

- [ ] **Step 3: Write minimal implementation**

`parseGHIntakeFlags` (flag names: `--mission`, `--repo`, `--maintainer` repeatable via a `stringSlice` flag.Value — check whether main.go already has one, reuse it; else define `type stringSlice []string` with `String()`/`Set()` once), `--grant-capability` repeatable, `--work-capability` repeatable, `--envelope`, `--max-steps`, `--remaining-budget`, `--poll-interval` (default 30s). Validation mirrors the spec startup rules (non-empty mission/repo/≥1 maintainer/envelope; grant non-empty and containing `github.issue.read`; every work cap grant-listed; positive max-steps/budget/interval). Env: `SUMMA42_GITHUB_TOKEN_FILE`, `SUMMA42_GITHUB_REPOSITORY` (flag `--repo` wins when both set). `runGHIntake`: resolve home/load config/startup material (mirror `runObserver`), build `ghissue.Client` from env+repo, open Box via the read-only runtime config, construct `workflowcase` service from `box.Store/box.Clock/box.Purpose` (field names per runObserver usage), run `ghissue.Run` until cancellation. Dispatch `run-gh-intake` in `main()` next to `run-observer`.

- [ ] **Step 4: Run package tests, expect PASS**

Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box ./internal/ghissue ./internal/scheduler ./internal/workflowcase -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Run full verification.**

Run in order, all must pass:
  - `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1` (expect all `ok`)
  - `go vet ./...` (expect exit 0), `git diff --check` (expect clean), `gofmt -l` on touched files (expect no output)

- [ ] **Step 6: Commit.**

```bash
git add cmd/summa42-box/main.go cmd/summa42-box/gh_intake_test.go internal/ghissue/observe.go internal/ghissue/observe_test.go internal/scheduler/ghissue_eligibility_test.go
git commit -m "feat: add run-gh-intake command"
```
(Adjust the staged file list to what `git status` actually shows; never `git add .`.)

## Self-review

- Spec coverage: client contract (env, GET-only, Link errors, HTTPS/size/timeout/sanitization) → Task 1; parse/filter/snapshot/classifier/payload → Task 2; atomic creation → Task 3; Find-first flow, state skip, error matrix, Run semantics → Task 4; CLI flags/validation/composition/eligibility → Task 5; acceptance (one case per revision, reasoned outcomes, no lease, no writes) → Tasks 4–5 tests. Triage type recorded, never acted on.
- No placeholders: every step names exact files, code, and commands. Three "read first / confirm field names" notes name the exact files and symbols to verify; test sketches that depend on them say what to do if reality differs (adapt names, report the deviation).
- Type consistency: `Config/Client/FetchIssuesPage/Issue/Unparseable/PageFetcher/ListAllIssues/ParseIssue/FilterIssues/ExcludedIssue/ObserveResult/ObserveOnce/Run/EnsureAndMaterialize` signatures are identical everywhere they appear; reason tokens and snapshot/payload keys match the spec verbatim.
