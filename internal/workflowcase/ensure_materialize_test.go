package workflowcase

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
	"github.com/SofiaFlux/summa42/internal/workflow"
)

func ensureFixture(t *testing.T) (context.Context, *state.Store, *Service, *execution.Service, domain.ID, domain.ID) {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	cases := New(store, clk, purposes)
	mission, err := purposes.CreateMission(ctx, "triage incoming issues")
	if err != nil {
		t.Fatal(err)
	}
	envelope := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, ?, ?)`,
		envelope, 100, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	return ctx, store, cases, execSvc, mission, envelope
}

func githubObservation(mission domain.ID) Observation {
	return Observation{
		MissionID: mission, Source: "github", ObjectID: "github:o/r#7",
		RevisionID: "2026-09-24T10:00:00Z", EvidenceID: "evidence-1",
		FirstWork: workflow.WorkProposal{Kind: "github.issue.triage", RequiredCapabilities: []string{"github.issue.read"}, AuthorityCeiling: []string{"github.issue.read"}},
		Grant:     workflow.Grant{Capabilities: []string{"github.issue.read"}}, MaxSteps: 3, RemainingBudget: 10,
	}
}

func triageTemplate(envelope domain.ID) execution.TaskRequest {
	return execution.TaskRequest{
		Objective:          "Triage GitHub issue o/r#7",
		PayloadJSON:        json.RawMessage(`{"issue":7}`),
		AcceptanceCriteria: []string{"triage decision recorded for 2026-09-24T10:00:00Z"},
		ResourceEnvelopeID: envelope,
	}
}

func TestEnsureAndMaterializeCreatesCaseAndTask(t *testing.T) {
	ctx, _, cases, execSvc, mission, envelope := ensureFixture(t)
	created, task, err := cases.EnsureAndMaterialize(ctx, execSvc, githubObservation(mission), triageTemplate(envelope))
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || task.ID == "" {
		t.Fatalf("created=%+v task=%+v", created, task)
	}
	if task.IdempotencyKey != string(created.CurrentWorkID) {
		t.Fatalf("idempotency key = %q, want work %q", task.IdempotencyKey, created.CurrentWorkID)
	}
	if task.TaskClass != "github.issue.triage" {
		t.Fatalf("task class = %q", task.TaskClass)
	}
	if len(task.RequiredCapabilities) != 1 || task.RequiredCapabilities[0] != "github.issue.read" {
		t.Fatalf("capabilities = %v", task.RequiredCapabilities)
	}
	if task.RequiredEnforcement != domain.EnforcementUnenforced {
		t.Fatalf("required enforcement = %q, want %q", task.RequiredEnforcement, domain.EnforcementUnenforced)
	}
	stored, err := cases.Get(ctx, created.ID)
	if err != nil || stored.ObjectID != "github:o/r#7" {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	fetched, err := execSvc.Task(ctx, task.ID)
	if err != nil || fetched.Objective != "Triage GitHub issue o/r#7" {
		t.Fatalf("fetched=%+v err=%v", fetched, err)
	}
}

func TestEnsureAndMaterializeRollsBackCaseWhenTaskInsertFails(t *testing.T) {
	ctx, store, cases, execSvc, mission, envelope := ensureFixture(t)
	template := triageTemplate(envelope)
	template.RequiredEnforcement = domain.EnforcementLevel("BOGUS")
	if _, _, err := cases.EnsureAndMaterialize(ctx, execSvc, githubObservation(mission), template); err == nil {
		t.Fatal("expected task creation error")
	}
	if _, found, err := cases.Find(ctx, mission, "github", "github:o/r#7", "2026-09-24T10:00:00Z"); err != nil || found {
		t.Fatalf("case persisted after task failure: found=%v err=%v", found, err)
	}
	var tasks int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 {
		t.Fatalf("tasks = %d, want 0 after rollback", tasks)
	}
}

func TestEnsureAndMaterializeRejectsMissingServices(t *testing.T) {
	ctx, _, cases, execSvc, mission, envelope := ensureFixture(t)
	if _, _, err := cases.EnsureAndMaterialize(ctx, nil, githubObservation(mission), triageTemplate(envelope)); err == nil {
		t.Fatal("expected error for nil execution service")
	}
	if _, _, err := cases.EnsureAndMaterialize(ctx, execSvc, githubObservation(mission), execution.TaskRequest{}); err == nil {
		t.Fatal("expected error for empty template")
	}
}
