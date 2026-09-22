package wake_test

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
	"github.com/SofiaFlux/summa42/internal/wake"
)

type staleCounter int

func (c staleCounter) StaleCapabilityAssessments(context.Context) (int, error) { return int(c), nil }

func TestDurableWakeupsSurviveServiceRestartAndFireOnce(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC))

	first := wake.New(store, clk, nil)
	dueAt := clk.Now().Add(5 * time.Minute)
	created, err := first.Schedule(ctx, wake.KindTaskReevaluation, "", dueAt)
	if err != nil {
		t.Fatal(err)
	}

	restarted := wake.New(store, clk, nil)
	next, err := restarted.NextDeadline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || !next.Equal(dueAt) {
		t.Fatalf("next deadline = %v, want %v", next, dueAt)
	}

	clk.Advance(5 * time.Minute)
	due, err := restarted.Due(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != created.ID {
		t.Fatalf("due = %#v, want wakeup %s", due, created.ID)
	}
	if err := restarted.Acknowledge(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	again, err := restarted.Due(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("wakeup fired more than once after acknowledgement: %#v", again)
	}
	if next, err := restarted.NextDeadline(ctx); err != nil {
		t.Fatal(err)
	} else if next != nil {
		t.Fatalf("next deadline after firing = %v, want nil", next)
	}
}

func TestRecoverExpiredLeasesRevokesAuthorityAndMakesTaskSchedulableAgain(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	resourceSvc := resources.New(store, clk)
	schedulerSvc := scheduler.New(store, clk, purposes, execSvc, resourceSvc, time.Minute)
	wakeSvc := wake.New(store, clk, schedulerSvc)

	envelopeID := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, 100, ?)`,
		envelopeID, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	task, err := execSvc.CreateTask(ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: "owner-retry"},
		AcceptanceCriteria:   []string{"done"},
		RequiredCapabilities: []string{"shell"},
		RequiredEnforcement:  domain.EnforcementEnforced,
		AuthorityCeiling:     []string{"shell"},
		ResourceEnvelopeID:   envelopeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := execSvc.StartAttempt(ctx, task.ID, "shell", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(2 * time.Minute)

	recovered, err := wakeSvc.RecoverExpiredLeases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}

	var leaseState domain.LeaseState
	var attemptState domain.AttemptState
	if err := store.DB().QueryRowContext(ctx,
		`SELECT lease_state, state FROM attempts WHERE attempt_id = ?`, attempt.ID,
	).Scan(&leaseState, &attemptState); err != nil {
		t.Fatal(err)
	}
	if leaseState != domain.LeaseExpired || attemptState != domain.AttemptExpired {
		t.Fatalf("attempt after recovery = lease %s state %s", leaseState, attemptState)
	}

	capacity := scheduler.CapacitySnapshot{Capabilities: map[string]scheduler.CapabilityCapacity{
		"shell": {Accessible: true, Enforcement: domain.EnforcementEnforced},
	}}
	candidate, err := schedulerSvc.Next(ctx, capacity)
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil || candidate.Task.ID != task.ID {
		t.Fatalf("candidate after expiry recovery = %#v, want task %s", candidate, task.ID)
	}
	replacement, err := schedulerSvc.Lease(ctx, task.ID, "shell")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.FenceGeneration <= attempt.FenceGeneration {
		t.Fatalf("replacement fence = %d, stale fence = %d", replacement.FenceGeneration, attempt.FenceGeneration)
	}
}

func TestStrategicPulseKeepsEmptyCollectiveDormantAndSchedulesOneNextPulse(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 11, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	resourceSvc := resources.New(store, clk)
	schedulerSvc := scheduler.New(store, clk, purposes, execSvc, resourceSvc, time.Minute)
	wakeSvc := wake.New(store, clk, schedulerSvc)
	interval := 15 * time.Minute

	result, err := wakeSvc.StrategicPulse(ctx, scheduler.CapacitySnapshot{}, interval)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Dormant || result.AuthorizedWork || result.BlockedTasks != 0 || result.DueObligations != 0 || result.KnownUnknowns != 0 {
		t.Fatalf("empty Collective should remain dormant, got %#v", result)
	}
	if !result.NextPulseAt.Equal(clk.Now().Add(interval)) {
		t.Fatalf("next pulse = %v, want %v", result.NextPulseAt, clk.Now().Add(interval))
	}

	if _, err := wakeSvc.StrategicPulse(ctx, scheduler.CapacitySnapshot{}, interval); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM scheduled_wakeups WHERE kind = ? AND state = 'PENDING'`, wake.KindStrategicPulse,
	).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("pending strategic pulses = %d, want exactly 1", pending)
	}
}

func TestStrategicPulseSeesBlockedWorkAsMeaningfulSignal(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	resourceSvc := resources.New(store, clk)
	schedulerSvc := scheduler.New(store, clk, purposes, execSvc, resourceSvc, time.Minute)
	wakeSvc := wake.New(store, clk, schedulerSvc)

	envelopeID := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, 100, ?)`,
		envelopeID, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	task, err := execSvc.CreateTask(ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: "owner-blocked"},
		AcceptanceCriteria:   []string{"done"},
		RequiredCapabilities: []string{"shell"},
		RequiredEnforcement:  domain.EnforcementEnforced,
		AuthorityCeiling:     []string{"shell"},
		ResourceEnvelopeID:   envelopeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx,
		`UPDATE tasks SET state = ? WHERE task_id = ?`, domain.TaskBlocked, task.ID,
	); err != nil {
		t.Fatal(err)
	}

	result, err := wakeSvc.StrategicPulse(ctx, scheduler.CapacitySnapshot{Capabilities: map[string]scheduler.CapabilityCapacity{
		"shell": {Accessible: true, Enforcement: domain.EnforcementEnforced},
	}}, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if result.Dormant {
		t.Fatal("blocked work should wake Strategic Pulse orientation")
	}
	if result.BlockedTasks != 1 {
		t.Fatalf("blocked tasks = %d, want 1", result.BlockedTasks)
	}
	if result.AuthorizedWork {
		t.Fatal("BLOCKED work must not be reported as directly schedulable authorized work")
	}
}

func TestStrategicPulseConsumesCapabilityFreshnessSignal(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 13, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	resourceSvc := resources.New(store, clk)
	schedulerSvc := scheduler.New(store, clk, purposes, execSvc, resourceSvc, time.Minute)
	wakeSvc := wake.New(store, clk, schedulerSvc, staleCounter(2))

	result, err := wakeSvc.StrategicPulse(ctx, scheduler.CapacitySnapshot{}, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if result.StaleCapabilityAssessments != 2 {
		t.Fatalf("stale capability assessments = %d, want 2", result.StaleCapabilityAssessments)
	}
	if result.Dormant {
		t.Fatal("stale capability assessments are a meaningful orientation signal")
	}
}
