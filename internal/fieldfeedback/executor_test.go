package fieldfeedback

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/approvals"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/executors"
	"github.com/SofiaFlux/meeseek-collective/internal/localconfig"
	"github.com/SofiaFlux/meeseek-collective/internal/operations"
	"github.com/SofiaFlux/meeseek-collective/internal/policy"
	"github.com/SofiaFlux/meeseek-collective/internal/purpose"
	"github.com/SofiaFlux/meeseek-collective/internal/resources"
)

type emitPolicy struct {
	mu sync.Mutex
	outcome domain.PolicyOutcome
	required []domain.ID
}

func (p *emitPolicy) Evaluate(_ context.Context, in policy.PolicyInput) (domain.PolicyDecision, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return domain.PolicyDecision{
		ID: domain.NewID("policy-decision"), Outcome: p.outcome,
		RequiredApprovals: append([]domain.ID(nil), p.required...),
		ReasonCodes: []string{"test"}, PolicySetID: "policy_test",
		PolicySetHash: "policy-v1", PolicyCapabilitiesHash: "caps_test",
		InputDigest: "input", EvaluatedAt: in.Now,
	}, nil
}

type emitHarness struct {
	ctx      context.Context
	feedback *Feedback
	artifact domain.SanitizedFeedback
	exec     *execution.Service
	ops      *operations.Service
	sink     *fakeSink
	emitter  *EmitExecutor
	task     domain.Task
	attempt  domain.Attempt
	approvals *approvals.Service
}

