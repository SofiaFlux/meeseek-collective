package workflowcase

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/workflow"
)

func TestMaterializeUsesCaseAuthorityAndStableWorkKey(t *testing.T) {
	svc, purposes, missionID, ctx := setupEnsure(t)
	caseRequest := sampleObservation(missionID)
	caseRequest.Grant = workflow.Grant{Capabilities: []string{"read", "write"}}
	c, err := svc.Ensure(ctx, caseRequest)
	if err != nil {
		t.Fatal(err)
	}
	executionSvc := execution.New(svc.store, svc.clock, purposes)
	template := execution.TaskRequest{
		Purpose:   domain.PurposeRef{Kind: domain.PurposeGovernance, ID: domain.ID("wrong")},
		TaskClass: "wrong", Objective: "Review the current proposal",
		PayloadJSON: json.RawMessage(`{"change":42}`), IdempotencyKey: "wrong",
		AcceptanceCriteria:   []string{"review recorded"},
		RequiredCapabilities: []string{"write"}, AuthorityCeiling: []string{"write"},
		RequiredEnforcement: domain.EnforcementPartial,
		ResourceEnvelopeID:  domain.ID("review-envelope"), Priority: 7,
	}
	first, err := svc.MaterializeTask(ctx, executionSvc, c.ID, c.CurrentWorkID, template)
	if err != nil {
		t.Fatal(err)
	}
	if first.Purpose != (domain.PurposeRef{Kind: domain.PurposeMission, ID: missionID}) ||
		first.TaskClass != "inspect" || first.IdempotencyKey != string(c.CurrentWorkID) ||
		!reflect.DeepEqual(first.RequiredCapabilities, []string{"read"}) ||
		!reflect.DeepEqual(first.AuthorityCeiling, []string{"read"}) {
		t.Fatalf("case authority was not applied: %+v", first)
	}
	if first.Objective != template.Objective || string(first.PayloadJSON) != string(template.PayloadJSON) ||
		!reflect.DeepEqual(first.AcceptanceCriteria, template.AcceptanceCriteria) ||
		first.RequiredEnforcement != template.RequiredEnforcement ||
		first.ResourceEnvelopeID != template.ResourceEnvelopeID || first.Priority != template.Priority {
		t.Fatalf("recipe fields were not preserved: %+v", first)
	}
	replay, err := svc.MaterializeTask(ctx, executionSvc, c.ID, c.CurrentWorkID, template)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay = %+v, %v; want task %s", replay, err, first.ID)
	}
	drift := template
	drift.Objective = "Different review"
	if _, err := svc.MaterializeTask(ctx, executionSvc, c.ID, c.CurrentWorkID, drift); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("same-work drift = %v, want conflict", err)
	}
	next, err := svc.Assess(ctx, AssessmentRequest{CaseID: c.ID, WorkID: c.CurrentWorkID,
		Assessment: workflow.Assessment{Verdict: workflow.Continue, EvidenceIDs: []string{"reviewed"},
			Next: &workflow.WorkProposal{Kind: "revise", RequiredCapabilities: []string{"write"}, AuthorityCeiling: []string{"write"}}},
		RemainingBudget: 4, ProgressSignature: "reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MaterializeTask(ctx, executionSvc, c.ID, c.CurrentWorkID, template); err == nil {
		t.Fatal("stale work was materialized")
	}
	second, err := svc.MaterializeTask(ctx, executionSvc, c.ID, next.Case.CurrentWorkID, template)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.IdempotencyKey != string(next.Case.CurrentWorkID) || second.TaskClass != "revise" ||
		!reflect.DeepEqual(second.RequiredCapabilities, []string{"write"}) {
		t.Fatalf("new work did not create distinct task: first=%+v second=%+v", first, second)
	}
}

func TestMaterializeRejectsInvalidTemplateAndWork(t *testing.T) {
	svc, purposes, missionID, ctx := setupEnsure(t)
	c, err := svc.Ensure(ctx, sampleObservation(missionID))
	if err != nil {
		t.Fatal(err)
	}
	executionSvc := execution.New(svc.store, svc.clock, purposes)
	valid := execution.TaskRequest{Objective: "Review", AcceptanceCriteria: []string{"record result"}, ResourceEnvelopeID: "envelope"}
	tests := []struct {
		name   string
		workID domain.ID
		edit   func(*execution.TaskRequest)
	}{
		{"blank objective", c.CurrentWorkID, func(r *execution.TaskRequest) { r.Objective = "  " }},
		{"blank criteria", c.CurrentWorkID, func(r *execution.TaskRequest) { r.AcceptanceCriteria = []string{"  "} }},
		{"blank envelope", c.CurrentWorkID, func(r *execution.TaskRequest) { r.ResourceEnvelopeID = "  " }},
		{"wrong work", domain.ID("old-work"), func(*execution.TaskRequest) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid
			tt.edit(&r)
			if _, err := svc.MaterializeTask(ctx, executionSvc, c.ID, tt.workID, r); err == nil {
				t.Fatal("invalid request was materialized")
			}
		})
	}
}

func TestMaterializeRejectsCaseAdvancedAfterSnapshotBeforeInsert(t *testing.T) {
	svc, purposes, missionID, ctx := setupEnsure(t)
	c, err := svc.Ensure(ctx, sampleObservation(missionID))
	if err != nil {
		t.Fatal(err)
	}
	executionSvc := execution.New(svc.store, svc.clock, purposes)
	template := execution.TaskRequest{Objective: "Review", AcceptanceCriteria: []string{"record result"}, ResourceEnvelopeID: "envelope"}

	// c is the same snapshot MaterializeTask receives from Get. Advance the
	// durable case before the insertion stage to exercise that exact window.
	_, err = svc.Assess(ctx, AssessmentRequest{CaseID: c.ID, WorkID: c.CurrentWorkID,
		Assessment:      workflow.Assessment{Verdict: workflow.Ready, EvidenceIDs: []string{"reviewed"}},
		RemainingBudget: 4, ProgressSignature: "reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.materializeCurrentCase(ctx, executionSvc, c, template); err == nil {
		t.Fatal("stale snapshot created an eligible Task")
	}
	var count int
	if err := svc.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE idempotency_key = ?", c.CurrentWorkID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale work created %d Task rows, want zero", count)
	}
}
