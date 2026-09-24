# Scheduler Worker Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Drive `ELIGIBLE` Tasks to completed or failed Attempts with a sequential polling worker plus a thin `run-worker` CLI command.

**Architecture:** New `Worker` in `internal/scheduler` composes the existing `Service`, `execution`, `evidence` and `verification` services with an executor registry. `StepOnce` performs one lease→execute→complete/fail iteration (deterministic, fake-friendly); `Run` ticks it until context cancellation. The CLI adds `os.Args` dispatch without touching the control-plane path.

**Tech Stack:** Go 1.27, SQLite via existing `state.Store`, stdlib `flag`, existing `testutil` store/clock helpers.

## Global Constraints

- Generic only: no recipe-, ADO- or Copilot-specific branches anywhere in this slice.
- TDD: failing test first for every behavior, then minimal implementation.
- The worker dispatches no `operations.Service` effects; executors only return evidence.
- Success never yields `SUCCEEDED`: completion lands in `AWAITING_VERIFICATION` with `PENDING` verification work.
- Full `go test ./internal/scheduler -count=1` must pass before each Task 1–3 commit; full `go test ./... -count=1` and `go vet ./...` must pass in Task 4 before the final commit.

---

## File structure

- Create `internal/scheduler/worker.go`: `Outcome`, `StepResult`, `Worker`, `NewWorker`, `StepOnce`, `Run`, plus unexported helpers (`registryKinds`, `buildEnvelope`, `persistEvidence`, `failExecution`). One responsibility: one supervised attempt per step.
- Create `internal/scheduler/worker_test.go`: fake executor plus full service stack via `testutil`; covers idle, success, failure, panic, cancellation.
- Modify `cmd/summa42-box/main.go`: `os.Args` dispatch for `run-worker`, `parseWorkerFlags` helper, `runWorker` wiring shared `Open` to `Worker.Run`. Control-plane `run()` untouched.
- Create `cmd/summa42-box/worker_flags_test.go`: flag default/override/error cases for `parseWorkerFlags`.

**Shared test stack (copy this pattern in every worker test):**

```go
ctx := context.Background()
store := testutil.OpenStore(t)
clk := testutil.NewClock(time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC))
purposes := purpose.New(store, clk)
execSvc := execution.New(store, clk, purposes)
resourceSvc := resources.New(store, clk)
schedSvc := scheduler.New(store, clk, purposes, execSvc, resourceSvc, time.Minute)
evidenceStore, err := evidence.New(store, t.TempDir(), clk)
if err != nil {
    t.Fatal(err)
}
verifySvc := verification.New(store, clk, execSvc)
```

`scheduler.New` without a preference keeps executor choice on the deterministic sorted baseline. `time.Minute` lease doubles as the executor timeout.

**Shared task fixture:**

```go
envelopeID := domain.NewID("envelope")
if _, err := store.DB().ExecContext(ctx,
    `INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, ?, ?)`,
    envelopeID, 100, clk.Now().UTC().Format(time.RFC3339Nano),
); err != nil {
    t.Fatal(err)
}
task, err := execSvc.CreateTask(ctx, execution.TaskRequest{
    Purpose:              domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: domain.ID("owner-worker")},
    Objective:            "review the diff",
    PayloadJSON:          json.RawMessage(`{"pr":7}`),
    AcceptanceCriteria:   []string{"done"},
    RequiredCapabilities: []string{"shell"},
    RequiredEnforcement:  domain.EnforcementEnforced,
    AuthorityCeiling:     []string{"shell"},
    ResourceEnvelopeID:   envelopeID,
})
if err != nil {
    t.Fatal(err)
}
capacity := scheduler.CapacitySnapshot{Capabilities: map[string]scheduler.CapabilityCapacity{
    "shell": {Accessible: true, Enforcement: domain.EnforcementEnforced},
}}
```

**Shared fake executor:**

```go
type fakeExecutor struct {
    result executors.ExecutionResult
    err    error
    seen   []executors.AttemptEnvelope
}

func (f *fakeExecutor) Start(ctx context.Context, envelope executors.AttemptEnvelope) (executors.ExecutionResult, error) {
    f.seen = append(f.seen, envelope)
    return f.result, f.err
}
```

### Task 1: Worker skeleton with idle step