func newEmitHarness(t *testing.T, mode localconfig.FeedbackMode, loseAck bool) *emitHarness {
	t.Helper()
	ctx := context.Background()
	feedback, artifact := emissionArtifact(t)
	execSvc := execution.New(feedback.store, feedback.clock, purpose.New(feedback.store, feedback.clock))
	cfg := localconfig.FieldFeedbackConfig{
		Enabled: true, Mode: mode, Provider: "github", Destination: "owner/repo",
		MaintenanceEnvelopeID: "env_feedback", RequiredEnforcement: domain.EnforcementEnforced,
	}
	if err := feedback.ConfigureEmission(execSvc, cfg, "owner_1"); err != nil {
		t.Fatal(err)
	}
	if err := feedback.OnSanitized(ctx, artifact.ID); err != nil {
		t.Fatal(err)
	}
	task, err := feedback.RequestEmit(ctx, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := execSvc.StartAttempt(ctx, task.ID, "feedback-emitter", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := feedback.clock.Now().UTC().Format(time.RFC3339Nano)
	if _, err := feedback.store.DB().ExecContext(ctx, `
		INSERT INTO policy_sets(
			policy_set_id, version, module_name, module, policy_hash, capabilities_hash, active, created_at
		) VALUES ('policy_test', 1, 'test.rego', 'package test', 'policy-v1', 'caps_test', 1, ?)`, now,
	); err != nil {
		t.Fatal(err)
	}
	sink := newFakeSink()
	sink.loseAck = loseAck
	provider, err := NewProvider("github", "owner/repo", feedback, sink)
	if err != nil {
		t.Fatal(err)
	}
	approvalSvc := approvals.New(feedback.store, feedback.clock)
	pol := &emitPolicy{outcome: domain.PolicyAllow}
	ops := operations.New(
		feedback.store, feedback.clock, execSvc, pol,
		resources.New(feedback.store, feedback.clock), approvalSvc,
		"collective_feedback", provider,
	)
	emitter, err := NewEmitExecutor(feedback, ops, "github")
	if err != nil {
		t.Fatal(err)
	}
	return &emitHarness{
		ctx: ctx, feedback: feedback, artifact: artifact, exec: execSvc, ops: ops,
		sink: sink, emitter: emitter, task: task, attempt: attempt, approvals: approvalSvc,
	}
}

func TestEmitExecutorRefusesTaskWithoutEmissionLinkage(t *testing.T) {
	h := newEmitHarness(t, localconfig.FeedbackModeAutoIfAllowed, false)
	_, err := h.emitter.Start(h.ctx, executors.AttemptEnvelope{
		TaskID: "not-linked", AttemptID: h.attempt.ID,
	})
	if err == nil {
		t.Fatal("executor accepted task without feedback_emissions linkage")
	}
}

func TestEmitExecutorUsesFencedAttemptAndReturnsSanitizedEvidenceOnly(t *testing.T) {
	h := newEmitHarness(t, localconfig.FeedbackModeAutoIfAllowed, false)
	result, err := h.emitter.Start(h.ctx, executors.AttemptEnvelope{
		TaskID: h.task.ID, AttemptID: h.attempt.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || len(result.Evidence) == 0 {
		t.Fatalf("execution result = %+v", result)
	}
	var operationAttempt domain.ID
	if err := h.feedback.store.DB().QueryRowContext(h.ctx,
		"SELECT attempt_id FROM external_operations WHERE task_id = ? ORDER BY created_at DESC LIMIT 1", h.task.ID,
	).Scan(&operationAttempt); err != nil {
		t.Fatal(err)
	}
	if operationAttempt != h.attempt.ID {
		t.Fatalf("operation attempt = %s, want fenced %s", operationAttempt, h.attempt.ID)
	}
	for _, evidence := range result.Evidence {
		if strings.Contains(evidence.Content, "Generic safe field observation") ||
			strings.Contains(evidence.Content, "recover automatically") {
			t.Fatalf("executor evidence leaked candidate text: %q", evidence.Content)
		}
	}
	createCount, _ := h.sink.counts()
	if createCount != 1 {
		t.Fatalf("sink create count = %d, want 1", createCount)
	}
}

func TestEmitExecutorPendingApprovalDoesNotDispatchProvider(t *testing.T) {
	h := newEmitHarness(t, localconfig.FeedbackModeRequireApproval, false)
	result, err := h.emitter.Start(h.ctx, executors.AttemptEnvelope{
		TaskID: h.task.ID, AttemptID: h.attempt.ID,
	})
	if err == nil {
		t.Fatal("pending approval returned success")
	}
	if result.ExitCode == 0 {
		t.Fatalf("pending approval result = %+v", result)
	}
	createCount, _ := h.sink.counts()
	if createCount != 0 {
		t.Fatalf("sink create count = %d, want 0", createCount)
	}
	var state domain.OperationState
	if err := h.feedback.store.DB().QueryRowContext(h.ctx,
		"SELECT state FROM external_operations WHERE task_id = ? ORDER BY created_at DESC LIMIT 1", h.task.ID,
	).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != domain.OperationPrepared {
		t.Fatalf("operation state = %s, want PREPARED", state)
	}
}

func TestUnknownOutcomeReconcilesWithoutDuplicateCreate(t *testing.T) {
	h := newEmitHarness(t, localconfig.FeedbackModeAutoIfAllowed, true)
	_, err := h.emitter.Start(h.ctx, executors.AttemptEnvelope{
		TaskID: h.task.ID, AttemptID: h.attempt.ID,
	})
	if !errors.Is(err, domain.ErrOutcomeUnknown) {
		t.Fatalf("first start error = %v, want ErrOutcomeUnknown", err)
	}
	if err := h.exec.RevokeLease(h.ctx, h.attempt.ID); err != nil {
		t.Fatal(err)
	}
	replacement, err := h.exec.StartAttempt(h.ctx, h.task.ID, "feedback-emitter", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h.sink.loseAck = false
	result, err := h.emitter.Start(h.ctx, executors.AttemptEnvelope{
		TaskID: h.task.ID, AttemptID: replacement.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("replacement result = %+v", result)
	}
	createCount, lookupCount := h.sink.counts()
	if createCount != 1 || lookupCount == 0 {
		t.Fatalf("sink counts create=%d lookup=%d, want create=1 lookup>0", createCount, lookupCount)
	}
	var state domain.OperationState
	if err := h.feedback.store.DB().QueryRowContext(h.ctx,
		"SELECT state FROM external_operations WHERE task_id = ? ORDER BY created_at DESC LIMIT 1", h.task.ID,
	).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != domain.OperationConfirmedEffect {
		t.Fatalf("final operation state = %s, want CONFIRMED_EFFECT", state)
	}
}
