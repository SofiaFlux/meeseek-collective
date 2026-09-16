package operations_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/operations"
	"github.com/SofiaFlux/meeseek-collective/internal/policy"
	"github.com/SofiaFlux/meeseek-collective/internal/purpose"
	"github.com/SofiaFlux/meeseek-collective/internal/resources"
	state "github.com/SofiaFlux/meeseek-collective/internal/state/sqlite"
	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
)

type mutablePolicy struct {
	mu      sync.Mutex
	outcome domain.PolicyOutcome
	hash    string
}

func (p *mutablePolicy) Evaluate(_ context.Context, in policy.PolicyInput) (domain.PolicyDecision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return domain.PolicyDecision{
		ID:                     domain.NewID("decision"),
		Outcome:                p.outcome,
		ReasonCodes:            []string{"test"},
		PolicySetID:            domain.ID("policy_test"),
		PolicySetHash:          p.hash,
		PolicyCapabilitiesHash: "caps_test",
		InputDigest:            "input_test",
		EvaluatedAt:            in.Now,
	}, nil
}

func (p *mutablePolicy) set(outcome domain.PolicyOutcome, hash string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outcome = outcome
	p.hash = hash
}

type harness struct {
	ctx      context.Context
	store    *state.Store
	exec     *execution.Service
	ledger   *resources.Service
	provider *fakeProvider
	policy   *mutablePolicy
	svc      *operations.Service
	taskID   domain.ID
	attempt  domain.Attempt
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 15, 16, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	ledger := resources.New(store, clk)
	provider := newFakeProvider("fake")
	pol := &mutablePolicy{outcome: domain.PolicyAllow, hash: "policy-v1"}
	collectiveID := domain.ID("collective_test")
	svc := operations.New(store, clk, execSvc, pol, ledger, collectiveID, provider)

	taskID := domain.NewID("task")
	envelopeID := domain.NewID("envelope")
	now := clk.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO policy_sets(
			policy_set_id, version, module_name, module, policy_hash, capabilities_hash, active, created_at
		) VALUES ('policy_test', 1, 'test.rego', 'package test', 'policy-v1', 'caps_test', 1, ?)`, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, 1000, ?)`,
		envelopeID, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO tasks(
			task_id, purpose_kind, purpose_id, state, current_fence,
			acceptance_criteria_json, required_capabilities_json, required_enforcement,
			authority_ceiling_json, resource_envelope_id, priority, created_at, updated_at
		) VALUES (?, 'OWNER_DIRECTIVE', 'owner-test', 'ELIGIBLE', 0, '[]', '[]', 'ENFORCED', ?, ?, 0, ?, ?)`,
		taskID, `["`+provider.Capability()+`"]`, envelopeID, now, now,
	); err != nil {
		t.Fatal(err)
	}
	attempt, err := execSvc.StartAttempt(ctx, taskID, "test-executor", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{ctx: ctx, store: store, exec: execSvc, ledger: ledger, provider: provider, policy: pol, svc: svc, taskID: taskID, attempt: attempt}
}

func setActivePolicyHash(t *testing.T, h *harness, hash string) {
	t.Helper()
	if _, err := h.store.DB().ExecContext(h.ctx,
		`UPDATE policy_sets SET policy_hash = ? WHERE active = 1`, hash,
	); err != nil {
		t.Fatal(err)
	}
}

func preparePurchase(t *testing.T, h *harness, attemptID domain.ID, key string, quantity int) domain.ExternalOperation {
	t.Helper()
	op, err := h.svc.Prepare(h.ctx, operations.PrepareRequest{
		AttemptID:      attemptID,
		Provider:       h.provider.Name(),
		TrustedSlotKey: key,
		Intent:         testutil.PurchaseIntent{SKU: "sku-1", Quantity: quantity},
		Risk:           "LOW",
	})
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func replacementAttempt(t *testing.T, h *harness, previous domain.ID) domain.Attempt {
	t.Helper()
	if err := h.exec.RevokeLease(h.ctx, previous); err != nil {
		t.Fatal(err)
	}
	attempt, err := h.exec.StartAttempt(h.ctx, h.taskID, "replacement", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}

func TestPrepareRejectsDecisionWhosePolicyProfileIsNotActive(t *testing.T) {
	h := newHarness(t)
	setActivePolicyHash(t, h, "policy-v2")

	_, err := h.svc.Prepare(h.ctx, operations.PrepareRequest{
		AttemptID:      h.attempt.ID,
		Provider:       h.provider.Name(),
		TrustedSlotKey: "purchase-primary",
		Intent:         testutil.PurchaseIntent{SKU: "sku-1", Quantity: 1},
		Risk:           "LOW",
	})
	if !errors.Is(err, domain.ErrPolicyDenied) {
		t.Fatalf("expected policy denied for inactive decision profile, got %v", err)
	}
	var slots int
	if err := h.store.DB().QueryRowContext(h.ctx, `SELECT count(*) FROM effect_slots`).Scan(&slots); err != nil {
		t.Fatal(err)
	}
	if slots != 0 {
		t.Fatalf("effect slots = %d, want 0 when policy profile is stale", slots)
	}
}

func TestDispatchRejectsDecisionWhosePolicyProfileIsNotActive(t *testing.T) {
	h := newHarness(t)
	op := preparePurchase(t, h, h.attempt.ID, "purchase-primary", 1)
	setActivePolicyHash(t, h, "policy-v2")

	_, err := h.svc.Dispatch(h.ctx, op.ID, h.attempt.ID)
	if !errors.Is(err, domain.ErrPolicyDenied) {
		t.Fatalf("expected policy denied for inactive dispatch profile, got %v", err)
	}
	if got := h.provider.DispatchCount(); got != 0 {
		t.Fatalf("provider dispatch count = %d, want 0", got)
	}
	var stateValue domain.OperationState
	if err := h.store.DB().QueryRowContext(h.ctx,
		`SELECT state FROM external_operations WHERE operation_id = ?`, op.ID,
	).Scan(&stateValue); err != nil {
		t.Fatal(err)
	}
	if stateValue == domain.OperationDispatched {
		t.Fatal("stale policy profile crossed dispatch commitment boundary")
	}
}

func TestChangedIntentDoesNotMintSecondEffectSlot(t *testing.T) {
	h := newHarness(t)
	first := preparePurchase(t, h, h.attempt.ID, "purchase-primary", 1)
	secondAttempt := replacementAttempt(t, h, h.attempt.ID)

	second, err := h.svc.Prepare(h.ctx, operations.PrepareRequest{
		AttemptID:      secondAttempt.ID,
		Provider:       h.provider.Name(),
		TrustedSlotKey: "purchase-primary",
		Intent:         testutil.PurchaseIntent{SKU: "sku-1", Quantity: 2},
		Risk:           "LOW",
	})
	if !errors.Is(err, domain.ErrIntentConflict) {
		t.Fatalf("expected intent conflict, got %v", err)
	}
	if second.EffectSlotID != "" && second.EffectSlotID != first.EffectSlotID {
		t.Fatalf("minted second slot %s; first was %s", second.EffectSlotID, first.EffectSlotID)
	}
	var slots int
	if err := h.store.DB().QueryRowContext(h.ctx,
		`SELECT count(*) FROM effect_slots WHERE task_id = ? AND trusted_slot_key = 'purchase-primary'`, h.taskID,
	).Scan(&slots); err != nil {
		t.Fatal(err)
	}
	if slots != 1 {
		t.Fatalf("effect slots = %d, want 1", slots)
	}
}

func TestPreparedRevocationPreventsProviderDispatch(t *testing.T) {
	h := newHarness(t)
	op := preparePurchase(t, h, h.attempt.ID, "purchase-primary", 1)
	h.policy.set(domain.PolicyDeny, "policy-v2")
	setActivePolicyHash(t, h, "policy-v2")

	_, err := h.svc.Dispatch(h.ctx, op.ID, h.attempt.ID)
	if !errors.Is(err, domain.ErrPolicyDenied) {
		t.Fatalf("expected policy denied, got %v", err)
	}
	if got := h.provider.DispatchCount(); got != 0 {
		t.Fatalf("provider dispatch count = %d, want 0", got)
	}
	var stateValue domain.OperationState
	if err := h.store.DB().QueryRowContext(h.ctx,
		`SELECT state FROM external_operations WHERE operation_id = ?`, op.ID,
	).Scan(&stateValue); err != nil {
		t.Fatal(err)
	}
	if stateValue == domain.OperationDispatched {
		t.Fatal("revoked PREPARED operation became DISPATCHED")
	}
}

func TestUnknownOutcomeReconcilesBeforeReplacementDispatch(t *testing.T) {
	h := newHarness(t)
	h.provider.SetLoseAcknowledgement(true)
	op := preparePurchase(t, h, h.attempt.ID, "purchase-primary", 2)

	unknown, err := h.svc.Dispatch(h.ctx, op.ID, h.attempt.ID)
	if !errors.Is(err, domain.ErrOutcomeUnknown) {
		t.Fatalf("expected unknown outcome, got %v", err)
	}
	if unknown.State != domain.OperationOutcomeUnknown {
		t.Fatalf("state = %s, want OUTCOME_UNKNOWN", unknown.State)
	}
	var reservationState domain.ReservationState
	if err := h.store.DB().QueryRowContext(h.ctx,
		`SELECT state FROM resource_reservations WHERE reservation_id = ?`, unknown.ReservationID,
	).Scan(&reservationState); err != nil {
		t.Fatal(err)
	}
	if reservationState != domain.ReservationUnresolved {
		t.Fatalf("reservation state = %s, want UNRESOLVED", reservationState)
	}

	secondAttempt := replacementAttempt(t, h, h.attempt.ID)
	resolved, err := h.svc.Dispatch(h.ctx, op.ID, secondAttempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.State != domain.OperationConfirmedEffect {
		t.Fatalf("resolved state = %s, want CONFIRMED_EFFECT", resolved.State)
	}
	if got := h.provider.DispatchCount(); got != 1 {
		t.Fatalf("provider dispatch count = %d, want exactly 1", got)
	}
	if got := h.provider.LookupCount(); got != 1 {
		t.Fatalf("provider lookup count = %d, want 1", got)
	}
}

func TestRepeatedSettlementRejectsConflictingEconomicOutcome(t *testing.T) {
	h := newHarness(t)
	op := preparePurchase(t, h, h.attempt.ID, "purchase-primary", 1)
	settled, err := h.svc.Dispatch(h.ctx, op.ID, h.attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.State != domain.OperationConfirmedEffect {
		t.Fatalf("state = %s, want CONFIRMED_EFFECT", settled.State)
	}

	_, err = h.svc.SettleOutcome(h.ctx, op.ID, operations.ProviderOutcome{
		State:             domain.OperationConfirmedEffect,
		ProviderReference: "conflicting-provider-reference",
		ActualCost:        999,
	})
	if err == nil {
		t.Fatal("conflicting repeated settlement was silently accepted")
	}

	var actualCost int64
	var providerReference string
	if err := h.store.DB().QueryRowContext(h.ctx,
		`SELECT actual_cost, provider_reference FROM external_operations WHERE operation_id = ?`, op.ID,
	).Scan(&actualCost, &providerReference); err != nil {
		t.Fatal(err)
	}
	if actualCost != 10 {
		t.Fatalf("actual cost changed to %d, want original 10", actualCost)
	}
	if providerReference == "conflicting-provider-reference" {
		t.Fatal("provider reference was overwritten by conflicting settlement")
	}
}