**Files:**
- Create: `internal/scheduler/worker.go`
- Create: `internal/scheduler/worker_test.go`

**Interfaces:**
- Consumes: `scheduler.New`, `execution.New`, `evidence.New`, `verification.New`, `testutil.OpenStore`, `testutil.NewClock` (patterns above).
- Produces: `scheduler.Outcome`, `scheduler.StepResult`, `scheduler.NewWorker`, `(*scheduler.Worker).StepOnce` used by Tasks 2–4.

- [ ] **Step 1: Write the failing idle test.** In `internal/scheduler/worker_test.go` (`package scheduler_test`):

```go
func TestStepOnceIdleWhenNoCandidate(t *testing.T) {
    ctx := context.Background()
    store := testutil.OpenStore(t)
    clk := testutil.NewClock(time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC))
    purposes := purpose.New(store, clk)
    execSvc := execution.New(store, clk, purposes)
    resourceSvc := resources.New(store, clk)
    schedSvc := scheduler.New(store, clk, purposes, execSvc, resourceSvc, time.Minute)
    evidenceStore, err := evidence.New(store, t.TempDir(), clk)
    if err != nil {
        t.Fatal(err)
    }
    verifySvc := verification.New(store, clk, execSvc)
    worker, err := scheduler.NewWorker(schedSvc, execSvc, evidenceStore, verifySvc,
        map[string]executors.Executor{"shell": &fakeExecutor{}}, clk, t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    got, err := worker.StepOnce(ctx, scheduler.CapacitySnapshot{Capabilities: map[string]scheduler.CapabilityCapacity{
        "shell": {Accessible: true, Enforcement: domain.EnforcementEnforced},
    }})
    if err != nil {
        t.Fatal(err)
    }
    if got.Outcome != scheduler.StepIdle {
        t.Fatalf("outcome = %q, want IDLE", got.Outcome)
    }
    if got.TaskID != "" || got.AttemptID != "" {
        t.Fatalf("idle step leased task %q attempt %q", got.TaskID, got.AttemptID)
    }
}
```

- [ ] **Step 2: Run it, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/scheduler -run TestStepOnceIdleWhenNoCandidate -count=1`. Expected: FAIL (`undefined: scheduler.NewWorker`).
- [ ] **Step 3: Implement the skeleton.** In `internal/scheduler/worker.go`:

```go
package scheduler

import (
    "context"
    "errors"
    "sort"
    "time"

    "github.com/SofiaFlux/summa42/internal/clock"
    "github.com/SofiaFlux/summa42/internal/domain"
    "github.com/SofiaFlux/summa42/internal/evidence"
    "github.com/SofiaFlux/summa42/internal/execution"
    "github.com/SofiaFlux/summa42/internal/executors"
    "github.com/SofiaFlux/summa42/internal/verification"
)

type Outcome string

const (
    StepIdle      Outcome = "IDLE"
    StepCompleted Outcome = "COMPLETED"
    StepFailed    Outcome = "FAILED"
)

type StepResult struct {
    Outcome     Outcome
    TaskID      domain.ID
    AttemptID   domain.ID
    EvidenceIDs []domain.ID
}

type Worker struct {
    scheduler     *Service
    execution     *execution.Service
    evidence      *evidence.Store
    verification  *verification.Service
    executors     map[string]executors.Executor
    clock         clock.Clock
    workspaceRoot string
}

func NewWorker(schedulerSvc *Service, executionSvc *execution.Service, evidenceStore *evidence.Store, verificationSvc *verification.Service, registry map[string]executors.Executor, clk clock.Clock, workspaceRoot string) (*Worker, error) {
    if schedulerSvc == nil || executionSvc == nil || evidenceStore == nil || verificationSvc == nil || clk == nil {
        return nil, errors.New("worker requires scheduler, execution, evidence, verification and clock")
    }
    if strings.TrimSpace(workspaceRoot) == "" {
        return nil, errors.New("worker requires a workspace root")
    }
    cleaned := make(map[string]executors.Executor, len(registry))
    for kind, executor := range registry {
        kind = strings.TrimSpace(kind)
        if kind != "" && executor != nil {
            cleaned[kind] = executor
        }
    }
    if len(cleaned) == 0 {
        return nil, errors.New("worker requires at least one executor")
    }
    return &Worker{scheduler: schedulerSvc, execution: executionSvc, evidence: evidenceStore, verification: verificationSvc, executors: cleaned, clock: clk, workspaceRoot: workspaceRoot}, nil
}

