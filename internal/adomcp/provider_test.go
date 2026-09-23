package adomcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeSession struct {
	tools  []string
	called []string
	args   []any
	result *mcp.CallToolResult
}

func (s *fakeSession) ListTools(_ context.Context, _ *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	tools := make([]*mcp.Tool, 0, len(s.tools))
	for _, name := range s.tools {
		tools = append(tools, &mcp.Tool{Name: name})
	}
	return &mcp.ListToolsResult{Tools: tools}, nil
}
func (s *fakeSession) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.called = append(s.called, params.Name)
	s.args = append(s.args, params.Arguments)
	if s.result != nil {
		return s.result, nil
	}
	return &mcp.CallToolResult{}, nil
}
func (s *fakeSession) Close() error { return nil }

func TestProviderAdvertisesOnlyExplicitReadCapabilities(t *testing.T) {
	p, err := New(Config{Command: "/bin/true", Organization: "Contoso"})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := p.Advertise(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 2 {
		t.Fatalf("got %d definitions, want 2", len(definitions))
	}
	if definitions[0].Skill.Name != "ado.projects.list" || definitions[1].Skill.Name != "ado.work_item.read" {
		t.Fatalf("unexpected definitions: %+v", definitions)
	}
	for _, definition := range definitions {
		if definition.Access.Provider != p.Name() || len(definition.Authority.Capabilities) != 1 || definition.Authority.Capabilities[0] != definition.Skill.Name {
			t.Fatalf("unscoped definition: %+v", definition)
		}
	}
}

func TestProviderRejectsUnknownAndMutatingActionsBeforeDial(t *testing.T) {
	p, _ := New(Config{Command: "/bin/true", Organization: "Contoso"})
	dials := 0
	p.dial = func(context.Context) (mcpSession, error) { dials++; return &fakeSession{}, nil }
	for _, tc := range []struct {
		name    string
		request any
	}{
		{"ado.unknown", nil},
		{"ado.work_item.read", map[string]any{"action": "update", "id": 42}},
		{"ado.work_item.read", map[string]any{"action": "get", "tool": "wit_work_item_write"}},
		{"ado.projects.list", map[string]any{"action": "delete"}},
	} {
		if _, err := p.Call(t.Context(), tc.name, tc.request); err == nil {
			t.Fatalf("accepted %s: %#v", tc.name, tc.request)
		}
	}
	if dials != 0 {
		t.Fatalf("dialed %d times for rejected calls", dials)
	}
}

func TestProviderCallsReadToolAndPropagatesToolError(t *testing.T) {
	p, _ := New(Config{Command: "/bin/true", Organization: "Contoso"})
	session := &fakeSession{tools: []string{"core_list_projects", "wit_work_item"}}
	p.dial = func(context.Context) (mcpSession, error) { return session, nil }
	if _, err := p.Call(t.Context(), "ado.work_item.read", map[string]any{"action": "get", "id": 42}); err != nil {
		t.Fatal(err)
	}
	if len(session.called) != 1 || session.called[0] != "wit_work_item" {
		t.Fatalf("called tools: %v", session.called)
	}
	forwarded, ok := session.args[0].(map[string]any)
	if !ok || forwarded["action"] != "get" || forwarded["id"] != 42 {
		t.Fatalf("work item arguments: %#v", session.args[0])
	}
	session.result = &mcp.CallToolResult{IsError: true}
	if _, err := p.Call(t.Context(), "ado.projects.list", nil); err == nil {
		t.Fatal("MCP tool error was ignored")
	}
}

func TestProviderProbeMarksMissingToolUnavailable(t *testing.T) {
	p, _ := New(Config{Command: "/bin/true", Organization: "Contoso"})
	p.dial = func(context.Context) (mcpSession, error) {
		return &fakeSession{tools: []string{"core_list_projects"}}, nil
	}
	definitions, _ := p.Advertise(t.Context())
	projects, err := p.Probe(t.Context(), definitions[0])
	if err != nil || !projects.Available {
		t.Fatalf("projects probe: %+v, %v", projects, err)
	}
	items, err := p.Probe(t.Context(), definitions[1])
	if err != nil || items.Available {
		t.Fatalf("work item probe: %+v, %v", items, err)
	}
	p.dial = func(context.Context) (mcpSession, error) { return nil, errors.New("offline") }
	if _, err := p.Probe(t.Context(), definitions[0]); err == nil {
		t.Fatal("transport failure ignored")
	}
}
