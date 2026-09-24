// Package adomcp exposes a small, explicit read-only subset of Azure DevOps MCP.
package adomcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/SofiaFlux/summa42/internal/capabilities"
	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const providerName = "ado-mcp"

var organizationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,49}$`)

type Config struct {
	Command      string
	Organization string
	Timeout      time.Duration
}

type mcpSession interface {
	ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error)
	Close() error
}

type Provider struct {
	config Config
	dial   func(context.Context) (mcpSession, error)
}

func New(config Config) (*Provider, error) {
	config.Command = strings.TrimSpace(config.Command)
	config.Organization = strings.TrimSpace(config.Organization)
	if config.Command == "" || !organizationPattern.MatchString(config.Organization) {
		return nil, errors.New("ADO MCP requires an executable and a valid organization name")
	}
	if config.Timeout <= 0 {
		config.Timeout = 15 * time.Second
	}
	p := &Provider{config: config}
	p.dial = p.connect
	return p, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Advertise(context.Context) ([]capabilities.Definition, error) {
	return []capabilities.Definition{
		p.definition("ado.projects.list", "core_list_projects"),
		p.definition("ado.work_item.read", "wit_work_item"),
		p.definition("ado.pr.list", "repo_pull_request"),
		p.definition("ado.pr.get", "repo_pull_request"),
		p.definition("ado.pr.org_active", "repo_pull_request_org"),
		p.definition("ado.pr.threads", "repo_pull_request_thread"),
		p.definition("ado.pr.file", "repo_file"),
		p.definition("ado.build.status", "pipelines_build"),
	}, nil
}

func (p *Provider) definition(name, tool string) capabilities.Definition {
	return capabilities.Definition{
		ID:          domain.ID("cap_" + strings.ReplaceAll(name, ".", "_") + "_v1"),
		Skill:       capabilities.Skill{Name: name, Version: "v1"},
		Access:      capabilities.Access{Provider: p.Name(), Context: "ado:" + p.config.Organization + ":" + tool},
		Authority:   capabilities.AuthorityRequirement{Capabilities: []string{name}},
		Environment: capabilities.EnvironmentRequirement{MinimumEnforcement: domain.EnforcementPartial},
	}
}

func (p *Provider) Probe(ctx context.Context, definition capabilities.Definition) (capabilities.ProbeResult, error) {
	tool, err := toolFor(definition.Skill.Name)
	if err != nil {
		return capabilities.ProbeResult{}, err
	}
	available, err := p.hasTool(ctx, tool)
	if err != nil {
		return capabilities.ProbeResult{}, err
	}
	health := capabilities.HealthHealthy
	if !available {
		health = capabilities.HealthUnhealthy
	}
	return capabilities.ProbeResult{
		Enforcement:  domain.EnforcementPartial,
		Evidence:     []string{"ado-mcp:tool-present:" + tool + ":" + fmt.Sprint(available)},
		CostMetadata: map[string]any{"unit": "mcp-call", "class": "external-read"},
		Health:       health,
		Available:    available,
	}, nil
}

func (p *Provider) Call(ctx context.Context, capability string, request any) (any, error) {
	tool, err := toolFor(capability)
	if err != nil {
		return nil, err
	}
	args := map[string]any{}
	switch capability {
	case "ado.projects.list":
		if request != nil {
			m, ok := request.(map[string]any)
			if !ok || len(m) != 0 {
				return nil, errors.New("project listing accepts no parameters")
			}
		}
	case "ado.work_item.read":
		m, ok := request.(map[string]any)
		if !ok {
			return nil, errors.New("work item request must be an object")
		}
		action, ok := m["action"].(string)
		if !ok || !allowedAction(capability, action) {
			return nil, fmt.Errorf("work item action %q is not read-only", action)
		}
		if _, bypass := m["tool"]; bypass {
			return nil, errors.New("MCP tool override is forbidden")
		}
		for key, value := range m {
			args[key] = value
		}
	case "ado.pr.list", "ado.pr.get":
		m, ok := request.(map[string]any)
		if !ok {
			return nil, errors.New("pull request request must be an object")
		}
		action, ok := m["action"].(string)
		if !ok || !allowedAction(capability, action) {
			return nil, fmt.Errorf("pull request action %q is not allowed for %q", action, capability)
		}
		if _, bypass := m["tool"]; bypass {
			return nil, errors.New("MCP tool override is forbidden")
		}
		for key, value := range m {
			args[key] = value
		}
	case "ado.pr.org_active":
		if request != nil {
			m, ok := request.(map[string]any)
			if !ok {
				return nil, errors.New("org PR request must be an object")
			}
			if _, has := m["action"]; has {
				return nil, errors.New("org PR listing accepts no action")
			}
			if _, bypass := m["tool"]; bypass {
				return nil, errors.New("MCP tool override is forbidden")
			}
			for key, value := range m {
				args[key] = value
			}
		}
	case "ado.pr.threads", "ado.pr.file", "ado.build.status":
		m, ok := request.(map[string]any)
		if !ok {
			return nil, errors.New("request must be an object")
		}
		action, ok := m["action"].(string)
		if !ok || !allowedAction(capability, action) {
			return nil, fmt.Errorf("action %q is not allowed for %q", action, capability)
		}
		if _, bypass := m["tool"]; bypass {
			return nil, errors.New("MCP tool override is forbidden")
		}
		for key, value := range m {
			args[key] = value
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()
	session, err := p.dial(callCtx)
	if err != nil {
		return nil, fmt.Errorf("connect ADO MCP: %w", err)
	}
	defer session.Close()
	result, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("ADO MCP %s: %w", tool, err)
	}
	if result == nil || result.IsError {
		return nil, fmt.Errorf("ADO MCP %s returned a tool error", tool)
	}
	return result, nil
}

func toolFor(capability string) (string, error) {
	switch capability {
	case "ado.projects.list":
		return "core_list_projects", nil
	case "ado.work_item.read":
		return "wit_work_item", nil
	case "ado.pr.list", "ado.pr.get":
		return "repo_pull_request", nil
	case "ado.pr.org_active":
		return "repo_pull_request_org", nil
	case "ado.pr.threads":
		return "repo_pull_request_thread", nil
	case "ado.pr.file":
		return "repo_file", nil
	case "ado.build.status":
		return "pipelines_build", nil
	default:
		return "", fmt.Errorf("unsupported ADO capability %q", capability)
	}
}

var allowedActions = map[string]map[string]bool{
	"ado.work_item.read": {
		"get": true, "get_batch": true, "list_comments": true, "my": true,
		"list_revisions": true, "list_for_iteration": true, "get_type": true,
	},
	"ado.pr.list":      {"list": true, "list_by_commits": true},
	"ado.pr.get":       {"get": true},
	"ado.pr.threads":   {"list": true, "list_comments": true},
	"ado.pr.file":      {"get_content": true, "list_directory": true},
	"ado.build.status": {"get_status": true},
}

func allowedAction(capability, action string) bool {
	return allowedActions[capability][action]
}

func (p *Provider) hasTool(ctx context.Context, name string) (bool, error) {
	probeCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()
	session, err := p.dial(probeCtx)
	if err != nil {
		return false, fmt.Errorf("connect ADO MCP: %w", err)
	}
	defer session.Close()
	for cursor := ""; ; {
		page, err := session.ListTools(probeCtx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return false, fmt.Errorf("list ADO MCP tools: %w", err)
		}
		if page == nil {
			return false, errors.New("ADO MCP returned an empty tool list response")
		}
		for _, tool := range page.Tools {
			if tool != nil && tool.Name == name {
				return true, nil
			}
		}
		if page.NextCursor == "" {
			return false, nil
		}
		if page.NextCursor == cursor {
			return false, errors.New("ADO MCP repeated pagination cursor")
		}
		cursor = page.NextCursor
	}
}

func (p *Provider) connect(ctx context.Context) (mcpSession, error) {
	cmd := exec.CommandContext(ctx, p.config.Command, p.config.Organization)
	cmd.Env = adoEnvironment()
	client := mcp.NewClient(&mcp.Implementation{Name: "summa42-ado-readonly", Version: "v1"}, nil)
	return client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
}

func adoEnvironment() []string {
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true,
		"AZURE_CONFIG_DIR": true, "AZURE_DEVOPS_EXT_PAT": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
		"SSL_CERT_FILE": true, "NODE_EXTRA_CA_CERTS": true,
	}
	var result []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if allowed[key] {
			result = append(result, entry)
		}
	}
	return result
}
