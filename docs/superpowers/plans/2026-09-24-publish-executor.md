# Publish Executor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish approved review decisions through effect slots in staged modes, with Work 2 ceilings covering effect capabilities.

**Architecture:** `Publisher` executor in `internal/adoreview` driven by a 2-method `PublishOps` interface (real `operations.Service` in prod, fake in tests); assessor amended to append effect caps; env builder registers kind `ado-publish` in `runWorker` only.

**Tech Stack:** Go 1.27, existing operations/evidence/workflowcase services, fake `PublishOps` + fake MCP; no live credentials.

## Global Constraints

- No `SettleOutcome`/provider-`LookupOutcome` calls in this slice: Prepare→Dispatch only; UNKNOWN recorded for later reconciliation.
- Vote value exactly 10; approve risk never defaults to LOW (`all` mode without explicit approve risk is a startup error).
- `adomcp` untouched; `run()` and `runObserver` untouched.
- TDD: failing test first for every behavior, then minimal implementation.
- Package suites before each task commit; full `go test ./... -count=1` and `go vet ./...` in Task 2 before the final commit.

---

## File structure

- Create `internal/adoreview/publish.go`: `PublishMode`, `PublishConfig`, `PublishOps`, `PublishPayload`, `Publisher`, `Start`.
- Create `internal/adoreview/publish_test.go`: fake ops + fake MCP decisions.
- Modify `internal/adoreview/assess.go` + `assess_test.go`: effect-cap append + grant-denies-capability holds.
- Modify `docs/superpowers/specs/2026-09-24-review-assessment-design.md` + `docs/superpowers/plans/2026-09-24-review-assessment.md`: 2–3 line amend each describing the cap-append rule.
- Modify `cmd/summa42-box/main.go`: env builder + `runWorker` merge (+ notice when absent).
- Create `cmd/summa42-box/publish_flags_test.go` or extend env tests (check existing file layout first): builder tests.

### Task 1: Publisher executor plus assessor cap fix

**Files:**
- Create: `internal/adoreview/publish.go`
- Create: `internal/adoreview/publish_test.go`
- Modify: `internal/adoreview/assess.go`, `assess_test.go`
- Modify: the two Slice 2 docs (short amends)

**Interfaces:**
- Consumes: `executors.Executor/AttemptEnvelope/ExecutionResult/Evidence`, `evidence.Get/Put/Metadata`, `operations.PrepareRequest/ProviderDispatchRequest/ProviderOutcome`, `adoeffects.CommentIntent/VoteIntent/CommentProvider/VoteProvider`, `ReviewDecision`, `workflowcase.Assess`.
- Produces: `Publisher`, `PublishConfig`, `PublishOps`, `PublishPayload` used by Task 2 wiring.

- [ ] **Step 1: Write the failing tests.** In `publish_test.go` (`package adoreview`):

```go
type fakePublishOps struct {
    prepared []operations.PrepareRequest
    dispatched []domain.ID
    ops     map[domain.ID]domain.ExternalOperation
    nextID  int
    dispatchState domain.OperationState
}

func (f *fakePublishOps) Prepare(_ context.Context, request operations.PrepareRequest) (domain.ExternalOperation, error) {
    f.prepared = append(f.prepared, request)
    f.nextID++
    op := domain.ExternalOperation{ID: domain.ID(fmt.Sprintf("op-%d", f.nextID))}
    f.ops[op.ID] = op
    return op, nil
}

func (f *fakePublishOps) Dispatch(_ context.Context, operationID, _ domain.ID) (domain.ExternalOperation, error) {
    f.dispatched = append(f.dispatched, operationID)
    op := f.ops[operationID]
    if f.dispatchState != "" {
        op.State = f.dispatchState
    } else {
        op.State = domain.OperationConfirmedEffect
    }
    return op, nil
}
```

Check `domain.ExternalOperation` exact fields first (`internal/domain/operation.go`: ID field at least; use only `ID` plus whatever the compiler demands — read the struct before finalizing the fake). Tests:

