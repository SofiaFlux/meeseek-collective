# Review Assessment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Assess completed Copilot reviews through live gates into durable vote/comments/hold decisions.

**Architecture:** `evidence.Store.Get` adds blob reads; `internal/adoreview/assess.go` implements `AssessReview` (gates → decision blob → `workflowcase.Assess`). No driver, no writes beyond evidence/cases/assessments.

**Tech Stack:** Go 1.27, existing services, `testutil` store/clock, fake `PRCaller`; no live credentials.

## Global Constraints

- No external writes: assessment proposes, never dispatches.
- Findings never yield approval; gate failures hold even CLEAN verdicts.
- `ado.pr.org_active` never takes an `action` key (not used here, but the rule stands).
- TDD: failing test first for every behavior, then minimal implementation.
- Package suite before each task commit; full `go test ./... -count=1` and `go vet ./...` in Task 2 before the final commit.

---

## File structure

- Modify `internal/evidence/store.go`: add `Get`.
- Modify `internal/evidence/store_test.go`: round-trip + unknown-ID tests (read the file's existing setup pattern first and reuse it).
- Create `internal/adoreview/assess.go`: `ReviewInput`, `ReviewDecision`, `DecisionComment`, `AssessReview`.
- Create `internal/adoreview/assess_test.go`: gate/mapping/decision tests on fakes.

### Task 1: evidence.Get

**Files:**
- Modify: `internal/evidence/store.go`
- Modify: `internal/evidence/store_test.go`

**Interfaces:**
- Consumes: `evidence_objects` schema (`evidence_id, content_hash, media_type, kind, size_bytes, created_at` with RFC3339Nano timestamps), `verifyHash`, blob layout `blobs/sha256/<xx>/<hash>`.
- Produces: `(EvidenceObject, []byte, error) Get` used by Task 2 tests (driver use comes later).

- [ ] **Step 1: Write the failing tests.** Reuse the existing `store_test.go` setup (read the file first for its store constructor pattern). Append:

```go
func TestGetRoundTripsPutContent(t *testing.T) {
    // build store via the file's existing helper
    object, err := store.Put(ctx, strings.NewReader(`{"a":1}`), Metadata{MediaType: "application/json", Kind: "ado.review.decision"})
    if err != nil {
        t.Fatal(err)
    }
    loaded, data, err := store.Get(ctx, object.ID)
    if err != nil {
        t.Fatal(err)
    }
    if string(data) != `{"a":1}` {
        t.Fatalf("data = %q", data)
    }
    if loaded.ID != object.ID || loaded.ContentHash != object.ContentHash || loaded.MediaType != "application/json" || loaded.Kind != "ado.review.decision" || loaded.SizeBytes != object.SizeBytes {
        t.Fatalf("loaded = %+v, want %+v", loaded, object)
    }
    if loaded.CreatedAt.IsZero() {
        t.Fatal("created at is zero")
    }
}

func TestGetUnknownIDErrors(t *testing.T) {
    // same setup
    if _, _, err := store.Get(ctx, domain.NewID("evidence")); err == nil {
        t.Fatal("expected error for unknown evidence ID")
    }
}
```

Check imports in `store_test.go` first (`context` vs `t.Context()`, `strings`, `domain`); adapt names to what the file uses.

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/evidence -run 'TestGet(RoundTripsPutContent|UnknownIDErrors)$' -count=1`. Expected: FAIL (undefined `Get`).
- [ ] **Step 3: Implement `Get`.** In `store.go` (needs `time` import — check whether already imported):

```go
func (s *Store) Get(ctx context.Context, id domain.ID) (EvidenceObject, []byte, error) {
    if s == nil || s.state == nil {
        return EvidenceObject{}, nil, errors.New("evidence store is not configured")
    }
    var object EvidenceObject
    var createdAt string
    object.ID = id
    if err := s.state.DB().QueryRowContext(ctx,
        `SELECT content_hash, media_type, kind, size_bytes, created_at FROM evidence_objects WHERE evidence_id = ?`, id,
    ).Scan(&object.ContentHash, &object.MediaType, &object.Kind, &object.SizeBytes, &createdAt); err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return EvidenceObject{}, nil, fmt.Errorf("evidence %q not found", id)
        }
        return EvidenceObject{}, nil, err
    }
    var err error
    if object.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
        return EvidenceObject{}, nil, fmt.Errorf("parse evidence timestamp: %w", err)
    }
    path := filepath.Join(s.root, "blobs", "sha256", object.ContentHash[:2], object.ContentHash)
    data, err := os.ReadFile(path)
    if err != nil {
        return EvidenceObject{}, nil, fmt.Errorf("read evidence blob: %w", err)
    }
    if err := verifyHash(path, object.ContentHash); err != nil {
        return EvidenceObject{}, nil, fmt.Errorf("verify evidence blob: %w", err)
    }
    return object, data, nil
}
```

`store.go` already imports `database/sql`, `errors`, `fmt`, `os`, `path/filepath`, `strings`; add `time` only if missing (check the import block first).

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/evidence -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git add internal/evidence/store.go internal/evidence/store_test.go && git commit -m "feat: add evidence blob read"`.

