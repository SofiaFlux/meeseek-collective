package scheduler_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/purpose"
	"github.com/SofiaFlux/summa42/internal/resources"
	"github.com/SofiaFlux/summa42/internal/scheduler"
	"github.com/SofiaFlux/summa42/internal/testutil"
	"github.com/SofiaFlux/summa42/internal/verification"
)

type fakeExecutor struct {
	result executors.ExecutionResult
	err    error
	seen   []executors.AttemptEnvelope
}

func (f *fakeExecutor) Start(ctx context.Context, envelope executors.AttemptEnvelope) (executors.ExecutionResult, error) {
	f.seen = append(f.seen, envelope)
	return f.result, f.err
}

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

func TestStepOnceCompletesEligibleTask(t *testing.T) {
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
		PayloadJSON:          json.RawMessage(`{"pr": 7}`),
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

func TestStepOnceFailsExecutorErrorAndBlocksRepeatSignature(t *testing.T) {
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
		PayloadJSON:          json.RawMessage(`{"pr": 7}`),
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
		PayloadJSON:          json.RawMessage(`{"pr": 7}`),
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

type advancingExecutor struct {
	result executors.ExecutionResult
	clk    *testutil.Clock
}

func (a *advancingExecutor) Start(_ context.Context, _ executors.AttemptEnvelope) (executors.ExecutionResult, error) {
	a.clk.Advance(2 * time.Minute)
	return a.result, nil
}

func TestStepOnceToleratesStaleLeaseOnComplete(t *testing.T) {
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
		PayloadJSON:          json.RawMessage(`{"pr": 7}`),
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
	exec := &advancingExecutor{
		result: executors.ExecutionResult{ExitCode: 0, Stdout: "review ok", Evidence: []executors.Evidence{{Kind: executors.EvidenceAgentMessage, Content: "clean"}}},
		clk:    clk,
	}
	worker, err := scheduler.NewWorker(schedSvc, execSvc, evidenceStore, verifySvc,
		map[string]executors.Executor{"shell": exec}, clk, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := worker.StepOnce(ctx, capacity)
	if err != nil {
		t.Fatalf("stale lease aborted the step: %v", err)
	}
	if got.Outcome != scheduler.StepFailed {
		t.Fatalf("outcome = %q, want FAILED", got.Outcome)
	}
	if got.TaskID != task.ID || got.AttemptID == "" {
		t.Fatalf("result = %+v, want task %q with attempt", got, task.ID)
	}
	if len(got.EvidenceIDs) == 0 {
		t.Fatalf("evidence IDs = %v, want persisted IDs kept", got.EvidenceIDs)
	}
}
