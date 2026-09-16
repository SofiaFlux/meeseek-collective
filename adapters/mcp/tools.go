package mcp

import (
	"context"
	"errors"
	"strings"

	"github.com/SofiaFlux/meeseek-collective/internal/capabilities"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool describes a semantic capability exposed through one Attempt-scoped MCP server.
// It is presentation metadata only; authority remains exclusively in capabilities.Session.
type Tool struct {
	Name        string
	Description string
}

func addTool(server *sdkmcp.Server, session *capabilities.Session, tool Tool) error {
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		return errors.New("MCP tool name is required")
	}
	if name != tool.Name {
		return errors.New("MCP tool name must not contain surrounding whitespace")
	}

	sdkmcp.AddTool[any, any](server, &sdkmcp.Tool{
		Name:        name,
		Description: tool.Description,
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input any) (*sdkmcp.CallToolResult, any, error) {
		result, err := session.Call(ctx, name, input)
		return nil, result, err
	})
	return nil
}