### Task 2: AssessReview with gates and decision blob

**Files:**
- Create: `internal/adoreview/assess.go`
- Create: `internal/adoreview/assess_test.go`

**Interfaces:**
- Consumes: `PRCaller`, `CopilotReviewPayload` (define locally: repo/pr/sourceCommit/targetCommit — do NOT import executors for the payload; see below), `workflowcase.Ensure/Assess/Case`, `execution` (only via `MaterializeTask`? No — Task 2 does not materialize; `execution` import unneeded), `evidence.Put/Get/Metadata`, `workflow.Decision/Assessment/WorkProposal/Grant`.
- Produces: `ReviewInput`, `ReviewDecision`, `AssessReview` for the Slice 3 driver.

Payload note: `executors` already defines an equivalent payload struct for the Copilot executor. To avoid an `adoreview → executors` import for one struct, define the four fields inline in `ReviewInput` (`Repo string, PR int64, SourceCommit, TargetCommit string`) instead of a shared type. The driver maps both sides in Slice 3.

- [ ] **Step 1: Write the failing tests.** Fixture (in `assess_test.go`, `package adoreview` like `observe_test.go`):

```go
func setupAssess(t *testing.T, grant workflow.Grant) (context.Context, *workflowcase.Service, *evidence.Store, domain.ID) {
    t.Helper()
    ctx := context.Background()
    store := testutil.OpenStore(t)
    clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
    purposes := purpose.New(store, clk)
    cases := workflowcase.New(store, clk, purposes)
    evidenceStore, err := evidence.New(store, t.TempDir(), clk)
    if err != nil {
        t.Fatal(err)
    }
    mission, err := purposes.CreateMission(ctx, "ado review")
    if err != nil {
        t.Fatal(err)
    }
    return ctx, cases, evidenceStore, mission
}

func ensureReviewCase(t *testing.T, ctx context.Context, cases *workflowcase.Service, mission domain.ID, grant workflow.Grant) workflowcase.Case {
    t.Helper()
    c, err := cases.Ensure(ctx, workflowcase.Observation{
        MissionID: mission, Source: "ado", ObjectID: "shop#1", RevisionID: "a:b",
        EvidenceID: "ev-obs-1",
        FirstWork: workflow.WorkProposal{Kind: "ado.pr.review", RequiredCapabilities: []string{"read"}, AuthorityCeiling: []string{"read"}},
        Grant: grant, MaxSteps: 3, RemainingBudget: 10,
    })
    if err != nil {
        t.Fatal(err)
    }
    return c
}
```

Caller fake: reuse the `fakeCaller` pages pattern from `source_test.go` (same package) with `map[string]any{"prs":...}`? No — gates call `ado.pr.get` / `ado.build.status` directly; fake returns per-capability canned maps:

