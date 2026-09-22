package mcp_test

import (
	"context"
	"sync"
	"testing"
	"time"

	mcpa "github.com/SofiaFlux/summa42/adapters/mcp"
	"github.com/SofiaFlux/summa42/internal/capabilities"
	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/execution"
	"github.com/SofiaFlux/summa42/internal/purpose"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/testutil"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type provider struct {
	mu    sync.Mutex
	calls map[string]int
}

func (p *provider) Name() string { return "mcp-test-provider" }

func (p *provider) Call(_ context.Context, capability string, request any) (any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[capability]++
	return map[string]any{"capability": capability, "request": request}, nil
}

func (p *provider) Advertise(context.Context) ([]capabilities.Definition, error) {
	return []capabilities.Definition{definition("alpha"), definition("beta")}, nil
}

func (p *provider) Probe(context.Context, capabilities.Definition) (capabilities.ProbeResult, error) {
	return capabilities.ProbeResult{
		Enforcement:  domain.EnforcementEnforced,
		Evidence:     []string{"probe:mcp-test:ok"},
		CostMetadata: map[string]any{"class": "free-test"},
		Health:       capabilities.HealthHealthy,
		Available:    true,
	}, nil
}

func (p *provider) callCount(name string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[name]
}

func definition(name string) capabilities.Definition {
	return capabilities.Definition{
		ID:    domain.ID("cap_" + name),
		Skill: capabilities.Skill{Name: name, Version: "v1"},
		Access: capabilities.Access{
			Provider: "mcp-test-provider",
			Context:  "local-test",
		},
		Authority: capabilities.AuthorityRequirement{Capabilities: []string{name}},
		Environment: capabilities.EnvironmentRequirement{
			MinimumEnforcement: domain.EnforcementEnforced,
		},
	}
}

type harness struct {
	ctx      context.Context
	store    *state.Store
	clock    *testutil.Clock
	exec     *execution.Service
	provider *provider
	registry *capabilities.Registry
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 18, 0, 0, 0, time.UTC))
	purposes := purpose.New(store, clk)
	execSvc := execution.New(store, clk, purposes)
	p := &provider{calls: map[string]int{}}
	registry := capabilities.NewRegistry(store, clk, execSvc, p)
	if _, err := registry.AssessProvider(ctx, p.Name()); err != nil {
		t.Fatal(err)
	}
	return &harness{ctx: ctx, store: store, clock: clk, exec: execSvc, provider: p, registry: registry}
}

func (h *harness) openSession(t *testing.T, name string, lease time.Duration) *capabilities.Session {
	t.Helper()
	envelopeID := domain.NewID("mcp-envelope")
	if _, err := h.store.DB().ExecContext(h.ctx,
		`INSERT INTO resource_envelopes(envelope_id, hard_limit, created_at) VALUES (?, 100, ?)`,
		envelopeID, h.clock.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	task, err := h.exec.CreateTask(h.ctx, execution.TaskRequest{
		Purpose:              domain.PurposeRef{Kind: domain.PurposeOwnerDirective, ID: domain.ID("owner-mcp-" + name)},
		AcceptanceCriteria:   []string{"MCP capability call completes"},
		RequiredCapabilities: []string{name},
		RequiredEnforcement:  domain.EnforcementEnforced,
		AuthorityCeiling:     []string{name},
		ResourceEnvelopeID:   envelopeID,
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := h.exec.StartAttempt(h.ctx, task.ID, "mcp-test-executor", lease)
	if err != nil {
		t.Fatal(err)
	}
	session, err := h.registry.OpenSession(h.ctx, "collective-mcp-test", attempt.ID, []string{name})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func connectClient(t *testing.T, server *mcpa.Server) *sdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "mcp-test-client", Version: "v1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	})
	return clientSession
}

func toolNames(t *testing.T, client *sdkmcp.ClientSession) []string {
	t.Helper()
	result, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestAttemptScopedServersExposeOnlyTheirVisibleTools(t *testing.T) {
	h := newHarness(t)
	alphaSession := h.openSession(t, "alpha", time.Hour)
	betaSession := h.openSession(t, "beta", time.Hour)

	alphaServer, err := mcpa.NewServer(alphaSession, []mcpa.Tool{{Name: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	betaServer, err := mcpa.NewServer(betaSession, []mcpa.Tool{{Name: "beta"}})
	if err != nil {
		t.Fatal(err)
	}
	alphaClient := connectClient(t, alphaServer)
	betaClient := connectClient(t, betaServer)

	if got := toolNames(t, alphaClient); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("Attempt A tools = %v, want [alpha]", got)
	}
	if got := toolNames(t, betaClient); len(got) != 1 || got[0] != "beta" {
		t.Fatalf("Attempt B tools = %v, want [beta]", got)
	}
}

func TestMCPToolCannotGrantAuthorityOutsideBoundCapabilitySession(t *testing.T) {
	h := newHarness(t)
	betaSession := h.openSession(t, "beta", time.Hour)
	server, err := mcpa.NewServer(betaSession, []mcpa.Tool{{Name: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	client := connectClient(t, server)

	result, err := client.CallTool(h.ctx, &sdkmcp.CallToolParams{Name: "alpha", Arguments: map[string]any{"value": "nope"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("MCP call outside the bound capability session succeeded")
	}
	if got := h.provider.callCount("alpha"); got != 0 {
		t.Fatalf("provider alpha call count = %d, want 0", got)
	}
}

func TestRevokedOrExpiredLeaseFailsWhileMCPTransportRemainsOpen(t *testing.T) {
	t.Run("revoked session", func(t *testing.T) {
		h := newHarness(t)
		session := h.openSession(t, "alpha", time.Hour)
		server, err := mcpa.NewServer(session, []mcpa.Tool{{Name: "alpha"}})
		if err != nil {
			t.Fatal(err)
		}
		client := connectClient(t, server)
		if err := session.Revoke(h.ctx); err != nil {
			t.Fatal(err)
		}

		result, err := client.CallTool(h.ctx, &sdkmcp.CallToolParams{Name: "alpha", Arguments: map[string]any{"value": "revoked"}})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatal("revoked capability session MCP call succeeded")
		}
		if got := h.provider.callCount("alpha"); got != 0 {
			t.Fatalf("provider calls after revoke = %d, want 0", got)
		}
		if got := toolNames(t, client); len(got) != 1 || got[0] != "alpha" {
			t.Fatalf("transport did not remain open after revocation; tools = %v", got)
		}
	})

	t.Run("expired lease", func(t *testing.T) {
		h := newHarness(t)
		session := h.openSession(t, "alpha", time.Minute)
		server, err := mcpa.NewServer(session, []mcpa.Tool{{Name: "alpha"}})
		if err != nil {
			t.Fatal(err)
		}
		client := connectClient(t, server)
		h.clock.Advance(2 * time.Minute)

		result, err := client.CallTool(h.ctx, &sdkmcp.CallToolParams{Name: "alpha", Arguments: map[string]any{"value": "expired"}})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatal("expired lease MCP call succeeded")
		}
		if got := h.provider.callCount("alpha"); got != 0 {
			t.Fatalf("provider calls after expiry = %d, want 0", got)
		}
		if got := toolNames(t, client); len(got) != 1 || got[0] != "alpha" {
			t.Fatalf("transport did not remain open after expiry; tools = %v", got)
		}
	})
}
