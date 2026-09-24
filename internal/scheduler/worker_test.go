package scheduler_test

import (
	"context"
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
