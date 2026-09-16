package capabilities_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/capabilities"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/purpose"
	state "github.com/SofiaFlux/meeseek-collective/internal/state/sqlite"
	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
)

type countingProvider struct {
	mu       sync.Mutex
	calls    int
	probe    capabilities.ProbeResult
	probeSet bool
}

func (p *countingProvider) Name() string { return "echo-provider" }

func (p *countingProvider) Call(_ context.Context, capability string, request any) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return map[string]any{"capability": capability, "request": request}, nil
}

func (p *countingProvider) Advertise(_ context.Context) ([]capabilities.Definition, error) {
	return []capabilities.Definition{echoDefinition()}, nil
}

func (p *countingProvider) Probe(_ context.Context, _ capabilities.Definition) (capabilities.ProbeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.probeSet {
		return p.probe, nil
	}
	return healthyProbeResult(), nil
}

func (p *countingProvider) SetProbeResult(result capabilities.ProbeResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probe = result
	p.probeSet = true
}

func (p *countingProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func healthyProbeResult() capabilities.ProbeResult {
	return capabilities.ProbeResult{
		Enforcement: domain.EnforcementEnforced,
		Evidence:    []string{"safe-probe:echo:ok"},
		CostMetadata: map[string]any{
			"unit":  "call",
			"class": "free-test",
		},
		Health:    capabilities.HealthHealthy,
		Available: true,
	}
}

func echoDefinition() capabilities.Definition {
	return capabilities.Definition{
		ID: domain.ID("cap_echo"),
		Skill: capabilities.Skill{
			Name:    "echo",
			Version: "v1",
		},
		Access: capabilities.Access{
			Provider: "echo-provider",
			Context:  "local-test",
		},
		Authority: capabilities.AuthorityRequirement{
			Capabilities: []string{"echo"},
		},
		Environment: capabilities.EnvironmentRequirement{
			MinimumEnforcement: domain.EnforcementEnforced,
		},
	}
}

type sessionHarness struct {
	ctx      context.Context
	store    *state.Store
	clock    *testutil.Clock
	exec     *execution.Service
	provider *countingProvider
	registry *capabilities.Registry
	attempt  domain.Attempt
}

func newSessionHarness(t *testing.T) *sessionHarness {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 16, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)

	envelopeID := domain.NewID("envelope")
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, 100, ?)`,
		envelopeID, clk.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	task, err := execSvc.CreateTask(ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: "owner-capability-session"},
		AcceptanceCriteria:   []string{"capability call completes"},
		RequiredCapabilities: []string{"echo"},
		RequiredEnforcement:  domain.EnforcementEnforced,
		AuthorityCeiling:     []string{"echo"},
		ResourceEnvelopeID:   envelopeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := execSvc.StartAttempt(ctx, task.ID, "test-executor", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	provider := &countingProvider{}
	registry := capabilities.NewRegistry(store, clk, execSvc, provider)
	return &sessionHarness{ctx: ctx, store: store, clock: clk, exec: execSvc, provider: provider, registry: registry, attempt: attempt}
}

func TestOpenSessionRejectsCapabilityWithoutObservedAssessment(t *testing.T) {
	h := newSessionHarness(t)
	if err := h.registry.Register(h.ctx, echoDefinition()); err != nil {
		t.Fatal(err)
	}

	_, err := h.registry.OpenSession(h.ctx, "collective-test", h.attempt.ID, []string{"echo"})
	if !errors.Is(err, domain.ErrPolicyDenied) {
		t.Fatalf("open session for unassessed capability = %v, want ErrPolicyDenied", err)
	}
	if got := h.provider.Calls(); got != 0 {
		t.Fatalf("provider call count = %d, want 0", got)
	}
}

func TestSessionCallRejectsRevokedLeaseBeforeProviderIO(t *testing.T) {
	h := newSessionHarness(t)
	if _, err := h.registry.AssessProvider(h.ctx, h.provider.Name()); err != nil {
		t.Fatal(err)
	}
	session, err := h.registry.OpenSession(h.ctx, "collective-test", h.attempt.ID, []string{"echo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.exec.RevokeLease(h.ctx, h.attempt.ID); err != nil {
		t.Fatal(err)
	}

	_, err = session.Call(h.ctx, "echo", map[string]any{"value": "hello"})
	if !errors.Is(err, domain.ErrLeaseInactive) {
		t.Fatalf("session call after lease revocation = %v, want ErrLeaseInactive", err)
	}
	if got := h.provider.Calls(); got != 0 {
		t.Fatalf("provider call count = %d, want 0 after lease revocation", got)
	}
}

func TestSessionRevokeRejectsFurtherCallsBeforeProviderIO(t *testing.T) {
	h := newSessionHarness(t)
	if _, err := h.registry.AssessProvider(h.ctx, h.provider.Name()); err != nil {
		t.Fatal(err)
	}
	session, err := h.registry.OpenSession(h.ctx, "collective-test", h.attempt.ID, []string{"echo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Revoke(h.ctx); err != nil {
		t.Fatal(err)
	}

	_, err = session.Call(h.ctx, "echo", map[string]any{"value": "hello"})
	if !errors.Is(err, domain.ErrLeaseInactive) {
		t.Fatalf("session call after session revocation = %v, want ErrLeaseInactive", err)
	}
	if got := h.provider.Calls(); got != 0 {
		t.Fatalf("provider call count = %d, want 0 after session revocation", got)
	}
}

func TestSessionCallRejectsCapabilityAfterAssessmentBecomesUnavailable(t *testing.T) {
	h := newSessionHarness(t)
	if _, err := h.registry.AssessProvider(h.ctx, h.provider.Name()); err != nil {
		t.Fatal(err)
	}
	session, err := h.registry.OpenSession(h.ctx, "collective-test", h.attempt.ID, []string{"echo"})
	if err != nil {
		t.Fatal(err)
	}

	h.clock.Advance(time.Second)
	h.provider.SetProbeResult(capabilities.ProbeResult{
		Enforcement: domain.EnforcementEnforced,
		Evidence:    []string{"safe-probe:echo:unavailable"},
		CostMetadata: map[string]any{
			"unit":  "call",
			"class": "free-test",
		},
		Health:    capabilities.HealthUnhealthy,
		Available: false,
	})
	if _, err := h.registry.AssessProvider(h.ctx, h.provider.Name()); err != nil {
		t.Fatal(err)
	}

	_, err = session.Call(h.ctx, "echo", map[string]any{"value": "hello"})
	if !errors.Is(err, domain.ErrPolicyDenied) {
		t.Fatalf("session call after capability assessment became unavailable = %v, want ErrPolicyDenied", err)
	}
	if got := h.provider.Calls(); got != 0 {
		t.Fatalf("provider call count = %d, want 0 after capability became unavailable", got)
	}
}
