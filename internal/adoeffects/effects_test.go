package adoeffects

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/approvals"
	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/operations"
	"github.com/SofiaFlux/summa42/internal/policy"
	"github.com/SofiaFlux/summa42/internal/purpose"
	"github.com/SofiaFlux/summa42/internal/resources"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

type fakeDial struct {
	calls []mcpCall
	pages []map[string]any
	err   error
}

type mcpCall struct {
	tool string
	args map[string]any
}

func (f *fakeDial) dial(context.Context) (callToolFunc, error) {
	return func(_ context.Context, tool string, args map[string]any) (map[string]any, error) {
		f.calls = append(f.calls, mcpCall{tool, args})
		if f.err != nil {
			return nil, f.err
		}
		if len(f.pages) == 0 {
			return map[string]any{"ok": true}, nil
		}
		page := f.pages[0]
		f.pages = f.pages[1:]
		return page, nil
	}, nil
}

func TestCommentIntentCanonicalStable(t *testing.T) {
	p := &CommentProvider{}
	a, err := p.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical unstable:\n%s\n%s", a, b)
	}
	var decoded map[string]any
	if err := json.Unmarshal(a, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestProvidersRejectForeignIntents(t *testing.T) {
	comment := &CommentProvider{}
	if _, err := comment.CanonicalIntent(VoteIntent{PR: 1, Vote: 10}); err == nil {
		t.Fatal("comment provider accepted vote intent")
	}
	vote := &VoteProvider{}
	if _, err := vote.CanonicalIntent(CommentIntent{Body: "x"}); err == nil {
		t.Fatal("vote provider accepted comment intent")
	}
	if _, err := vote.CanonicalIntent(VoteIntent{PR: 1, Vote: -10}); err == nil {
		t.Fatal("vote provider accepted rejection vote")
	}
	if vote.Capability() != "ado.pr.approve" || comment.Capability() != "ado.pr.comment" {
		t.Fatalf("capabilities = %q %q", vote.Capability(), comment.Capability())
	}
}

func stubRead(context.Context, string, any) (any, error) {
	return nil, errors.New("no reads in dispatch test")
}

func TestDispatchCallsWriteTools(t *testing.T) {
	commentDial := &fakeDial{pages: []map[string]any{{"threadId": 9}}}
	comment := &CommentProvider{
		config: Config{Command: "/bin/true", Organization: "Contoso", Timeout: time.Minute},
		dial:   commentDial.dial,
		read:   stubRead,
	}
	canonical, err := comment.CanonicalIntent(CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: "[summa42:c:w]"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := comment.Dispatch(context.Background(), operations.ProviderDispatchRequest{
		OperationID: "op-1", TaskID: "task-1", EffectSlotID: "slot-1",
		IntentFingerprint: "fp", IntentRevision: 1, CanonicalIntent: canonical,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != domain.OperationConfirmedEffect && outcome.State != domain.OperationOutcomeUnknown {
		t.Fatalf("state = %q", outcome.State)
	}
	if len(commentDial.calls) != 1 {
		t.Fatalf("calls = %v, want exactly 1", commentDial.calls)
	}
	call := commentDial.calls[0]
	if call.tool != "repo_pull_request_thread_write" || call.args["action"] != "create" {
		t.Fatalf("call = %+v", call)
	}
	voteDial := &fakeDial{pages: []map[string]any{{"vote": 10}}}
	vote := &VoteProvider{
		config: Config{Command: "/bin/true", Organization: "Contoso", Timeout: time.Minute},
		dial:   voteDial.dial,
		read:   stubRead,
	}
	voteCanonical, err := vote.CanonicalIntent(VoteIntent{Project: "proj", Repository: "shop", PR: 7, Vote: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vote.Dispatch(context.Background(), operations.ProviderDispatchRequest{
		OperationID: "op-2", TaskID: "task-1", EffectSlotID: "slot-2",
		IntentFingerprint: "fp2", IntentRevision: 1, CanonicalIntent: voteCanonical,
	}); err != nil {
		t.Fatal(err)
	}
	if len(voteDial.calls) != 1 || voteDial.calls[0].tool != "repo_pull_request_write" || voteDial.calls[0].args["action"] != "vote" {
		t.Fatalf("calls = %+v", voteDial.calls)
	}
}

func threadsPayload(contents ...string) map[string]any {
	comments := make([]any, 0, len(contents))
	for _, content := range contents {
		comments = append(comments, map[string]any{"content": content})
	}
	return map[string]any{"threads": []any{map[string]any{"comments": comments}}}
}

func mustCommentProvider(t *testing.T, read ReadFunc) *CommentProvider {
	t.Helper()
	provider, err := NewCommentProvider(Config{Command: "/bin/true", Organization: "Contoso"}, read)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func mustVoteProvider(t *testing.T, read ReadFunc) *VoteProvider {
	t.Helper()
	provider, err := NewVoteProvider(Config{Command: "/bin/true", Organization: "Contoso"}, read)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestCommentLookupFindsMarker(t *testing.T) {
	marker := "[summa42:c:w]"
	intent := CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: marker}

	found := mustCommentProvider(t, func(context.Context, string, any) (any, error) {
		return threadsPayload("nit\n" + marker), nil
	})
	canonical, err := found.CanonicalIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	request := operations.ProviderDispatchRequest{
		OperationID: "op-1", TaskID: "task-1", EffectSlotID: "slot-1",
		IntentFingerprint: "fp", IntentRevision: 1, CanonicalIntent: canonical,
	}
	outcome, err := found.LookupOutcome(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != domain.OperationConfirmedEffect {
		t.Fatalf("state = %q, want CONFIRMED_EFFECT", outcome.State)
	}

	absent := mustCommentProvider(t, func(context.Context, string, any) (any, error) {
		return threadsPayload("unrelated comment"), nil
	})
	outcome, err = absent.LookupOutcome(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != domain.OperationOutcomeUnknown {
		t.Fatalf("state = %q, want OUTCOME_UNKNOWN", outcome.State)
	}

	failing := mustCommentProvider(t, func(context.Context, string, any) (any, error) {
		return nil, errors.New("transport boom")
	})
	outcome, err = failing.LookupOutcome(context.Background(), request)
	if err != nil {
		t.Fatalf("read error must surface as UNKNOWN with nil error, got %v", err)
	}
	if outcome.State != domain.OperationOutcomeUnknown {
		t.Fatalf("state = %q, want OUTCOME_UNKNOWN", outcome.State)
	}
}

func TestVoteLookupConfirmsMatchingVote(t *testing.T) {
	intent := VoteIntent{Project: "proj", Repository: "shop", PR: 7, Vote: 10}
	matching := mustVoteProvider(t, func(context.Context, string, any) (any, error) {
		return map[string]any{"reviewers": []any{map[string]any{"vote": float64(10)}}}, nil
	})
	canonical, err := matching.CanonicalIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	request := operations.ProviderDispatchRequest{
		OperationID: "op-1", TaskID: "task-1", EffectSlotID: "slot-1",
		IntentFingerprint: "fp", IntentRevision: 1, CanonicalIntent: canonical,
	}
	outcome, err := matching.LookupOutcome(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != domain.OperationConfirmedEffect {
		t.Fatalf("state = %q, want CONFIRMED_EFFECT", outcome.State)
	}

	zero := mustVoteProvider(t, func(context.Context, string, any) (any, error) {
		return map[string]any{"reviewers": []any{map[string]any{"vote": float64(0)}}}, nil
	})
	outcome, err = zero.LookupOutcome(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != domain.OperationOutcomeUnknown {
		t.Fatalf("state = %q, want OUTCOME_UNKNOWN", outcome.State)
	}

	failing := mustVoteProvider(t, func(context.Context, string, any) (any, error) {
		return nil, errors.New("transport boom")
	})
	outcome, err = failing.LookupOutcome(context.Background(), request)
	if err != nil {
		t.Fatalf("read error must surface as UNKNOWN with nil error, got %v", err)
	}
	if outcome.State != domain.OperationOutcomeUnknown {
		t.Fatalf("state = %q, want OUTCOME_UNKNOWN", outcome.State)
	}
}

// allowPolicy mirrors the fake-policy allow path in
// internal/operations/service_test.go: a fixed PolicyAllow decision whose
// provenance matches the policy_test profile seeded below.
type allowPolicy struct{}

func (allowPolicy) Evaluate(_ context.Context, in policy.PolicyInput) (domain.PolicyDecision, error) {
	return domain.PolicyDecision{
		ID:                     domain.NewID("decision"),
		Outcome:                domain.PolicyAllow,
		PolicySetID:            domain.ID("policy_test"),
		PolicySetHash:          "policy-v1",
		PolicyCapabilitiesHash: "caps_test",
		InputDigest:            "input_test",
		EvaluatedAt:            in.Now,
	}, nil
}

func TestCommentHarnessRoundTripWithSlotDedup(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 15, 16, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	ledger := resources.New(store, clk)
	approvalSvc := approvals.New(store, clk)

	marker := "[summa42:c:w]"
	commentDial := &fakeDial{pages: []map[string]any{{"threadId": 42}}}
	provider, err := NewCommentProvider(Config{Command: "/bin/true", Organization: "Contoso"}, func(context.Context, string, any) (any, error) {
		return threadsPayload("nit\n" + marker), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.dial = commentDial.dial

	collectiveID := domain.ID("collective_test")
	svc := operations.New(store, clk, execSvc, allowPolicy{}, ledger, approvalSvc, collectiveID, provider)

	now := clk.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO policy_sets(
			policy_set_id, version, module_name, module, policy_hash, capabilities_hash, active, created_at
		) VALUES ('policy_test', 1, 'test.rego', 'package test', 'policy-v1', 'caps_test', 1, ?)`, now,
	); err != nil {
		t.Fatal(err)
	}
	envelopeID := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, 1000, ?)`,
		envelopeID, now,
	); err != nil {
		t.Fatal(err)
	}
	taskID := domain.NewID("task")
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

	intent := CommentIntent{Project: "proj", Repository: "shop", PR: 7, Path: "main.go", Line: 3, Body: "nit", Marker: marker}
	op, err := svc.Prepare(ctx, operations.PrepareRequest{
		AttemptID:      attempt.ID,
		Provider:       provider.Name(),
		TrustedSlotKey: "comment-primary",
		Intent:         intent,
		Risk:           "LOW",
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := svc.Dispatch(ctx, op.ID, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.State != domain.OperationConfirmedEffect {
		t.Fatalf("state = %s, want CONFIRMED_EFFECT", settled.State)
	}

	again, err := svc.Prepare(ctx, operations.PrepareRequest{
		AttemptID:      attempt.ID,
		Provider:       provider.Name(),
		TrustedSlotKey: "comment-primary",
		Intent:         intent,
		Risk:           "LOW",
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != op.ID {
		t.Fatalf("second prepare minted %s, want slot-deduped %s", again.ID, op.ID)
	}
	resettled, err := svc.Dispatch(ctx, again.ID, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resettled.State != domain.OperationConfirmedEffect {
		t.Fatalf("state = %s, want CONFIRMED_EFFECT", resettled.State)
	}
	if len(commentDial.calls) != 1 {
		t.Fatalf("dial calls = %d, want exactly 1 (slot dedup)", len(commentDial.calls))
	}
}