```go
func putDecision(t *testing.T, store *evidence.Store, ctx context.Context, decision ReviewDecision) domain.ID {
    t.Helper()
    raw, err := json.Marshal(decision)
    if err != nil {
        t.Fatal(err)
    }
    object, err := store.Put(ctx, bytes.NewReader(raw), evidence.Metadata{MediaType: "application/json", Kind: "ado.review.decision"})
    if err != nil {
        t.Fatal(err)
    }
    return object.ID
}

func publishEnvelope(task, attempt string, workspace string) executors.AttemptEnvelope {
    return executors.AttemptEnvelope{TaskID: domain.ID(task), AttemptID: domain.ID(attempt), Workspace: workspace, Objective: "publish"}
}

func publishPayload(t *testing.T, decision domain.ID) json.RawMessage {
    t.Helper()
    raw, err := json.Marshal(PublishPayload{Decision: string(decision), CaseID: "case-1", WorkID: "work-1", Project: "proj", Repo: "shop", PR: 1, Revision: "a:b"})
    if err != nil {
        t.Fatal(err)
    }
    return raw
}

func TestPublishNoneRecordsWithoutPrepare(t *testing.T) {
    ctx := context.Background()
    store := testutil.OpenStore(t)
    clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
    evidenceStore, err := evidence.New(store, t.TempDir(), clk)
    if err != nil {
        t.Fatal(err)
    }
    id := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"})
    publisher, err := NewPublisher(PublishConfig{Mode: PublishNone, Operations: &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}}, OwnerApprovals: []domain.ID{"owner-1"}, RiskComment: "LOW"})
    if err != nil {
        t.Fatal(err)
    }
    envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
    envelope.PayloadJSON = publishPayload(t, id)
    result, err := publisher.Start(ctx, envelope)
    if err != nil {
        t.Fatal(err)
    }
    if len(result.Evidence) == 0 {
        t.Fatal("no evidence recorded")
    }
}

func TestPublishCommentsDispatchesAndSkipsApprove(t *testing.T) {
    ctx := context.Background()
    store := testutil.OpenStore(t)
    clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
    evidenceStore, err := evidence.New(store, t.TempDir(), clk)
    if err != nil {
        t.Fatal(err)
    }
    id := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionCommentAction, Vote: "approve", Reason: "mixed",
        Comments: []DecisionComment{{Path: "a.go", Line: 1, Body: "a.go:1: nit"}, {Path: "b.go", Line: 0, Body: "b.go: nit"}}})
    ops := &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}}
    publisher, err := NewPublisher(PublishConfig{Mode: PublishComments, Operations: ops, OwnerApprovals: []domain.ID{"owner-1"}, RiskComment: "LOW"})
    if err != nil {
        t.Fatal(err)
    }
    envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
    envelope.PayloadJSON = publishPayload(t, id)
    if _, err := publisher.Start(ctx, envelope); err != nil {
        t.Fatal(err)
    }
    if len(ops.prepared) != 2 {
        t.Fatalf("prepared = %d, want 2", len(ops.prepared))
    }
    for i, want := range []string{"ado.pr.comment:proj/shop#1:a:b:0", "ado.pr.comment:proj/shop#1:a:b:1"} {
        if ops.prepared[i].TrustedSlotKey != want {
            t.Fatalf("slot[%d] = %q, want %q", i, ops.prepared[i].TrustedSlotKey, want)
        }
        if ops.prepared[i].Provider != "ado-pr-comment" || ops.prepared[i].Risk != "LOW" {
            t.Fatalf("prepared[%d] = %+v", i, ops.prepared[i])
        }
        if len(ops.prepared[i].RequiredApprovals) != 1 || ops.prepared[i].RequiredApprovals[0] != "owner-1" {
            t.Fatalf("approvals = %v", ops.prepared[i].RequiredApprovals)
        }
    }
}

func TestPublishAllDispatchesVote(t *testing.T) {
    ctx := context.Background()
    store := testutil.OpenStore(t)
    clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
    evidenceStore, err := evidence.New(store, t.TempDir(), clk)
    if err != nil {
        t.Fatal(err)
    }
    id := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"})
    ops := &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}}
    publisher, err := NewPublisher(PublishConfig{Mode: PublishAll, Operations: ops, RiskComment: "LOW", RiskApprove: "OWNER"})
    if err != nil {
        t.Fatal(err)
    }
    envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
    envelope.PayloadJSON = publishPayload(t, id)
    if _, err := publisher.Start(ctx, envelope); err != nil {
        t.Fatal(err)
    }
    if len(ops.prepared) != 1 {
        t.Fatalf("prepared = %d, want 1", len(ops.prepared))
    }
    got := ops.prepared[0]
    if got.Provider != "ado-pr-vote" || got.TrustedSlotKey != "ado.pr.approve:proj/shop#1:a:b" || got.Risk != "OWNER" {
        t.Fatalf("prepared = %+v", got)
    }
}

func TestPublishUnknownRecordedAndStops(t *testing.T) {
    ctx := context.Background()
    store := testutil.OpenStore(t)
    clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
    evidenceStore, err := evidence.New(store, t.TempDir(), clk)
    if err != nil {
        t.Fatal(err)
    }
    id := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionCommentAction, Reason: "mixed",
        Comments: []DecisionComment{{Path: "a.go", Line: 1, Body: "a.go:1: nit"}, {Path: "b.go", Body: "b.go: nit"}}})
    ops := &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}, dispatchState: domain.OperationOutcomeUnknown}
    publisher, err := NewPublisher(PublishConfig{Mode: PublishComments, Operations: ops, RiskComment: "LOW"})
    if err != nil {
        t.Fatal(err)
    }
    envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
    envelope.PayloadJSON = publishPayload(t, id)
    result, err := publisher.Start(ctx, envelope)
    if err != nil {
        t.Fatal(err)
    }
    if len(ops.prepared) != 1 {
        t.Fatalf("prepared = %d, want 1 (stop after UNKNOWN)", len(ops.prepared))
    }
    if len(result.Evidence) == 0 {
        t.Fatal("no evidence recorded")
    }
}

func TestPublishRejectsBadPayload(t *testing.T) {
    ctx := context.Background()
    store := testutil.OpenStore(t)
    clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
    evidenceStore, err := evidence.New(store, t.TempDir(), clk)
    if err != nil {
        t.Fatal(err)
    }
    ops := &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}}
    publisher, err := NewPublisher(PublishConfig{Mode: PublishAll, Operations: ops, RiskComment: "LOW", RiskApprove: "OWNER"})
    if err != nil {
        t.Fatal(err)
    }
    envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
    raw, _ := json.Marshal(PublishPayload{CaseID: "case-1", WorkID: "work-1", Project: "proj", Repo: "shop", PR: 1, Revision: "a:b"})
    envelope.PayloadJSON = raw
    if _, err := publisher.Start(ctx, envelope); err == nil {
        t.Fatal("accepted blank decision")
    }
    holdID := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionHoldAction, Reason: "uncertain"})
    envelope.PayloadJSON = publishPayload(t, holdID)
    result, err := publisher.Start(ctx, envelope)
    if err != nil {
        t.Fatal(err)
    }
    if len(ops.prepared) != 0 || len(result.Evidence) == 0 {
        t.Fatalf("prepared = %d evidence = %d, want 0 and >0", len(ops.prepared), len(result.Evidence))
    }
}
```

