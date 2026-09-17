package audit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
)

func TestDecisionRecordCapturesDecisionTimeContextAndIsImmutable(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 16, 0, 0, 0, time.UTC))
	svc := New(store, clk)

	recorded, err := svc.RecordDecision(ctx, DecisionRecord{
		ID:                 "decision-audit-1",
		Trigger:            "task requested protected repository write",
		Alternatives:       []string{"deny", "request owner approval", "prepare governed operation"},
		BasisClass:         "POLICY_AUTHORITY",
		EvidenceIDs:        []domain.ID{"evidence-policy-1", "evidence-owner-grant-1"},
		PolicyDecisionID:   "policy-decision-1",
		AuthorityIDs:       []domain.ID{"authority-grant-1"},
		ExpectedOutcome:    "governed repository write is prepared but not dispatched",
		Confidence:         0.92,
		ResourceEnvelopeID: "resource-envelope-1",
		ActorID:            "box-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !recorded.CreatedAt.Equal(clk.Now()) {
		t.Fatalf("created_at = %s, want trusted clock %s", recorded.CreatedAt, clk.Now())
	}

	got, err := svc.Decision(ctx, recorded.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Trigger != recorded.Trigger || got.BasisClass != recorded.BasisClass || got.ExpectedOutcome != recorded.ExpectedOutcome || got.ActorID != recorded.ActorID {
		t.Fatalf("loaded decision lost canonical fields: %+v", got)
	}
	if len(got.Alternatives) != 3 || len(got.EvidenceIDs) != 2 || len(got.AuthorityIDs) != 1 {
		t.Fatalf("loaded decision lost list context: %+v", got)
	}

	if _, err := store.DB().ExecContext(ctx, `UPDATE decision_records SET trigger = ? WHERE decision_id = ?`, "tampered", recorded.ID); err == nil || !strings.Contains(strings.ToLower(err.Error()), "immutable") {
		t.Fatalf("decision update err = %v, want immutable rejection", err)
	}
	if _, err := store.DB().ExecContext(ctx, `DELETE FROM decision_records WHERE decision_id = ?`, recorded.ID); err == nil || !strings.Contains(strings.ToLower(err.Error()), "immutable") {
		t.Fatalf("decision delete err = %v, want immutable rejection", err)
	}

	_, err = svc.RecordDecision(ctx, DecisionRecord{
		ID:                 recorded.ID,
		Trigger:            "replacement attempt",
		Alternatives:       []string{"overwrite"},
		BasisClass:         "INVALID",
		EvidenceIDs:        []domain.ID{"other-evidence"},
		PolicyDecisionID:   "other-policy",
		AuthorityIDs:       []domain.ID{"other-authority"},
		ExpectedOutcome:    "overwrite original",
		Confidence:         1,
		ResourceEnvelopeID: "resource-envelope-1",
		ActorID:            "box-2",
	})
	if err == nil {
		t.Fatal("duplicate decision id overwrote immutable record")
	}

	still, err := svc.Decision(ctx, recorded.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.Trigger != recorded.Trigger || !still.CreatedAt.Equal(recorded.CreatedAt) {
		t.Fatalf("original decision changed after duplicate insert: %+v", still)
	}
}

func TestAuditEventsAreAppendOnly(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 16, 30, 0, 0, time.UTC))
	svc := New(store, clk)

	event, err := svc.Append(ctx, Event{
		ID:        "audit-event-1",
		Kind:      "DECISION_RECORDED",
		ActorID:   "box-1",
		SubjectID: "decision-audit-1",
		Payload:   map[string]any{"reason": "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !event.CreatedAt.Equal(clk.Now()) {
		t.Fatalf("event created_at = %s, want %s", event.CreatedAt, clk.Now())
	}
	if _, err := store.DB().ExecContext(ctx, `UPDATE audit_events SET kind = ? WHERE audit_id = ?`, "TAMPERED", event.ID); err == nil {
		t.Fatal("audit event update succeeded")
	}
}
