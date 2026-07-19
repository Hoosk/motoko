package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RemoteTool is the ToolAdapter implementation used by the manager. It
// forwards Run invocations to the server via tools/call and serialises the
// result into a ToolResult.
//
// A RemoteTool is constructed per (server, tool) pair; the manager caches one
// instance per registered name and re-registers it on notifications/tools/
// list_changed.
type RemoteTool struct {
	manager    *Manager
	serverName string
	registered string
	tool       Tool
}

// NewRemoteToolAdapter builds a RemoteTool for the given server-side tool
// definition. The caller is responsible for assigning a unique registered
// name (use ToolPrefix).
func NewRemoteToolAdapter(serverName, registered string, t Tool, mgr *Manager) ToolAdapter {
	return &RemoteTool{
		serverName: serverName,
		registered: registered,
		tool:       t,
		manager:    mgr,
	}
}

// Spec returns the tool metadata surfaced to the host.
func (r *RemoteTool) Spec() ToolSpec {
	summary := r.tool.Description
	if summary == "" {
		summary = "Tool exposed by MCP server " + r.serverName
	}
	title := r.tool.Title
	if title == "" {
		title = r.tool.Name
	}
	usage := "mcp tool (see server description)"
	if len(r.tool.InputSchema) > 0 {
		// Provide a compact, human-readable usage hint that includes the
		// original tool name so users can invoke it consistently.
		usage = fmt.Sprintf("JSON args matching the tool's inputSchema; original tool name '%s' on server '%s'",
			r.tool.Name, r.serverName)
	}
	return ToolSpec{
		Name:        r.registered,
		Title:       title,
		Summary:     summary,
		Description: r.tool.Description,
		Usage:       usage,
		ReadOnly:    isReadOnly(r.tool.Annotations),
	}
}

// Run dispatches a call to the server. The raw `args` string is interpreted
// as JSON when it looks like a JSON object; otherwise it is forwarded as a
// string under the conventional `input` key.
func (r *RemoteTool) Run(ctx context.Context, args string) (ToolResult, error) {
	if r.manager == nil {
		return ToolResult{}, fmt.Errorf("mcp: remote tool not bound to a manager")
	}
	srv, err := r.manager.lookupServer(r.serverName)
	if err != nil {
		return ToolResult{}, err
	}
	if srv.client == nil {
		return ToolResult{}, fmt.Errorf("mcp: server %q not connected", r.serverName)
	}

	payload, err := buildCallArguments(args)
	if err != nil {
		return ToolResult{}, err
	}

	result, err := srv.client.CallTool(ctx, r.tool.Name, payload)
	if err != nil {
		return ToolResult{}, err
	}
	if result == nil {
		return ToolResult{Spec: r.Spec(), Summary: "no result", Output: ""}, nil
	}

	output := renderCallResult(result)
	summary := fmt.Sprintf("mcp:%s/%s (%d blocks)", r.serverName, r.tool.Name, len(result.Content))
	if result.IsError {
		summary = "mcp tool returned an error: " + trimForSummary(output, 120)
	}
	return ToolResult{
		Spec:    r.Spec(),
		Summary: summary,
		Output:  output,
	}, nil
}

// ServerName returns the MCP server that owns this tool.
func (r *RemoteTool) ServerName() string { return r.serverName }

// RegisteredName returns the name under which the tool is published in the
// host registry.
func (r *RemoteTool) RegisteredName() string { return r.registered }

// OriginalName returns the original tool name as advertised by the server.
func (r *RemoteTool) OriginalName() string { return r.tool.Name }

// IsReadOnly reports whether the tool is annotated as read-only.
func (r *RemoteTool) IsReadOnly() bool { return isReadOnly(r.tool.Annotations) }

func isReadOnly(a *ToolAnnotations) bool {
	if a == nil {
		return false
	}
	return a.ReadOnlyHint
}

func buildCallArguments(args string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return json.RawMessage(`{}`), nil
	}
	if strings.HasPrefix(trimmed, "{") {
		if !json.Valid([]byte(trimmed)) {
			return nil, fmt.Errorf("mcp: arguments are not valid JSON: %s", trimmed)
		}
		return json.RawMessage(trimmed), nil
	}
	// Treat bare strings as a single `input` field so the host's existing
	// text-style tool invocations still work for tools that take one
	// argument.
	wrapped, err := json.Marshal(map[string]any{"input": trimmed})
	if err != nil {
		return nil, err
	}
	return wrapped, nil
}

func renderCallResult(r *CallToolResult) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, block := range r.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				b.WriteString(block.Text)
				if !strings.HasSuffix(block.Text, "\n") {
					b.WriteString("\n")
				}
			}
		case "image", "audio":
			fmt.Fprintf(&b, "[%s: %s, %d bytes]\n", block.Type, block.MimeType, len(block.Data))
		case "resource":
			b.WriteString("[embedded resource]\n")
			if len(block.Resource) > 0 {
				b.Write(block.Resource)
				b.WriteString("\n")
			}
		default:
			fmt.Fprintf(&b, "[unsupported content type: %s]\n", block.Type)
		}
	}
	if len(r.StructuredContent) > 0 {
		b.WriteString("\n--- structured ---\n")
		b.Write(r.StructuredContent)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func trimForSummary(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
