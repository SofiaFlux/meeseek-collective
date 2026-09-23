package workflowcase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/purpose"
	"github.com/SofiaFlux/summa42/internal/testutil"
	"github.com/SofiaFlux/summa42/internal/workflow"
)

func setupEnsure(t *testing.T) (*Service, *purpose.Service, domain.ID, context.Context) {
	t.Helper()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	ctx := context.Background()
	missionID, err := purposes.CreateMission(ctx, "Review incoming work")
	if err != nil {
		t.Fatal(err)
	}
	return New(store, clk, purposes), purposes, missionID, ctx
}

func sampleObservation(missionID domain.ID) Observation {
	return Observation{
		MissionID: missionID, Source: "source-a", ObjectID: "item-1", RevisionID: "r1", EvidenceID: "e1",
		FirstWork: workflow.WorkProposal{Kind: "inspect", RequiredCapabilities: []string{"read"}, AuthorityCeiling: []string{"read"}},
		Grant:     workflow.Grant{Capabilities: []string{"read"}}, MaxSteps: 3, RemainingBudget: 5,
	}
}

func TestEnsureCaseDeduplicatesObservationRevision(t *testing.T) {
	svc, _, missionID, ctx := setupEnsure(t)
	request := sampleObservation(missionID)
	first, err := svc.Ensure(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.CurrentWorkID == "" || first.State != Active {
		t.Fatalf("invalid initial case: %+v", first)
	}
	if first.MissionID != missionID || first.NextWork.Kind != "inspect" || first.RemainingBudget != 5 {
		t.Fatalf("case did not retain request: %+v", first)
	}
	replay, err := svc.Ensure(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != first.ID || replay.CurrentWorkID != first.CurrentWorkID {
		t.Fatalf("replay changed case or work ID: first=%+v replay=%+v", first, replay)
	}
	var count int
	if err := svc.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM workflow_cases").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("same revision created %d cases, want 1", count)
	}
	request.RevisionID = "r2"
	next, err := svc.Ensure(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == first.ID || next.CurrentWorkID == first.CurrentWorkID {
		t.Fatalf("new revision reused case or work ID: first=%+v next=%+v", first, next)
	}
}

func TestEnsureCaseRejectsInactiveMissionAndGrantChange(t *testing.T) {
	svc, purposes, missionID, ctx := setupEnsure(t)
	request := sampleObservation(missionID)
	first, err := svc.Ensure(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	request.Grant.Capabilities = []string{"read", "write"}
	if _, err := svc.Ensure(ctx, request); err == nil {
		t.Fatal("changed grant on same observation key was accepted")
	}
	var grantJSON string
	if err := svc.store.DB().QueryRowContext(ctx, "SELECT grant_json FROM workflow_cases WHERE case_id = ?", first.ID).Scan(&grantJSON); err != nil {
		t.Fatal(err)
	}
	if grantJSON != `{"Capabilities":["read"],"Actions":null}` {
		t.Fatalf("stored grant changed: %s", grantJSON)
	}
	if err := purposes.DeactivateMission(ctx, missionID); err != nil {
		t.Fatal(err)
	}
	request = sampleObservation(missionID)
	request.RevisionID = "r2"
	if _, err := svc.Ensure(ctx, request); !errors.Is(err, domain.ErrInvalidPurpose) {
		t.Fatalf("inactive Mission: got %v, want ErrInvalidPurpose", err)
	}
}

func TestEnsureCaseRejectsInvalidInputAndProposal(t *testing.T) {
	svc, _, missionID, ctx := setupEnsure(t)
	base := sampleObservation(missionID)
	tests := []struct {
		name string
		edit func(*Observation)
	}{
		{"blank mission", func(o *Observation) { o.MissionID = " " }},
		{"blank source", func(o *Observation) { o.Source = " " }},
		{"blank object", func(o *Observation) { o.ObjectID = " " }},
		{"blank revision", func(o *Observation) { o.RevisionID = " " }},
		{"blank evidence", func(o *Observation) { o.EvidenceID = " " }},
		{"blank first work", func(o *Observation) { o.FirstWork.Kind = " " }},
		{"zero steps", func(o *Observation) { o.MaxSteps = 0 }},
		{"zero budget", func(o *Observation) { o.RemainingBudget = 0 }},
		{"unauthorized capability", func(o *Observation) { o.FirstWork.AuthorityCeiling = []string{"write"} }},
		{"unauthorized action", func(o *Observation) { o.FirstWork.ProposedActions = []string{"comment"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := base
			tt.edit(&request)
			if _, err := svc.Ensure(ctx, request); err == nil {
				t.Fatal("invalid observation was accepted")
			}
		})
	}
}
