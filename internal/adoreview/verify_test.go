package adoreview

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/operations"
	"github.com/SofiaFlux/summa42/internal/verification"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
)

const (
	finalVerifierTestID   domain.ID = "final-verifier"
	finalVerifierTestType           = "AUTOMATED"
)

type finalVerifierFixture struct {
	*driverHarness
	verifier           *FinalVerifier
	caseID             domain.ID
	workID             domain.ID
	taskID             domain.ID
	decisionID         domain.ID
	completionEvidence domain.ID
}

func setupReadyFinalVerifier(t *testing.T, decision ReviewDecision, entries ...publishEvidenceEntry) *finalVerifierFixture {
	t.Helper()
	h := setupDriverHarness(t, decision)
	c, err := h.cases.Get(h.ctx, h.driverCaseID)
	if err != nil {
		t.Fatal(err)
	}
	task := materializeDriverWork2(t, h, c)
	completion := putDriverEvidence(t, h.evidence, driverPublisherContent(t, entries...), "text/plain", executors.EvidenceAgentMessage)
	completeDriverTask(t, h, task, completion.ID)
	if _, err := h.driver.StepOnce(h.ctx); err != nil {
		t.Fatal(err)
	}
	ready, err := h.cases.Get(h.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.State != workflowcase.ReadyForVerification {
		t.Fatalf("fixture case state = %q, want READY_FOR_VERIFICATION", ready.State)
	}
	verifier, err := NewFinalVerifier(h.cases, h.execution, h.evidence, h.verification, FinalVerifierConfig{
		MissionID: h.mission, Comment: h.comment, Vote: h.vote,
		VerifierID: finalVerifierTestID, VerifierType: finalVerifierTestType,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.comment.requests = nil
	h.vote.requests = nil
	return &finalVerifierFixture{
		driverHarness: h, verifier: verifier, caseID: c.ID, workID: c.CurrentWorkID,
		taskID: task.ID, decisionID: h.decision, completionEvidence: completion.ID,
	}
}

func replaceFinalVerifierEvidence(t *testing.T, h *driverHarness, id domain.ID, content string) {
	t.Helper()
	original, _, err := h.evidence.Get(h.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := h.evidence.Put(h.ctx, strings.NewReader(content), evidence.Metadata{
		MediaType: original.MediaType, Kind: original.Kind,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.DB().ExecContext(h.ctx,
		`UPDATE evidence_objects SET content_hash = ?, size_bytes = ? WHERE evidence_id = ?`,
		replacement.ContentHash, replacement.SizeBytes, id,
	); err != nil {
		t.Fatal(err)
	}
}

func finalVerificationRecord(t *testing.T, h *driverHarness, caseID domain.ID) workflowcase.VerificationRecord {
	t.Helper()
	var record workflowcase.VerificationRecord
	var evidenceJSON string
	if err := h.store.DB().QueryRowContext(h.ctx, `
		SELECT verification_id, case_id, verifier_id, verifier_type, snapshot_hash,
		       snapshot_json, evidence_ids_json
		FROM workflow_verifications WHERE case_id = ?`, caseID,
	).Scan(&record.ID, &record.CaseID, &record.VerifierID, &record.VerifierType,
		&record.SnapshotHash, &record.SnapshotJSON, &evidenceJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &record.EvidenceIDs); err != nil {
		t.Fatal(err)
	}
	return record
}

func finalVerificationCount(t *testing.T, h *driverHarness, caseID domain.ID) int {
	t.Helper()
	var count int
	if err := h.store.DB().QueryRowContext(h.ctx,
		`SELECT count(*) FROM workflow_verifications WHERE case_id = ?`, caseID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func finalAcceptance(t *testing.T, h *driverHarness, taskID domain.ID) (domain.ID, string, []domain.ID) {
	t.Helper()
	var verifierID domain.ID
	var verifierType string
	var evidenceJSON string
	if err := h.store.DB().QueryRowContext(h.ctx, `
		SELECT verifier_id, verifier_type, evidence_ids_json
		FROM acceptance_records WHERE task_id = ?`, taskID,
	).Scan(&verifierID, &verifierType, &evidenceJSON); err != nil {
		t.Fatal(err)
	}
	var evidenceIDs []domain.ID
	if err := json.Unmarshal([]byte(evidenceJSON), &evidenceIDs); err != nil {
		t.Fatal(err)
	}
	return verifierID, verifierType, evidenceIDs
}

func finalAcceptanceCount(t *testing.T, h *driverHarness, taskID domain.ID) int {
	t.Helper()
	var count int
	if err := h.store.DB().QueryRowContext(h.ctx,
		`SELECT count(*) FROM acceptance_records WHERE task_id = ?`, taskID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func finalTaskState(t *testing.T, h *driverHarness, workID domain.ID) domain.TaskState {
	t.Helper()
	task, found, err := h.execution.FindByIdempotencyKey(h.ctx, string(workID))
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("Work 2 task %s not found", workID)
	}
	return task.State
}

func requireFinalEvidence(t *testing.T, ids []domain.ID, wanted ...domain.ID) {
	t.Helper()
	for _, want := range wanted {
		index := sort.Search(len(ids), func(index int) bool { return ids[index] >= want })
		if index == len(ids) || ids[index] != want {
			t.Fatalf("evidence IDs = %v, want %s", ids, want)
		}
	}
}

func updateFinalVerifierTaskPayload(t *testing.T, h *driverHarness, taskID domain.ID, update func(*PublishPayload)) {
	t.Helper()
	task, err := h.execution.Task(h.ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	var payload PublishPayload
	if err := json.Unmarshal(task.PayloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	update(&payload)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.DB().ExecContext(h.ctx,
		`UPDATE tasks SET payload_json = ? WHERE task_id = ?`, string(body), taskID,
	); err != nil {
		t.Fatal(err)
	}
}

func updateFinalVerifierAssessmentWorkID(t *testing.T, h *driverHarness, caseID, workID domain.ID) {
	t.Helper()
	records, err := h.cases.ListAssessments(h.ctx, caseID)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.WorkID != string(workID) {
			continue
		}
		var request workflowcase.AssessmentRequest
		if err := json.Unmarshal([]byte(record.RequestJSON), &request); err != nil {
			t.Fatal(err)
		}
		request.WorkID = domain.NewID("mismatched-assessment-work")
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.store.DB().ExecContext(h.ctx,
			`UPDATE workflow_assessments SET request_json = ? WHERE assessment_id = ?`, string(body), record.ID,
		); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatalf("assessment for work %s not found", workID)
}

type gatedLookupProvider struct {
	delegate LookupProvider
	mu       sync.Mutex
	calls    int
	release  chan struct{}
}

func (p *gatedLookupProvider) Name() string { return p.delegate.Name() }

func (p *gatedLookupProvider) LookupOutcome(ctx context.Context, request operations.ProviderDispatchRequest) (operations.ProviderOutcome, error) {
	p.mu.Lock()
	p.calls++
	if p.calls == 2 {
		close(p.release)
	}
	p.mu.Unlock()
	select {
	case <-p.release:
	case <-ctx.Done():
		return operations.ProviderOutcome{}, ctx.Err()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.delegate.LookupOutcome(ctx, request)
}

func TestFinalVerifierClosesConfirmedCase(t *testing.T) {
	f := setupReadyFinalVerifier(t,
		ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
		publishEvidenceEntry{
			Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1",
			State: domain.OperationConfirmedEffect, Reference: "vote-1",
		},
	)

	result, err := f.verifier.StepOnce(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Closed) != 1 || result.Closed[0] != f.caseID {
		t.Fatalf("result = %+v, want closed case %s", result, f.caseID)
	}
	if state := finalTaskState(t, f.driverHarness, f.workID); state != domain.TaskSucceeded {
		t.Fatalf("task state = %q, want SUCCEEDED", state)
	}
	closed, err := f.cases.Get(f.ctx, f.caseID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.State != workflowcase.Closed || closed.CurrentWorkID != "" || closed.NextWork.Kind != "" {
		t.Fatalf("closed case = %+v", closed)
	}
	record := finalVerificationRecord(t, f.driverHarness, f.caseID)
	if record.VerifierID != finalVerifierTestID || record.VerifierType != finalVerifierTestType {
		t.Fatalf("verification identity = %q/%q", record.VerifierID, record.VerifierType)
	}
	if len(record.EvidenceIDs) != 2 {
		t.Fatalf("verification evidence = %v, want decision and completion evidence", record.EvidenceIDs)
	}
	requireFinalEvidence(t, record.EvidenceIDs, f.decisionID, f.completionEvidence)
	for _, id := range record.EvidenceIDs {
		object, _, err := f.evidence.Get(f.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if object.Kind == "ado.workflow.verification" {
			t.Fatalf("verification record cited transient verification evidence %s", id)
		}
	}
	var snapshot struct {
		CaseID   string   `json:"caseID"`
		Revision string   `json:"revision"`
		WorkID   string   `json:"workID"`
		Slots    []string `json:"slots"`
		Verdicts []struct {
			Slot  string                `json:"slot"`
			State domain.OperationState `json:"state"`
		} `json:"verdicts"`
	}
	if err := json.Unmarshal([]byte(record.SnapshotJSON), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.CaseID != string(f.caseID) || snapshot.Revision != "a:b" || snapshot.WorkID != string(f.workID) {
		t.Fatalf("snapshot identity = %+v", snapshot)
	}
	if len(snapshot.Slots) != 1 || snapshot.Slots[0] != "ado.pr.approve:proj/shop#1:a:b" {
		t.Fatalf("snapshot slots = %v", snapshot.Slots)
	}
	if len(snapshot.Verdicts) != 1 || snapshot.Verdicts[0] != (struct {
		Slot  string                `json:"slot"`
		State domain.OperationState `json:"state"`
	}{"ado.pr.approve:proj/shop#1:a:b", domain.OperationConfirmedEffect}) {
		t.Fatalf("snapshot verdicts = %+v", snapshot.Verdicts)
	}
	if len(f.vote.requests) != 1 || len(f.comment.requests) != 0 {
		t.Fatalf("final lookup counts = vote:%d comment:%d", len(f.vote.requests), len(f.comment.requests))
	}
	if count := finalAcceptanceCount(t, f.driverHarness, f.taskID); count != 1 {
		t.Fatalf("acceptance count = %d, want 1", count)
	}
	verifierID, verifierType, acceptanceEvidence := finalAcceptance(t, f.driverHarness, f.taskID)
	if verifierID != finalVerifierTestID || verifierType != finalVerifierTestType {
		t.Fatalf("task acceptance identity = %q/%q, want independent verifier", verifierID, verifierType)
	}
	if !reflect.DeepEqual(acceptanceEvidence, []domain.ID{f.completionEvidence}) {
		t.Fatalf("task acceptance evidence = %v, want completion evidence only", acceptanceEvidence)
	}
	if _, err := f.verifier.StepOnce(f.ctx); err != nil {
		t.Fatal(err)
	}
	if count := finalVerificationCount(t, f.driverHarness, f.caseID); count != 1 {
		t.Fatalf("verification count after replay = %d, want 1", count)
	}
	var active int
	if err := f.store.DB().QueryRowContext(f.ctx,
		`SELECT active FROM missions WHERE mission_id = ?`, f.mission).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("Mission active = %d, want 1", active)
	}
}

func TestFinalVerifierRejectsTamperedDecision(t *testing.T) {
	f := setupReadyFinalVerifier(t,
		ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
		publishEvidenceEntry{
			Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect,
		},
	)
	tampered, err := json.Marshal(ReviewDecision{Action: DecisionCommentAction, Reason: "tampered"})
	if err != nil {
		t.Fatal(err)
	}
	replaceFinalVerifierEvidence(t, f.driverHarness, f.decisionID, string(tampered))

	if _, err := f.verifier.StepOnce(f.ctx); err != nil {
		t.Fatal(err)
	}
	blocked, err := f.cases.Get(f.ctx, f.caseID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.State != workflowcase.Blocked {
		t.Fatalf("case state = %q, want BLOCKED", blocked.State)
	}
	record := finalVerificationRecord(t, f.driverHarness, f.caseID)
	if record.VerifierType != "REJECT" || !strings.Contains(record.SnapshotJSON, "review decision") {
		t.Fatalf("rejection record = %+v", record)
	}
	if state := finalTaskState(t, f.driverHarness, f.workID); state != domain.TaskAwaitingVerification {
		t.Fatalf("task state = %q, want AWAITING_VERIFICATION", state)
	}
}

func TestFinalVerifierHoldsOnUnknownOutcome(t *testing.T) {
	f := setupReadyFinalVerifier(t,
		ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
		publishEvidenceEntry{
			Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect,
		},
	)
	f.vote.outcome = domain.OperationOutcomeUnknown

	if _, err := f.verifier.StepOnce(f.ctx); err != nil {
		t.Fatal(err)
	}
	ready, err := f.cases.Get(f.ctx, f.caseID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.State != workflowcase.ReadyForVerification {
		t.Fatalf("case state = %q, want READY_FOR_VERIFICATION", ready.State)
	}
	if state := finalTaskState(t, f.driverHarness, f.workID); state != domain.TaskAwaitingVerification {
		t.Fatalf("task state = %q, want AWAITING_VERIFICATION", state)
	}
	if count := finalVerificationCount(t, f.driverHarness, f.caseID); count != 0 {
		t.Fatalf("verification count = %d, want 0", count)
	}
	if count := finalAcceptanceCount(t, f.driverHarness, f.taskID); count != 0 {
		t.Fatalf("acceptance count = %d, want 0", count)
	}
}

func TestFinalVerifierRejectsSlotMismatch(t *testing.T) {
	tests := []struct {
		name       string
		decision   ReviewDecision
		valid      []publishEvidenceEntry
		tampered   []publishEvidenceEntry
		reasonPart string
	}{
		{
			name:     "extra slot",
			decision: ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
			valid: []publishEvidenceEntry{{
				Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect,
			}},
			tampered: []publishEvidenceEntry{
				{Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect},
				{Slot: "unexpected", Operation: "op-2", State: domain.OperationConfirmedEffect},
			},
			reasonPart: "unexpected slot",
		},
		{
			name: "missing slot",
			decision: ReviewDecision{Action: DecisionCommentAction, Comments: []DecisionComment{
				{Path: "a.go", Line: 1, Body: "first"}, {Path: "b.go", Line: 2, Body: "second"},
			}},
			valid: []publishEvidenceEntry{
				{Slot: "ado.pr.comment:proj/shop#1:a:b:0", Operation: "op-1", State: domain.OperationConfirmedEffect},
				{Slot: "ado.pr.comment:proj/shop#1:a:b:1", Operation: "op-2", State: domain.OperationConfirmedEffect},
			},
			tampered: []publishEvidenceEntry{
				{Slot: "ado.pr.comment:proj/shop#1:a:b:0", Operation: "op-1", State: domain.OperationConfirmedEffect},
			},
			reasonPart: "missing slot",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := setupReadyFinalVerifier(t, test.decision, test.valid...)
			replaceFinalVerifierEvidence(t, f.driverHarness, f.completionEvidence, driverPublisherContent(t, test.tampered...))

			if _, err := f.verifier.StepOnce(f.ctx); err != nil {
				t.Fatal(err)
			}
			blocked, err := f.cases.Get(f.ctx, f.caseID)
			if err != nil {
				t.Fatal(err)
			}
			if blocked.State != workflowcase.Blocked {
				t.Fatalf("case state = %q, want BLOCKED", blocked.State)
			}
			record := finalVerificationRecord(t, f.driverHarness, f.caseID)
			if record.VerifierType != "REJECT" || !strings.Contains(record.SnapshotJSON, test.reasonPart) {
				t.Fatalf("rejection record = %+v, want reason containing %q", record, test.reasonPart)
			}
		})
	}
}

func TestFinalVerifierHandlesAcceptedTaskReplay(t *testing.T) {
	f := setupReadyFinalVerifier(t,
		ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
		publishEvidenceEntry{
			Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect,
		},
	)
	if _, err := f.verification.AcceptTask(f.ctx, f.taskID, verification.AcceptanceRequest{
		VerifierID: finalVerifierTestID, VerifierType: finalVerifierTestType, CriteriaMet: true,
		EvidenceIDs: []domain.ID{f.completionEvidence},
	}); err != nil {
		t.Fatal(err)
	}
	f.comment.requests = nil
	f.vote.requests = nil

	if _, err := f.verifier.StepOnce(f.ctx); err != nil {
		t.Fatal(err)
	}
	closed, err := f.cases.Get(f.ctx, f.caseID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.State != workflowcase.Closed {
		t.Fatalf("case state = %q, want CLOSED", closed.State)
	}
	if state := finalTaskState(t, f.driverHarness, f.workID); state != domain.TaskSucceeded {
		t.Fatalf("task state = %q, want SUCCEEDED", state)
	}
	if count := finalAcceptanceCount(t, f.driverHarness, f.taskID); count != 1 {
		t.Fatalf("acceptance count = %d, want 1 without re-acceptance", count)
	}
	if len(f.vote.requests) != 1 {
		t.Fatalf("final lookup count = %d, want 1", len(f.vote.requests))
	}
	if count := finalVerificationCount(t, f.driverHarness, f.caseID); count != 1 {
		t.Fatalf("verification count = %d, want 1", count)
	}
}

func TestFinalVerifierRejectsAssessmentWorkIDMismatch(t *testing.T) {
	f := setupReadyFinalVerifier(t,
		ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
		publishEvidenceEntry{
			Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect,
		},
	)
	updateFinalVerifierAssessmentWorkID(t, f.driverHarness, f.caseID, f.workID)

	result, err := f.verifier.StepOnce(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Blocked) != 1 || result.Blocked[0] != f.caseID {
		t.Fatalf("result = %+v, want blocked case %s", result, f.caseID)
	}
	blocked, err := f.cases.Get(f.ctx, f.caseID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.State != workflowcase.Blocked {
		t.Fatalf("case state = %q, want BLOCKED", blocked.State)
	}
	record := finalVerificationRecord(t, f.driverHarness, f.caseID)
	if record.VerifierType != "REJECT" || !strings.Contains(record.SnapshotJSON, "assessment WorkID") {
		t.Fatalf("rejection record = %+v, want assessment WorkID mismatch", record)
	}
}

func TestFinalVerifierRejectsDecisionIDMismatch(t *testing.T) {
	f := setupReadyFinalVerifier(t,
		ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
		publishEvidenceEntry{
			Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect,
		},
	)
	otherDecision := putDecision(t, f.evidence, f.ctx, ReviewDecision{
		Action: DecisionApproveAction, Vote: "approve", Reason: "other",
	})
	updateFinalVerifierTaskPayload(t, f.driverHarness, f.taskID, func(payload *PublishPayload) {
		payload.Decision = string(otherDecision)
	})

	result, err := f.verifier.StepOnce(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Blocked) != 1 || result.Blocked[0] != f.caseID {
		t.Fatalf("result = %+v, want blocked case %s", result, f.caseID)
	}
	blocked, err := f.cases.Get(f.ctx, f.caseID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.State != workflowcase.Blocked {
		t.Fatalf("case state = %q, want BLOCKED", blocked.State)
	}
	record := finalVerificationRecord(t, f.driverHarness, f.caseID)
	if record.VerifierType != "REJECT" || !strings.Contains(record.SnapshotJSON, "decision identity") {
		t.Fatalf("rejection record = %+v, want decision identity mismatch", record)
	}
}

func TestFinalVerifierUsesIndependentLookupOutcome(t *testing.T) {
	f := setupReadyFinalVerifier(t,
		ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
		publishEvidenceEntry{
			Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect,
		},
	)
	replaceFinalVerifierEvidence(t, f.driverHarness, f.completionEvidence, driverPublisherContent(t, publishEvidenceEntry{
		Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1",
		State: domain.OperationOutcomeUnknown, RecordedOnly: true,
	}))

	result, err := f.verifier.StepOnce(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Closed) != 1 || result.Closed[0] != f.caseID {
		t.Fatalf("result = %+v, want closed case %s", result, f.caseID)
	}
	if state := finalTaskState(t, f.driverHarness, f.workID); state != domain.TaskSucceeded {
		t.Fatalf("task state = %q, want SUCCEEDED", state)
	}
	if len(f.vote.requests) != 1 {
		t.Fatalf("independent lookup count = %d, want 1", len(f.vote.requests))
	}
}

func TestFinalVerifierConcurrentCloseReplay(t *testing.T) {
	f := setupReadyFinalVerifier(t,
		ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"},
		publishEvidenceEntry{
			Slot: "ado.pr.approve:proj/shop#1:a:b", Operation: "op-1", State: domain.OperationConfirmedEffect,
		},
	)
	provider := &gatedLookupProvider{delegate: f.vote, release: make(chan struct{})}
	f.verifier.config.Vote = provider
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan struct {
		result FinalVerifierResult
		err    error
	}, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := f.verifier.StepOnce(ctx)
			results <- struct {
				result FinalVerifierResult
				err    error
			}{result: result, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	closed := 0
	for outcome := range results {
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		closed += len(outcome.result.Closed)
	}
	if closed == 0 {
		t.Fatal("concurrent verification closed no case")
	}
	if count := finalVerificationCount(t, f.driverHarness, f.caseID); count != 1 {
		t.Fatalf("concurrent verification count = %d, want 1", count)
	}
}
