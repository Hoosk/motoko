package tools

import (
	"context"
	"strings"

	"github.com/Hoosk/motoko/internal/mcp"
)

// MCPRemoteTool adapts an MCP server-side tool into Motoko's Tool interface
// so it can live inside the existing tool registry without touching the
// agent loop or the prompt builder.
//
// The underlying *mcp.RemoteTool handles the JSON-RPC plumbing; this wrapper
// is responsible for converting Motoko's (Spec, Result) shapes.
type MCPRemoteTool struct {
	adapter mcp.ToolAdapter
}

// NewMCPRemoteTool wraps a pre-built mcp.ToolAdapter.
func NewMCPRemoteTool(adapter mcp.ToolAdapter) *MCPRemoteTool {
	return &MCPRemoteTool{adapter: adapter}
}

// Spec mirrors the MCP tool's metadata into Motoko's Spec.
func (t *MCPRemoteTool) Spec() Spec {
	s := t.adapter.Spec()
	return Spec{
		Name:    s.Name,
		Summary: firstNonEmpty(s.Summary, s.Description, "MCP tool"),
		Usage:   s.Usage,
	}
}

// Run forwards the call to the MCP server.
func (t *MCPRemoteTool) Run(ctx context.Context, args string) (Result, error) {
	out, err := t.adapter.Run(ctx, args)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Spec:    t.Spec(),
		Summary: out.Summary,
		Output:  out.Output,
	}, nil
}

// DynamicSpec reports a stable Spec; the read-only hint is propagated through
// the Summary line so the agent can see the original name.
func (t *MCPRemoteTool) DynamicSpec(ctx ToolContext) Spec {
	return t.Spec()
}

// IsReadOnly returns whether the MCP server advertised the tool as
// read-only. Used by the runtime to filter plan/search agents.
func (t *MCPRemoteTool) IsReadOnly() bool {
	if t == nil {
		return false
	}
	if rt, ok := t.adapter.(*mcp.RemoteTool); ok {
		return rt.IsReadOnly()
	}
	return false
}

// ServerName returns the MCP server that exposes this tool. Empty if the
// adapter isn't a *mcp.RemoteTool.
func (t *MCPRemoteTool) ServerName() string {
	if rt, ok := t.adapter.(*mcp.RemoteTool); ok {
		return rt.ServerName()
	}
	return ""
}

// OriginalName returns the original tool name (without server prefix).
func (t *MCPRemoteTool) OriginalName() string {
	if rt, ok := t.adapter.(*mcp.RemoteTool); ok {
		return rt.OriginalName()
	}
	return ""
}

// Name returns the registered tool name (the prefixed form).
func (t *MCPRemoteTool) Name() string {
	if t == nil {
		return ""
	}
	return t.Spec().Name
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "MCP tool"
}