```go
type gateCaller struct {
    pr     map[string]any
    build  map[string]any
    calls  []string
}

func (g *gateCaller) Call(_ context.Context, capability string, _ any) (any, error) {
    g.calls = append(g.calls, capability)
    switch capability {
    case "ado.pr.get":
        return g.pr, nil
    case "ado.build.status":
        return g.build, nil
    default:
        return nil, errors.New("unexpected capability " + capability)
    }
}
```

`pr` fixture: `map[string]any{"sourceCommit": "a", "targetCommit": "b"}`; build fixture: `map[string]any{"status": "succeeded"}`.

Review fixture: `executors.ReviewResult{Verdict: executors.ReviewClean, ReviewedCommits: []string{"a"}, ReviewedFiles: []string{"main.go"}}` — first confirm the exact `ReviewResult` field and `ReviewVerdict` constant identifiers in `internal/executors/copilot.go` (names above are as specified in the executor plan; use the file's truth if they differ).

Import `executors` for `ReviewResult` (no cycle: `executors` imports `domain`, `teb`, `clock` — never `adoreview`; verify with `grep -rn adoreview internal/executors/` before coding — empty output expected). Use `executors.ReviewResult` / verdict constants directly; do not redefine the struct.

Tests (each: setup, ensure case, fake caller, `AssessReview`, assert `AssessmentResult.Decision.Outcome`, `Next.ProposedActions`, case state via `cases.Get`, decision blob via helper `latestDecision(t, store)` selecting `evidence_id` from `evidence_objects WHERE kind='ado.review.decision'` order by rowid desc limit 1, then `evidenceStore.Get` + unmarshal into `ReviewDecision`.)

Cases: CLEAN→Continue+approve+case ACTIVE with new WorkID; FINDINGS(2 findings w/ paths ⊆ files)→Continue+comment, decision.Comments bodies exact; UNCERTAIN→Unknown+BLOCKED; stale (pr.get returns source "z")→Unknown reason contains stale-review; outside-files finding→Unknown; empty-findings FINDINGS→Unknown; CLEAN-with-findings→Unknown verdict-findings-mismatch; grant actions [] with CLEAN→Unknown grant-denies-ado.pr.approve; CI required+green→Continue, red→Unknown, missing status→Unknown ci-unknown; decision blob re-readable with Action approve/comments/hold.

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -run 'TestAssessReview' -count=1`. Expected: FAIL (undefined `AssessReview`, `ReviewInput`, `ReviewDecision`).
- [ ] **Step 3: Implement `assess.go`.**

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
    "github.com/SofiaFlux/summa42/internal/executors"
    "github.com/SofiaFlux/summa42/internal/workflow"
    "github.com/SofiaFlux/summa42/internal/workflowcase"
)

type ReviewInput struct {
    Case             workflowcase.Case
    WorkID           domain.ID
    Review           executors.ReviewResult
    ReviewEvidenceID domain.ID
    Repo             string
    PR               int64
    SourceCommit     string
    TargetCommit     string
    Project          string
    Caller           PRCaller
    RequireCI        bool
    CIStatus         string
    RemainingBudget  int64
}

type DecisionComment struct {
    Path string `json:"path"`
    Line int64  `json:"line"`
    Body string `json:"body"`
}

type ReviewDecision struct {
    Action   string            `json:"action"`
    Comments []DecisionComment `json:"comments"`
    Vote     string            `json:"vote"`
    Reason   string            `json:"reason"`
}

const (
    DecisionCommentAction = "comment"
    DecisionApproveAction = "approve"
    DecisionHoldAction    = "hold"
)
```

`AssessReview(ctx, callerUnused?, cases, evidenceStore, input)` — signature: `func AssessReview(ctx context.Context, cases *workflowcase.Service, evidenceStore *evidence.Store, input ReviewInput) (workflowcase.AssessmentResult, ReviewDecision, error)`. Caller comes from `input.Caller` (nil allowed only when gates need no live calls? Freshness ALWAYS needs pr.get → Caller required non-nil; validate).

Flow:
1. Validate: case ID/work ID non-blank, ReviewEvidenceID non-blank, repo/pr>0/commits non-blank, caller non-nil, CIStatus default "succeeded" when blank.
2. Freshness: `caller.Call(ctx, "ado.pr.get", args)` where args = `{"action":"get"}` + project if non-empty + repository + pullRequestId. Extract strings with helper `stringField(m, keys...)`: source = sourceCommit else lastMergeSourceCommit; target = targetCommit else lastMergeTargetCommit. Missing → hold `stale-review` (fail closed). Check source==input.SourceCommit && target==input.TargetCommit && contains(reviewedCommits, source) else hold.
3. Consistency: set of reviewedFiles; each finding path non-blank (executor guarantees; re-check defensively → hold `invalid-finding` if blank) and ∈ set else hold `uncovered-finding`; FINDINGS+len==0 → hold `empty-findings`.
4. CI if required: buildId = string/float fields mergeBuildId else buildId from pr.get map; missing → hold `ci-unknown`; call build.status {"action":"get_status","buildId":id}; status = status else result; != expected → hold `ci-failed`.
5. Verdict mapping: UNCERTAIN → hold `uncertain-review`; CLEAN+len(findings)>0 → hold `verdict-findings-mismatch`; CLEAN → propose approve (check "ado.pr.approve" ∈ case.Grant.Actions else hold `grant-denies-ado.pr.approve`); FINDINGS → propose comment (check "ado.pr.comment" else hold).
6. Decision: Action/Comments (Body `<path>[:<line>]: <explanation>` + "\n\nEvidence: " + finding.Evidence when non-blank)/Vote "approve" or ""/Reason human sentence. Canonical marshal → Put(application/json, ado.review.decision).
7. Next: Kind "publish-decision", caps/ceiling copied from input.Case.NextWork, ProposedActions per branch. Signature `ado:<ObjectID>:<RevisionID>:<verdict>` where ObjectID = repo#pr, RevisionID = source:target (rebuild from input, not methods — same encoding).
8. `cases.Assess(ctx, AssessmentRequest{CaseID, WorkID, Assessment: {Verdict, Reason: decision.Reason, EvidenceIDs: [reviewEvidenceID, decisionID], Next}, RemainingBudget, ProgressSignature})`. Return (result, decision, nil).

`contains`/`stringField`/`numberField` small helpers. `strconv` needed for float→string buildId? buildId may be float64 → format with `strconv.FormatInt(int64(v),10)`. Import strconv.

- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -count=1`. Expected: PASS.
- [ ] **Step 5: Run full verification.** In order: `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1`, `go vet ./...`, `git diff --check`, `gofmt -l` on touched files. All must pass.
- [ ] **Step 6: Commit.** `git add internal/evidence/store.go internal/evidence/store_test.go internal/adoreview/assess.go internal/adoreview/assess_test.go && git commit -m "feat: assess ADO reviews into publish decisions"`. (Task 1 files already committed; verify with `git status` first.)

## Self-review

- Spec coverage: input struct → Task 2 (with ReviewEvidenceID + Project added per review fixes); gates → freshness/consistency/CI branches with named hold reasons; mapping (full Next, grant-membership precheck, mismatch rule, BLOCKED-vs-error) → branches; ProgressSignature encoding + honest replay → signature builder + no retry-safety claim; decision rendering + vote marker → comments/vote block; evidence.Get → Task 1; testing list → all cases; acceptance → assertions on outcome/Next/blob.
- No placeholders: exact files, code, commands, outputs; fake shapes encode the provisional wire assumptions from the spec.
- Type consistency: `ReviewInput`/`ReviewDecision`/`DecisionComment`/`AssessReview`/`Get` signatures identical across tasks; `publish-decision`, `ado.pr.approve/comment`, `ado.review.decision`, `application/json` literals fixed.