`bytes`, `context`, `encoding/json`, `testing`, `time`, `domain`, `evidence`, `executors`, `operations`, `testutil` imports as needed. `PublishPayload` field types must match the implementation (`Decision/CaseID/WorkID/Project/Repo/Revision string`, `PR int64`).

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -run 'TestPublish' -count=1`. Expected: FAIL (undefined `Publisher`, `PublishConfig`, etc.).
- [ ] **Step 3: Implement `publish.go`.**

```go
package adoreview

type PublishMode string

const (
    PublishNone     PublishMode = "none"
    PublishComments PublishMode = "comments"
    PublishAll      PublishMode = "all"
)

type PublishOps interface {
    Prepare(ctx context.Context, request operations.PrepareRequest) (domain.ExternalOperation, error)
    Dispatch(ctx context.Context, operationID, attemptID domain.ID) (domain.ExternalOperation, error)
}

type PublishConfig struct {
    Mode            PublishMode
    Operations      PublishOps
    Comment         *adoeffects.CommentProvider
    Vote            *adoeffects.VoteProvider
    OwnerApprovals  []domain.ID
    RiskComment     string
    RiskApprove     string
}

type PublishPayload struct {
    Decision string `json:"decision"`
    CaseID   string `json:"caseID"`
    WorkID   string `json:"workID"`
    Project  string `json:"project"`
    Repo     string `json:"repo"`
    PR       int64  `json:"pr"`
    Revision string `json:"revision"`
}

