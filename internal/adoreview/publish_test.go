package adoreview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/evidence"
	"github.com/SofiaFlux/summa42/internal/executors"
	"github.com/SofiaFlux/summa42/internal/operations"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

type fakePublishOps struct {
	prepared      []operations.PrepareRequest
	dispatched    []domain.ID
	ops           map[domain.ID]domain.ExternalOperation
	nextID        int
	dispatchState domain.OperationState
}

func (f *fakePublishOps) Prepare(_ context.Context, request operations.PrepareRequest) (domain.ExternalOperation, error) {
	f.prepared = append(f.prepared, request)
	f.nextID++
	op := domain.ExternalOperation{ID: domain.ID(fmt.Sprintf("op-%d", f.nextID))}
	f.ops[op.ID] = op
	return op, nil
}

func (f *fakePublishOps) Dispatch(_ context.Context, operationID, _ domain.ID) (domain.ExternalOperation, error) {
	f.dispatched = append(f.dispatched, operationID)
	op := f.ops[operationID]
	if f.dispatchState != "" {
		op.State = f.dispatchState
	} else {
		op.State = domain.OperationConfirmedEffect
	}
	return op, nil
}

func newPublishTestEvidenceStore(t *testing.T) *evidence.Store {
	t.Helper()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	return evidenceStore
}

func TestPublishConfigDoesNotHoldProviderObjects(t *testing.T) {
	configType := reflect.TypeOf(PublishConfig{})
	for _, name := range []string{"Comment", "Vote"} {
		if _, exists := configType.FieldByName(name); exists {
			t.Errorf("PublishConfig.%s must not be defined", name)
		}
	}
}

func TestNewPublisherRequiresOperationsAndEvidence(t *testing.T) {
	evidenceStore := newPublishTestEvidenceStore(t)
	tests := []struct {
		name   string
		config PublishConfig
	}{
		{name: "operations", config: PublishConfig{Mode: PublishNone, Evidence: evidenceStore}},
		{name: "evidence", config: PublishConfig{Mode: PublishNone, Operations: &fakePublishOps{}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewPublisher(test.config); err == nil {
				t.Fatal("NewPublisher succeeded without required service")
			}
		})
	}
}

func TestNewPublisherAllRequiresApproveRisk(t *testing.T) {
	for _, risk := range []string{"", " \t"} {
		_, err := NewPublisher(PublishConfig{
			Mode:        PublishAll,
			Operations:  &fakePublishOps{},
			Evidence:    newPublishTestEvidenceStore(t),
			RiskApprove: risk,
		})
		if err == nil {
			t.Fatalf("NewPublisher accepted blank approve risk %q", risk)
		}
	}
}

func putDecision(t *testing.T, store *evidence.Store, ctx context.Context, decision ReviewDecision) domain.ID {
	t.Helper()
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	object, err := store.Put(ctx, bytes.NewReader(raw), evidence.Metadata{MediaType: "application/json", Kind: "ado.review.decision"})
	if err != nil {
		t.Fatal(err)
	}
	return object.ID
}

func publishEnvelope(task, attempt string, workspace string) executors.AttemptEnvelope {
	return executors.AttemptEnvelope{TaskID: domain.ID(task), AttemptID: domain.ID(attempt), Workspace: workspace, Objective: "publish"}
}

func publishPayload(t *testing.T, decision domain.ID) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(PublishPayload{Decision: string(decision), CaseID: "case-1", WorkID: "work-1", Project: "proj", Repo: "shop", PR: 1, Revision: "a:b"})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPublishNoneRecordsWithoutPrepare(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	id := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"})
	publisher, err := NewPublisher(PublishConfig{Mode: PublishNone, Operations: &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}}, Evidence: evidenceStore, OwnerApprovals: []domain.ID{"owner-1"}, RiskComment: "LOW"})
	if err != nil {
		t.Fatal(err)
	}
	envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
	envelope.PayloadJSON = publishPayload(t, id)
	result, err := publisher.Start(ctx, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Evidence) == 0 {
		t.Fatal("no evidence recorded")
	}
}

