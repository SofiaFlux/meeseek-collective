package workflowcase

import (
	"testing"

	"github.com/SofiaFlux/summa42/internal/workflow"
)

func TestFindReturnsCaseByIdentity(t *testing.T) {
	svc, _, missionID, ctx := setupEnsure(t)
	obs := Observation{
		MissionID: missionID, Source: "ado", ObjectID: "shop#1", RevisionID: "a:b", EvidenceID: "ev-1",
		FirstWork: workflow.WorkProposal{Kind: "ado.pr.review", RequiredCapabilities: []string{"read"}, AuthorityCeiling: []string{"read"}},
		Grant:     workflow.Grant{Capabilities: []string{"read"}}, MaxSteps: 3, RemainingBudget: 10,
	}
	created, err := svc.Ensure(ctx, obs)
	if err != nil {
		t.Fatal(err)
	}
	found, ok, err := svc.Find(ctx, missionID, "ado", "shop#1", "a:b")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Find missed existing case")
	}
	if found.ID != created.ID {
		t.Fatalf("Find returned %s, want %s", found.ID, created.ID)
	}
	missing, ok, err := svc.Find(ctx, missionID, "ado", "shop#1", "other")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("Find hit unexpected case %+v", missing)
	}
	if missing.ID != "" {
		t.Fatalf("missing case ID = %q, want empty", missing.ID)
	}
}