type Publisher struct {
    config PublishConfig
}

func NewPublisher(config PublishConfig) (*Publisher, error) {
    switch config.Mode {
    case PublishNone, PublishComments, PublishAll:
    default:
        return nil, fmt.Errorf("unknown publish mode %q", config.Mode)
    }
    if config.Operations == nil || config.Comment == nil || config.Vote == nil {
        return nil, errors.New("publisher requires operations and both effect providers")
    }
    if config.RiskComment == "" {
        config.RiskComment = "LOW"
    }
    if config.Mode == PublishAll && strings.TrimSpace(config.RiskApprove) == "" {
        return nil, errors.New("publish-all mode requires an explicit approve risk")
    }
    return &Publisher{config: config}, nil
}
```

`Start`: validate envelope (workspace/objective non-blank, codex pattern); decode payload (all fields non-blank, PR > 0) → fail before anything; `evidence.Get(decision)` → unmarshal `ReviewDecision` → hold action → record-only evidence, return success. Build intents: comments → `adoeffects.CommentIntent{Project, Repository: Repo, PR, Path, Line, Body, Marker: "[summa42:"+CaseID+":"+WorkID+"]"}`; approve vote → `VoteIntent{...Vote: 10}` iff decision.Vote == "approve". Mode filter: none → record all as recorded-only; comments → dispatch comments, approve skipped; all → dispatch all. Per dispatched intent: `Prepare{AttemptID: envelope.AttemptID, Provider: "ado-pr-comment"/"ado-pr-vote", TrustedSlotKey, Intent, Risk: RiskComment/RiskApprove, Attributes: {"repo","pr","revision","project"}, RequiredApprovals: OwnerApprovals}` → `Dispatch(op.ID, attemptID)` → if result state is UNKNOWN (check exact `domain.OperationOutcomeUnknown` constant) record + stop (return success with partial evidence, no error — reconciliation later); else record confirmed entry. Evidence: one `AGENT_MESSAGE` JSON array of `{slot, operation, state, reference|skipped|recorded-only}`. Non-UNKNOWN Dispatch error → return error (worker FailAttempt path).

Imports: `context`, `encoding/json`, `errors`, `fmt`, `strings`, `domain`, `evidence`, `executors`, `operations`, `adoeffects`.

- [ ] **Step 4: Amend the assessor caps.** In `assess.go`, where the branch builds Next (after the grant-actions check): determine `effectCap` (`ado.pr.comment` for comment branch, `ado.pr.approve` for approve branch); if not ∈ grant capabilities → hold `grant-denies-capability:<cap>`; else append to both RequiredCapabilities and AuthorityCeiling. Add tests: CLEAN grant caps `[read]` → hold `grant-denies-capability:ado.pr.approve`; CLEAN grant caps `[read, ado.pr.approve]` → approve proposal with ceiling containing the cap; FINDINGS mirror with comment cap. Amend the assessment spec Mapping section + assessment plan with 2–3 lines each stating the append rule (find the exact paragraphs first).
- [ ] **Step 5: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/adoreview -count=1`. Expected: PASS.
- [ ] **Step 6: Commit.** `git add internal/adoreview/publish.go internal/adoreview/publish_test.go internal/adoreview/assess.go internal/adoreview/assess_test.go docs/superpowers/specs/2026-09-24-review-assessment-design.md docs/superpowers/plans/2026-09-24-review-assessment.md && git commit -m "feat: add publish executor and effect caps"`.

### Task 2: Env registration, full verification

**Files:**
- Modify: `cmd/summa42-box/main.go`
- Create/modify: builder tests (check existing `copilot_env_test.go`/`worker_flags_test.go` style first)

**Interfaces:**
- Consumes: `Publisher`, `PublishConfig`, `PublishMode` from Task 1; `runtime.Config` executor/provider fields; ADO env vars.
- Produces: kind `ado-publish` in runWorker; nothing downstream in this plan.