func TestPublishCommentsDispatchesAndSkipsApprove(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	id := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionCommentAction, Vote: "approve", Reason: "mixed",
		Comments: []DecisionComment{{Path: "a.go", Line: 1, Body: "a.go:1: nit"}, {Path: "b.go", Line: 0, Body: "b.go: nit"}}})
	ops := &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}}
	publisher, err := NewPublisher(PublishConfig{Mode: PublishComments, Operations: ops, Evidence: evidenceStore, OwnerApprovals: []domain.ID{"owner-1"}, RiskComment: "LOW"})
	if err != nil {
		t.Fatal(err)
	}
	envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
	envelope.PayloadJSON = publishPayload(t, id)
	if _, err := publisher.Start(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if len(ops.prepared) != 2 {
		t.Fatalf("prepared = %d, want 2", len(ops.prepared))
	}
	for i, want := range []string{"ado.pr.comment:proj/shop#1:a:b:0", "ado.pr.comment:proj/shop#1:a:b:1"} {
		if ops.prepared[i].TrustedSlotKey != want {
			t.Fatalf("slot[%d] = %q, want %q", i, ops.prepared[i].TrustedSlotKey, want)
		}
		if ops.prepared[i].Provider != "ado-pr-comment" || ops.prepared[i].Risk != "LOW" {
			t.Fatalf("prepared[%d] = %+v", i, ops.prepared[i])
		}
		if len(ops.prepared[i].RequiredApprovals) != 1 || ops.prepared[i].RequiredApprovals[0] != "owner-1" {
			t.Fatalf("approvals = %v", ops.prepared[i].RequiredApprovals)
		}
	}
}

func TestPublishAllDispatchesVote(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	id := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionApproveAction, Vote: "approve", Reason: "clean"})
	ops := &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}}
	publisher, err := NewPublisher(PublishConfig{Mode: PublishAll, Operations: ops, Evidence: evidenceStore, RiskComment: "LOW", RiskApprove: "OWNER"})
	if err != nil {
		t.Fatal(err)
	}
	envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
	envelope.PayloadJSON = publishPayload(t, id)
	if _, err := publisher.Start(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if len(ops.prepared) != 1 {
		t.Fatalf("prepared = %d, want 1", len(ops.prepared))
	}
	got := ops.prepared[0]
	if got.Provider != "ado-pr-vote" || got.TrustedSlotKey != "ado.pr.approve:proj/shop#1:a:b" || got.Risk != "OWNER" {
		t.Fatalf("prepared = %+v", got)
	}
}

func TestPublishUnknownRecordedAndStops(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	id := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionCommentAction, Reason: "mixed",
		Comments: []DecisionComment{{Path: "a.go", Line: 1, Body: "a.go:1: nit"}, {Path: "b.go", Body: "b.go: nit"}}})
	ops := &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}, dispatchState: domain.OperationOutcomeUnknown}
	publisher, err := NewPublisher(PublishConfig{Mode: PublishComments, Operations: ops, Evidence: evidenceStore, RiskComment: "LOW"})
	if err != nil {
		t.Fatal(err)
	}
	envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
	envelope.PayloadJSON = publishPayload(t, id)
	result, err := publisher.Start(ctx, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops.prepared) != 1 {
		t.Fatalf("prepared = %d, want 1 (stop after UNKNOWN)", len(ops.prepared))
	}
	if len(result.Evidence) == 0 {
		t.Fatal("no evidence recorded")
	}
}

func TestPublishRejectsBadPayload(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC))
	evidenceStore, err := evidence.New(store, t.TempDir(), clk)
	if err != nil {
		t.Fatal(err)
	}
	ops := &fakePublishOps{ops: map[domain.ID]domain.ExternalOperation{}}
	publisher, err := NewPublisher(PublishConfig{Mode: PublishAll, Operations: ops, Evidence: evidenceStore, RiskComment: "LOW", RiskApprove: "OWNER"})
	if err != nil {
		t.Fatal(err)
	}
	envelope := publishEnvelope("task-1", "attempt-1", t.TempDir())
	raw, _ := json.Marshal(PublishPayload{CaseID: "case-1", WorkID: "work-1", Project: "proj", Repo: "shop", PR: 1, Revision: "a:b"})
	envelope.PayloadJSON = raw
	if _, err := publisher.Start(ctx, envelope); err == nil {
		t.Fatal("accepted blank decision")
	}
	holdID := putDecision(t, evidenceStore, ctx, ReviewDecision{Action: DecisionHoldAction, Reason: "uncertain"})
	envelope.PayloadJSON = publishPayload(t, holdID)
	result, err := publisher.Start(ctx, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops.prepared) != 0 || len(result.Evidence) == 0 {
		t.Fatalf("prepared = %d evidence = %d, want 0 and >0", len(ops.prepared), len(result.Evidence))
	}
}
