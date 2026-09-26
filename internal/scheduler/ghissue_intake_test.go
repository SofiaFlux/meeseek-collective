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

// The Task is gated by capability, not by task class: once an executor
// advertises github.issue.read under ENFORCED, the follow-on triage slice can
// lease it. Without this, the test above also passes for a Task the scheduler
// would never select for any reason.
func TestNextClaimsGitHubIssueTriageWithEnforcedReadCapability(t *testing.T) {
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
	enforced := scheduler.CapacitySnapshot{Capabilities: map[string]scheduler.CapabilityCapacity{
		"github.issue.read": {Accessible: true, Enforcement: domain.EnforcementEnforced},
	}}
	candidate, err := svc.Next(ctx, enforced)
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil {
		t.Fatalf("task %s is unclaimable even with enforced github.issue.read capacity", task.ID)
	}
	if candidate.Task.ID != task.ID {
		t.Fatalf("candidate %s, want %s", candidate.Task.ID, task.ID)
	}
}