- [ ] **Step 1: Write the failing tests.** Builder `buildPublishExecutorFromEnv() (map[string]executors.Executor, error)` — hmm, registration also needs `OperationProviders` appended. Single builder returning both: `buildPublishFromEnv() (executors map[string]executors.Executor, providers []operations.Provider, err error)`? Cleaner: `buildAdoEffectsProviders() ([]operations.Provider, error)` (nil,nil when ADO env absent, mirroring nil-nil-absent) + `buildPublisherExecutor(ops, providers)` pure constructor from already-built pieces? Keep it simple and testable:

```go
func TestPublishEnvAbsentRegistersNothing(t *testing.T) {
    t.Setenv("SUMMA42_PUBLISH_MODE", "")
    // with ADO env also absent
    executors, providers, err := buildPublishFromEnv()
    if err != nil || executors != nil || providers != nil {
        t.Fatalf("got %v %v %v, want nil nil nil", executors, providers, err)
    }
}

func TestPublishEnvRejectsBadMode(t *testing.T) {
    t.Setenv("SUMMA42_PUBLISH_MODE", "everything")
    if _, _, err := buildPublishFromEnv(); err == nil {
        t.Fatal("accepted bad mode")
    }
}

func TestPublishEnvRequiresApproveRiskInAllMode(t *testing.T) {
    t.Setenv("SUMMA42_PUBLISH_MODE", "all")
    t.Setenv("SUMMA42_PUBLISH_RISK_APPROVE", "")
    // + minimal ADO env pointing at /bin/true? provider construction needs Command+Organization only (no dial at build) — set SUMMA42_ADO_MCP_COMMAND=/bin/true + ORGANIZATION=Contoso
    if _, _, err := buildPublishFromEnv(); err == nil {
        t.Fatal("accepted all mode without approve risk")
    }
}
```

Confirm exact existing ADO env var names (`SUMMA42_ADO_MCP_COMMAND`, `SUMMA42_ADO_ORGANIZATION`) and `runtime.Config` provider/executor field names in the code before finalizing (grep first).

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -run 'TestPublishEnv' -count=1`. Expected: FAIL (undefined `buildPublishFromEnv`).
- [ ] **Step 3: Implement.** `buildPublishFromEnv`: mode parse (default none; reject others); if ADO env absent → (nil,nil,nil) with stderr notice at call site (not inside builder); build adomcp provider via existing `buildADOProviderFromEnv` (nil → absent path); construct comment+vote adoeffects providers with `ReadFunc` bound to the adomcp provider's `Call`; append to `[]operations.Provider`; build `Publisher` with `Operations` left nil here — NO: Publisher needs operations service, available only after Open as `box.Operations`. So builder returns providers + publish settings; `runWorker` after Open constructs `NewPublisher{Operations: box.Operations, ...}` and merges kind. Split: `buildAdoEffectProviders() ([]operations.Provider, error)` + settings struct `publishSettings{mode, approvers, riskComment, riskApprove}` from env; runWorker wires both. If ADO absent → skip all + notice.
- [ ] **Step 4: Run package tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/summa42-box -count=1`. Expected: PASS.
- [ ] **Step 5: Run full verification.** In order: `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1`, `go vet ./...`, `git diff --check`, `gofmt -l` on touched files. All must pass.
- [ ] **Step 6: Commit.** `git add cmd/summa42-box/main.go <exact-test-files-from-git-status> && git commit -m "feat: register publish executor from env"`.

## Self-review

- Spec coverage: contract (config/interface/payload/marker/slots) → Task 1; modes+risk split → config validation + tests; evidence lines → assertions; cap fix + doc amends → Task 1 Step 4; registration (env names, runWorker-only, absent notice) → Task 2; testing list → all; acceptance → staged assertions + ceiling coverage test.
- No placeholders: exact files, code, commands, outputs; three "check/confirm first" notes name exact files/symbols.
- Type consistency: `PublishConfig/PublishOps/PublishMode/PublishPayload/Publisher/NewPublisher`, `SUMMA42_PUBLISH_*`, slot/marker formats identical across tasks.