func (w *Worker) registryKinds() []string {
    kinds := make([]string, 0, len(w.executors))
    for kind := range w.executors {
        kinds = append(kinds, kind)
    }
    sort.Strings(kinds)
    return kinds
}

func (w *Worker) StepOnce(ctx context.Context, capacity CapacitySnapshot) (StepResult, error) {
    candidate, err := w.scheduler.Next(ctx, capacity)
    if err != nil {
        return StepResult{}, err
    }
    if candidate == nil {
        return StepResult{Outcome: StepIdle}, nil
    }
    return StepResult{}, errors.New("not implemented")
}

func (w *Worker) Run(ctx context.Context, capacity CapacitySnapshot, interval time.Duration) error {
    return errors.New("not implemented")
}
```

The `strings` import is required (`"strings"` alongside the others). `StepOnce` returns `IDLE` on `(nil, nil)` per `Next` semantics; the lease path arrives in Task 2.

- [ ] **Step 4: Run the test, expect PASS.** Same command as Step 2. Expected: PASS.
- [ ] **Step 5: Commit.** `git add internal/scheduler/worker.go internal/scheduler/worker_test.go && git commit -m "feat: add scheduler worker skeleton with idle step"`.

### Task 2: Success path — lease, execute, persist evidence, complete

**Files:**
- Modify: `internal/scheduler/worker.go`
- Modify: `internal/scheduler/worker_test.go`

**Interfaces:**
- Consumes: `StepOnce`, `NewWorker` from Task 1; `Lease`, `ChooseExecutor` on `*Service`; `executors.AttemptEnvelope` fields; `evidence.Metadata`; `verification.CompletionManifest`.
- Produces: completed-step behavior used by Task 3 (failure mirror) and Task 4 (`Run`).

- [ ] **Step 1: Write the failing success test.** Append to `worker_test.go`:

```go
func TestStepOnceCompletesEligibleTask(t *testing.T) {
    // shared stack + task fixture + capacity from the File structure section
    fake := &fakeExecutor{result: executors.ExecutionResult{ExitCode: 0, Stdout: "review ok", Evidence: []executors.Evidence{{Kind: executors.EvidenceAgentMessage, Content: "clean"}}}}
    worker, err := scheduler.NewWorker(schedSvc, execSvc, evidenceStore, verifySvc,
        map[string]executors.Executor{"shell": fake}, clk, t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    got, err := worker.StepOnce(ctx, capacity)
    if err != nil {
        t.Fatal(err)
    }
    if got.Outcome != scheduler.StepCompleted {
        t.Fatalf("outcome = %q, want COMPLETED", got.Outcome)
    }
    if got.TaskID != task.ID || got.AttemptID == "" {
        t.Fatalf("result = %+v, want task %q with attempt", got, task.ID)
    }
    if len(got.EvidenceIDs) != 2 {
        t.Fatalf("evidence IDs = %v, want stdout + agent message", got.EvidenceIDs)
    }
    if len(fake.seen) != 1 {
        t.Fatalf("executor calls = %d, want 1", len(fake.seen))
    }
    envelope := fake.seen[0]
    if envelope.TaskID != task.ID || envelope.AttemptID != got.AttemptID {
        t.Fatalf("envelope IDs = %q/%q, want %q/%q", envelope.TaskID, envelope.AttemptID, task.ID, got.AttemptID)
    }
    if envelope.Objective != "review the diff" || string(envelope.PayloadJSON) != `{"pr":7}` {
        t.Fatalf("envelope intent = %q %q", envelope.Objective, envelope.PayloadJSON)
    }
    if len(envelope.VisibleCapabilities) != 1 || envelope.VisibleCapabilities[0] != "shell" {
        t.Fatalf("visible capabilities = %v, want [shell]", envelope.VisibleCapabilities)
    }
    if envelope.ResourceEnvelopeID != task.ResourceEnvelopeID {
        t.Fatalf("resource envelope = %q, want %q", envelope.ResourceEnvelopeID, task.ResourceEnvelopeID)
    }
    if info, err := os.Stat(envelope.Workspace); err != nil || !info.IsDir() {
        t.Fatalf("workspace %q is not a directory: %v", envelope.Workspace, err)
    }
    reloaded, err := execSvc.Task(ctx, task.ID)
    if err != nil {
        t.Fatal(err)
    }
    if reloaded.State != domain.TaskAwaitingVerification {
        t.Fatalf("task state = %q, want AWAITING_VERIFICATION", reloaded.State)
    }
}
```

Imports needed in the test file: `context`, `encoding/json`, `os`, `testing`, `time`, `domain`, `evidence`, `execution`, `executors`, `purpose`, `resources`, `scheduler`, `testutil`, `verification`.

- [ ] **Step 2: Run it, expect FAIL.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/scheduler -run TestStepOnceCompletesEligibleTask -count=1`. Expected: FAIL with `outcome = "FAILED"` or `not implemented` (the skeleton errors after `Next` finds the candidate).
- [ ] **Step 3: Implement the success path.** Replace the `StepOnce` tail in `worker.go` (keep the `Next`/idle head):

```go
    kind, err := w.scheduler.ChooseExecutor(ctx, candidate.Task, w.registryKinds())
    if err != nil {
        return StepResult{}, err
    }
    executor, ok := w.executors[kind]
    if !ok {
        return StepResult{}, fmt.Errorf("executor %q is not registered", kind)
    }
    attempt, err := w.scheduler.Lease(ctx, candidate.Task.ID, kind)
    if err != nil {
        return StepResult{}, err
    }
    result := StepResult{Outcome: StepFailed, TaskID: candidate.Task.ID, AttemptID: attempt.ID}
    if err := w.executeAttempt(ctx, executor, kind, candidate.Task, attempt, &result); err != nil {
        return StepResult{}, err
    }
    return result, nil
```

Add the helpers (same file):

```go
func (w *Worker) executeAttempt(ctx context.Context, executor executors.Executor, kind string, task domain.Task, attempt domain.Attempt, result *StepResult) (err error) {
    workspace := filepath.Join(w.workspaceRoot, string(attempt.ID))
    if err := os.MkdirAll(workspace, 0o700); err != nil {
        return err
    }
    envelope := executors.AttemptEnvelope{
        TaskID: task.ID, AttemptID: attempt.ID,
        Objective: task.Objective, PayloadJSON: task.PayloadJSON,
        AcceptanceCriteria: append([]string(nil), task.AcceptanceCriteria...),
        Workspace: workspace, VisibleCapabilities: append([]string(nil), task.RequiredCapabilities...),
        ResourceEnvelopeID: task.ResourceEnvelopeID,
    }
    execCtx, cancel := context.WithTimeout(ctx, w.scheduler.leaseDuration)
    defer cancel()
    outcome, execErr := w.runExecutor(execCtx, executor, envelope)
    if execErr != nil || outcome.ExitCode != 0 {
        return w.failExecution(ctx, executor, kind, task, attempt, result, outcome, execErr)
    }
    return w.completeExecution(ctx, task, attempt, result, outcome)
}

func (w *Worker) runExecutor(ctx context.Context, executor executors.Executor, envelope executors.AttemptEnvelope) (result executors.ExecutionResult, err error) {
    defer func() {
        if recovered := recover(); recovered != nil {
            result = executors.ExecutionResult{}
            err = fmt.Errorf("executor panic: %v", recovered)
        }
    }()
    return executor.Start(ctx, envelope)
}

func (w *Worker) persistEvidence(ctx context.Context, outcome executors.ExecutionResult) ([]domain.ID, error) {
    type blob struct {
        content string
        media   string
        kind    string
    }
    blobs := make([]blob, 0, len(outcome.Evidence)+2)
    if outcome.Stdout != "" {
        blobs = append(blobs, blob{outcome.Stdout, "text/plain", string(executors.EvidenceStdout)})
    }
    if outcome.Stderr != "" {
        blobs = append(blobs, blob{outcome.Stderr, "text/plain", string(executors.EvidenceStderr)})
    }
    for _, item := range outcome.Evidence {
        if item.Content == "" {
            continue
        }
        blobs = append(blobs, blob{item.Content, "text/plain", string(item.Kind)})
    }
    ids := make([]domain.ID, 0, len(blobs))
    for _, b := range blobs {
        object, err := w.evidence.Put(ctx, strings.NewReader(b.content), evidence.Metadata{MediaType: b.media, Kind: b.kind})
        if err != nil {
            return nil, err
        }
        ids = append(ids, object.ID)
    }
    return ids, nil
}

func (w *Worker) completeExecution(ctx context.Context, task domain.Task, attempt domain.Attempt, result *StepResult, outcome executors.ExecutionResult) error {
    ids, err := w.persistEvidence(ctx, outcome)
    if err != nil {
        return err
    }
    if len(ids) == 0 {
        return w.failExecution(ctx, nil, "", task, attempt, result, outcome, errors.New("executor returned no evidence"))
    }
    if _, err := w.verification.CompleteAttempt(ctx, attempt.ID, verification.CompletionManifest{EvidenceIDs: ids}); err != nil {
        return err
    }
    result.Outcome = StepCompleted
    result.EvidenceIDs = ids
    return nil
}

func (w *Worker) failExecution(ctx context.Context, _ executors.Executor, kind string, task domain.Task, attempt domain.Attempt, result *StepResult, outcome executors.ExecutionResult, execErr error) error {
    ids, err := w.persistEvidence(ctx, outcome)
    if err != nil {
        return err
    }
    signature := "worker:" + kind + ":" + string(task.ID)
    if execErr != nil && strings.HasPrefix(execErr.Error(), "executor panic:") {
        signature = "worker:panic:" + kind + ":" + string(task.ID)
    }
    if err := w.execution.FailAttempt(ctx, attempt.ID, domain.FailureExecution, signature, ids); err != nil {
        return err
    }
    result.Outcome = StepFailed
    result.EvidenceIDs = ids
    return nil
}
```

New imports for `worker.go`: `fmt`, `os`, `path/filepath` (plus existing `context`, `errors`, `sort`, `strings`, `time` — `time` is used by `Run` in Task 4; keep it imported only when used, so add `time` in Task 4). Guard rejections from `CompleteAttempt`/`FailAttempt` propagate as step errors here; the tolerated-drop behavior is specified in Task 3.

- [ ] **Step 4: Run the scheduler tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/scheduler -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `git add internal/scheduler/worker.go internal/scheduler/worker_test.go && git commit -m "feat: complete worker lease-execute-evidence steps"`.

### Task 3: Failure semantics — errors, panics, guard rejection

**Files:**
- Modify: `internal/scheduler/worker.go`
- Modify: `internal/scheduler/worker_test.go`

**Interfaces:**
- Consumes: `StepOnce`, failure mapping from Task 2; `domain.FailureExecution`, `domain.TaskEligible`, `domain.TaskBlocked`.
- Produces: hardened `StepOnce` used by Task 4.

- [ ] **Step 1: Write the failing failure tests.** Append to `worker_test.go`:

```go
func TestStepOnceFailsExecutorErrorAndBlocksRepeatSignature(t *testing.T) {
    // shared stack + task fixture + capacity
    fake := &fakeExecutor{err: errors.New("provider timeout")}
    worker, err := scheduler.NewWorker(schedSvc, execSvc, evidenceStore, verifySvc,
        map[string]executors.Executor{"shell": fake}, clk, t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    first, err := worker.StepOnce(ctx, capacity)
    if err != nil {
        t.Fatal(err)
    }
    if first.Outcome != scheduler.StepFailed {
        t.Fatalf("outcome = %q, want FAILED", first.Outcome)
    }
    reloaded, err := execSvc.Task(ctx, task.ID)
    if err != nil {
        t.Fatal(err)
    }
    if reloaded.State != domain.TaskEligible {
        t.Fatalf("state after first failure = %q, want ELIGIBLE", reloaded.State)
    }
    var signature string
    if err := store.DB().QueryRowContext(ctx,
        `SELECT signature FROM attempt_failures WHERE task_id = ?`, task.ID).Scan(&signature); err != nil {
        t.Fatal(err)
    }
    if signature != "worker:shell:"+string(task.ID) {
        t.Fatalf("signature = %q", signature)
    }
    second, err := worker.StepOnce(ctx, capacity)
    if err != nil {
        t.Fatal(err)
    }
    if second.Outcome != scheduler.StepFailed {
        t.Fatalf("second outcome = %q, want FAILED", second.Outcome)
    }
    reloaded, err = execSvc.Task(ctx, task.ID)
    if err != nil {
        t.Fatal(err)
    }
    if reloaded.State != domain.TaskBlocked {
        t.Fatalf("state after repeat failure = %q, want BLOCKED", reloaded.State)
    }
}

func TestStepOnceRecoversExecutorPanic(t *testing.T) {
    // shared stack + task fixture + capacity
    panicking := &panicExecutor{}
    worker, err := scheduler.NewWorker(schedSvc, execSvc, evidenceStore, verifySvc,
        map[string]executors.Executor{"shell": panicking}, clk, t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    got, err := worker.StepOnce(ctx, capacity)
    if err != nil {
        t.Fatalf("panic escaped the step: %v", err)
    }
    if got.Outcome != scheduler.StepFailed {
        t.Fatalf("outcome = %q, want FAILED", got.Outcome)
    }
    var signature string
    if err := store.DB().QueryRowContext(ctx,
        `SELECT signature FROM attempt_failures WHERE task_id = ?`, task.ID).Scan(&signature); err != nil {
        t.Fatal(err)
    }
    if signature != "worker:panic:shell:"+string(task.ID) {
        t.Fatalf("signature = %q", signature)
    }
}

type panicExecutor struct{}

func (panicExecutor) Start(context.Context, executors.AttemptEnvelope) (executors.ExecutionResult, error) {
    panic("boom")
}
```

`errors` must be imported in the test file.

- [ ] **Step 2: Run them, expect PASS for both.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/scheduler -run 'TestStepOnce(FailsExecutorErrorAndBlocksRepeatSignature|RecoversExecutorPanic)$' -count=1`. Expected: PASS — panic recovery already lives in `runExecutor` from Task 2, so the panic test passes immediately. If the panic test FAILs (panic escapes), stop: `executeAttempt` is not wired through `runExecutor`.
- [ ] **Step 3: Verify recovery is in place, no new production code.** Confirm `runExecutor` contains the `recover()` branch from Task 2; do not duplicate it.
- [ ] **Step 4: Add guard-rejection tolerance.** Change `completeExecution` and `failExecution` so a stale/lease guard rejection does not abort the loop with a raw error: on `CompleteAttempt`/`FailAttempt` error matching `domain.ErrStaleAttempt` or `domain.ErrLeaseInactive` (both defined in `internal/domain/errors.go`), set `result.Outcome = StepFailed`, keep already-persisted IDs, and return nil. Any other error still returns.

```go
    if _, err := w.verification.CompleteAttempt(ctx, attempt.ID, verification.CompletionManifest{EvidenceIDs: ids}); err != nil {
        if errors.Is(err, domain.ErrStaleAttempt) || errors.Is(err, domain.ErrLeaseInactive) {
            result.Outcome = StepFailed
            result.EvidenceIDs = ids
            return nil
        }
        return err
    }
```

Mirror the same branch in `failExecution` around `FailAttempt`. Add a test `TestStepOnceToleratesStaleLeaseOnComplete`: lease the task manually via `schedSvc.Lease`, advance the test clock past the one-minute lease with `clk.Advance(2 * time.Minute)` (`testutil.Clock` has `Advance`), then `StepOnce` must return `FAILED` without error.

- [ ] **Step 5: Run scheduler tests, expect PASS.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/scheduler -count=1`. Expected: PASS.
- [ ] **Step 6: Commit.** `git add internal/scheduler/worker.go internal/scheduler/worker_test.go && git commit -m "feat: harden worker failure and stale-lease paths"`.

### Task 4: Run loop, CLI wiring, full verification

**Files:**
- Modify: `internal/scheduler/worker.go`
- Modify: `internal/scheduler/worker_test.go`
- Modify: `cmd/summa42-box/main.go`
- Create: `cmd/summa42-box/worker_flags_test.go`

**Interfaces:**
- Consumes: `StepOnce`/`StepResult` from Tasks 1–3; `runtime.Open`/`Box` fields (`Scheduler`, `Execution`, `Evidence`, `Verification`, `Executors`); stdlib `flag`.
- Produces: runnable `run-worker`; no further tasks depend on it.

- [ ] **Step 1: Write the failing Run test.** Append to `worker_test.go`:

```go
func TestRunStopsOnCancellationWithoutNewLease(t *testing.T) {
    // shared stack + task fixture, but NO task created: store stays empty
    worker, err := scheduler.NewWorker(schedSvc, execSvc, evidenceStore, verifySvc,
        map[string]executors.Executor{"shell": &fakeExecutor{}}, clk, t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    if err := worker.Run(ctx, capacity, time.Millisecond); err != nil {
        t.Fatalf("Run on cancelled context = %v, want nil", err)
    }
}

func TestRunStepsOnceThenStops(t *testing.T) {
    // shared stack + task fixture + capacity; fake returns ExitCode 0 with Stdout "ok"
    worker, err := scheduler.NewWorker(schedSvc, execSvc, evidenceStore, verifySvc,
        map[string]executors.Executor{"shell": fake}, clk, t.TempDir())
    if err != nil {
        t.Fatal(err)
    }
    ctx, cancel := context.WithCancel(context.Background())
    done := make(chan error, 1)
    go func() { done <- worker.Run(ctx, capacity, time.Millisecond) }()
    deadline := time.After(10 * time.Second)
    for {
        reloaded, err := execSvc.Task(ctx, task.ID)
        if err != nil {
            t.Fatal(err)
        }
        if reloaded.State == domain.TaskAwaitingVerification {
            break
        }
        select {
        case <-deadline:
            t.Fatal("worker did not complete the task")
        case <-time.After(time.Millisecond):
        }
    }
    cancel()
    select {
    case err := <-done:
        if err != nil {
            t.Fatalf("Run = %v, want nil", err)
        }
    case <-time.After(10 * time.Second):
        t.Fatal("Run did not stop after cancel")
    }
    if len(fake.seen) != 1 {
        t.Fatalf("executor calls = %d, want exactly 1", len(fake.seen))
    }
}
```

- [ ] **Step 2: Run them, expect compile failure.** Run: `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/scheduler -run 'TestRun(StopsOnCancellationWithoutNewLease|StepsOnceThenStops)$' -count=1`. Expected: FAIL (`undefined: Worker.Run` behavior — `Run` currently returns `not implemented`).
- [ ] **Step 3: Implement `Run`.** In `worker.go` (add the `time` import):

```go
func (w *Worker) Run(ctx context.Context, capacity CapacitySnapshot, interval time.Duration) error {
    if interval <= 0 {
        return errors.New("worker requires a positive poll interval")
    }
    if err := w.stepGuarded(ctx, capacity); err != nil {
        return err
    }
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            if err := w.stepGuarded(ctx, capacity); err != nil {
                return err
            }
        }
    }
}

func (w *Worker) stepGuarded(ctx context.Context, capacity CapacitySnapshot) error {
    if err := ctx.Err(); err != nil {
        return nil
    }
    _, err := w.StepOnce(ctx, capacity)
    return err
}
```

Clean shutdown returns nil even when `ctx` is cancelled; a `StepOnce` error aborts the loop visibly so a supervisor restarts the process.

- [ ] **Step 4: Wire the CLI.** In `cmd/summa42-box/main.go`: dispatch in `main()` before `run(ctx)`:

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    if len(os.Args) > 1 && os.Args[1] == "run-worker" {
        if err := runWorker(ctx, os.Args[2:]); err != nil {
            fmt.Fprintln(os.Stderr, err)
            os.Exit(1)
        }
        return
    }
    if err := run(ctx); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Add the flag helper and runner (same file):

```go
func parseWorkerFlags(args []string) (pollInterval time.Duration, leaseDuration time.Duration, err error) {
    flags := flag.NewFlagSet("run-worker", flag.ContinueOnError)
    flags.DurationVar(&pollInterval, "poll-interval", 30*time.Second, "interval between scheduler polls")
    flags.DurationVar(&leaseDuration, "lease-duration", 0, "attempt lease duration (0 uses Box default)")
    if err := flags.Parse(args); err != nil {
        return 0, 0, err
    }
    if pollInterval <= 0 {
        return 0, 0, errors.New("run-worker requires a positive --poll-interval")
    }
    if leaseDuration < 0 {
        return 0, 0, errors.New("run-worker requires a non-negative --lease-duration")
    }
    return pollInterval, leaseDuration, nil
}
```

`runWorker` mirrors `run()` through `Open` (same `localconfig` load, startup material, feedback sink, ADO provider, `CapabilityProviders`) but skips the control server: after `box.Close` defer, build capacity as an empty-capability snapshot only if that matches the deployment (otherwise return an explicit error naming the missing capability source — do not silently run with an empty map), construct the worker with `scheduler.NewWorker(box.Scheduler, box.Execution, box.Evidence, box.Verification, box.Executors, box.Clock, workspaceRoot)`, and call `worker.Run(ctx, capacity, pollInterval)`. `workspaceRoot` comes from a `--workspace-root` flag defaulting to `<EvidencePath>/../workspaces`? No — do not invent path joins: add `--workspace-root` with no default and return an error when empty, letting `NewWorker` validation enforce it. If `runtime.Config.LeaseDuration` must be honored when `--lease-duration` is set, thread it through the `Open` config before constructing the box.

- [ ] **Step 5: Write the flag tests.** In `cmd/summa42-box/worker_flags_test.go` (`package main`):

```go
func TestParseWorkerFlagsDefaults(t *testing.T) {
    poll, lease, err := parseWorkerFlags([]string{})
    if err != nil {
        t.Fatal(err)
    }
    if poll != 30*time.Second || lease != 0 {
        t.Fatalf("flags = %v %v, want 30s 0", poll, lease)
    }
}

func TestParseWorkerFlagsRejectsNonPositivePoll(t *testing.T) {
    if _, _, err := parseWorkerFlags([]string{"--poll-interval=0"}); err == nil {
        t.Fatal("expected error for zero poll interval")
    }
}

func TestParseWorkerFlagsAcceptsOverrides(t *testing.T) {
    poll, lease, err := parseWorkerFlags([]string{"--poll-interval=5s", "--lease-duration=2m"})
    if err != nil {
        t.Fatal(err)
    }
    if poll != 5*time.Second || lease != 2*time.Minute {
        t.Fatalf("flags = %v %v, want 5s 2m", poll, lease)
    }
}
```

- [ ] **Step 6: Run everything.** Run in order, all must pass:
  - `GOCACHE=/tmp/summa42-full-go-cache go test ./internal/scheduler -count=1` (expect PASS)
  - `GOCACHE=/tmp/summa42-full-go-cache go test ./cmd/... -count=1` (expect PASS)
  - `GOCACHE=/tmp/summa42-full-go-cache go test ./... -count=1` (expect all `ok`)
  - `go vet ./...` (expect exit 0), `git diff --check` (expect clean), `gofmt -l` on touched files (expect no output)
- [ ] **Step 7: Commit.** `git add internal/scheduler/worker.go internal/scheduler/worker_test.go cmd/summa42-box/main.go cmd/summa42-box/worker_flags_test.go && git commit -m "feat: add scheduler run loop and run-worker command"`.

## Self-review

- Spec coverage: envelope fields/derivation → Task 2; evidence mapping + empty-output edge → Task 2 `persistEvidence`/`completeExecution`; failure table + panic → Tasks 2–3; at-most-once/guarded + orphan tolerance → Task 3 Step 4; `(nil,nil)`→Idle → Task 1; signatures/capacity-per-tick/static config → Tasks 1/4 (`Run` takes capacity; static-config rebuild is the caller's one-liner, and the CLI refuses silent empty maps); preference determinism → Task 1 (nil preference); `VisibleCapabilities`/no-operations → Task 2 envelope + acceptance via tests; CLI `os.Args` dispatch → Task 4; acceptance criteria → Tasks 2–4 tests.
- No placeholders: every step names exact files, code and commands. Sentinel names (`domain.ErrStaleAttempt`, `domain.ErrLeaseInactive`) and `testutil.Clock.Advance` were verified against the codebase before writing.
- Type consistency: `Outcome`/`StepResult`/`NewWorker`/`StepOnce`/`Run` signatures are identical across Tasks 1–4; `CapacitySnapshot`, `CapacityCapacity`, `CompletionManifest`, `Metadata`, `FailureExecution` match the codebase.
