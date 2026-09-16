package capabilities_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/capabilities"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/purpose"
	"github.com/SofiaFlux/meeseek-collective/internal/testutil"
)

type assessingProvider struct {
	mu         sync.Mutex
	probeCalls int
}

func (p *assessingProvider) Name() string { return "probe-provider" }

func (p *assessingProvider) Call(_ context.Context, capability string, request any) (any, error) {
	return map[string]any{"capability": capability, "request": request}, nil
}

func (p *assessingProvider) Advertise(_ context.Context) ([]capabilities.Definition, error) {
	return []capabilities.Definition{{
		ID: domain.ID("cap_probe_echo"),
		Skill: capabilities.Skill{
			Name:    "probe-echo",
			Version: "v1",
		},
		Access: capabilities.Access{
			Provider: p.Name(),
			Context:  "local-probe",
		},
		Authority: capabilities.AuthorityRequirement{
			Capabilities: []string{"probe-echo"},
		},
		Environment: capabilities.EnvironmentRequirement{
			MinimumEnforcement: domain.EnforcementPartial,
		},
	}}, nil
}

func (p *assessingProvider) Probe(_ context.Context, definition capabilities.Definition) (capabilities.ProbeResult, error) {
	p.mu.Lock()
	p.probeCalls++
	p.mu.Unlock()
	return capabilities.ProbeResult{
		Enforcement: domain.EnforcementEnforced,
		Evidence:    []string{"safe-probe:probe-echo:ok"},
		CostMetadata: map[string]any{
			"unit":  "call",
			"class": "free-test",
		},
		Health:    capabilities.HealthHealthy,
		Available: true,
	}, nil
}

func (p *assessingProvider) ProbeCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.probeCalls
}

func TestAssessmentPersistsObservedProbeState(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 17, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	provider := &assessingProvider{}
	registry := capabilities.NewRegistry(store, clk, execSvc, provider)

	assessments, err := registry.AssessProvider(ctx, provider.Name())
	if err != nil {
		t.Fatal(err)
	}
	if provider.ProbeCalls() != 1 {
		t.Fatalf("probe calls = %d, want 1", provider.ProbeCalls())
	}
	if len(assessments) != 1 {
		t.Fatalf("assessments = %d, want 1", len(assessments))
	}
	got := assessments[0]
	if got.CapabilityID != domain.ID("cap_probe_echo") || got.Enforcement != domain.EnforcementEnforced || !got.Available || got.Health != capabilities.HealthHealthy {
		t.Fatalf("unexpected assessment: %#v", got)
	}
	if len(got.Evidence) != 1 || got.Evidence[0] != "safe-probe:probe-echo:ok" {
		t.Fatalf("assessment evidence = %#v", got.Evidence)
	}

	// Recreate the Registry to prove the assessment is durable state, not an
	// in-memory interpretation of provider descriptions.
	restarted := capabilities.NewRegistry(store, clk, execSvc, provider)
	persisted, err := restarted.LatestAssessment(ctx, "probe-echo")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.CapabilityID != got.CapabilityID || persisted.Enforcement != got.Enforcement || persisted.Health != got.Health || persisted.Available != got.Available {
		t.Fatalf("persisted assessment = %#v, want %#v", persisted, got)
	}
	if persisted.CostMetadata["unit"] != "call" || persisted.CostMetadata["class"] != "free-test" {
		t.Fatalf("persisted cost metadata = %#v", persisted.CostMetadata)
	}
}
