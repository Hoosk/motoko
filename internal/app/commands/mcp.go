package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/mcp"
	"github.com/Hoosk/motoko/internal/app/types"
)

const (
	msgNoMCPResources = "No MCP resources available."
	msgNoMCPPrompts   = "No MCP prompts available."
)

// handleDynamicPrompt resolves a command that didn't match the static
// registry against the prompts currently exposed by connected MCP servers.
// If exactly one host has a prompt with that name, the prompt is rendered
// and dispatched as a chat message; ambiguous matches surface the conflict
// to the user instead of guessing.
func (d *Dispatcher) handleDynamicPrompt(name string, args []string) (types.Response, bool) {
	if d.deps.MCPPromptHostsFn == nil || d.deps.MCPGetPromptFn == nil {
		return types.Response{}, false
	}
	ctx, cancel := withDispatchTimeout(context.Background())
	defer cancel()
	hosts := d.deps.MCPPromptHostsFn(ctx)
	var matches []mcp.PromptHost
	for _, h := range hosts {
		if strings.EqualFold(h.Name, name) {
			matches = append(matches, h)
		}
	}
	switch len(matches) {
	case 0:
		return types.Response{}, false
	case 1:
		// fall through
	default:
		servers := make([]string, 0, len(matches))
		for _, m := range matches {
			servers = append(servers, m.Server)
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Prompt /%s is hosted by multiple servers: %s. Use /mcp prompt <server> %s ... to disambiguate.", name, strings.Join(servers, ", "), name)}}}, true
	}
	parsed := make(map[string]string)
	for _, kv := range args {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Invalid argument %q (expected key=value)", kv)}}}, true
		}
		parsed[kv[:eq]] = kv[eq+1:]
	}
	result, err := d.deps.MCPGetPromptFn(ctx, matches[0].Server, matches[0].Name, parsed)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Get prompt failed: %v", err)}}}, true
	}
	var text strings.Builder
	if result.Description != "" {
		text.WriteString(result.Description)
		text.WriteString("\n\n")
	}
	for _, m := range result.Messages {
		if m.Content.Text != "" {
			text.WriteString(m.Content.Text)
			text.WriteString("\n")
		}
	}
	return types.Response{
		Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.TrimSpace(text.String())}},
	}, true
}

func (d *Dispatcher) handleMCPCommand(args []string) types.Response {
	if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "servers") || strings.EqualFold(args[0], "status") {
		return d.handleMCPList()
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "add":
		return d.handleMCPAdd(args[1:])
	case "remove":
		return d.handleMCPRemove(args[1:])
	case "tools":
		return d.handleMCPTools()
	case "info":
		return d.handleMCPInfo(args[1:])
	case "resources":
		return d.handleMCPResources(args[1:])
	case "prompts":
		return d.handleMCPPrompts(args[1:])
	case "read":
		return d.handleMCPRead(args[1:])
	case "prompt":
		return d.handleMCPGetPrompt(args[1:])

	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown subcommand: %s\nUsage: /mcp [list|add|remove|tools|info <server>|resources|prompts|read|prompt]", sub)}}}
	}
}

func (d *Dispatcher) handleMCPList() types.Response {
	var servers []mcp.ServerStatus
	if d.deps.MCPServersFn != nil {
		servers = d.deps.MCPServersFn()
	}
	if len(servers) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No MCP servers configured or running.\nAdd entries to .agents/mcp.json or application config."}}}
	}
	lines := []string{"MCP Servers:"}
	for _, s := range servers {
		statusStr := mcpStatus(s)
		lines = append(lines, fmt.Sprintf("• %s [%s] - %s (%d tools)", s.Name, s.Transport, statusStr, s.ToolCount))
		if len(s.Tools) > 0 {
			for _, t := range s.Tools {
				lines = append(lines, fmt.Sprintf("  └ %s", t))
			}
		}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
}

func (d *Dispatcher) handleMCPAdd(args []string) types.Response {
	if len(args) < 2 {
		return types.Response{
			Signal:  "open-mcp-popup",
			Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Opening MCP server configuration form..."}},
		}
	}
	name := args[0]
	srv, ok := mcpServerFromArgs(name, args[1:])
	if !ok {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /mcp add <name> http <url>"}}}
	}

	cfg := d.deps.ConfigFn()
	if cfg != nil {
		cfg.UpsertMCPServer(srv)
		_ = d.deps.SaveConfigFn()
	}
	if d.deps.AddMCPServerFn != nil {
		d.deps.AddMCPServerFn(srv)
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("MCP server added: %s", name)}}}
}

func (d *Dispatcher) handleMCPRemove(args []string) types.Response {
	if len(args) < 1 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /mcp remove <name>"}}}
	}
	name := args[0]
	removed := false
	cfg := d.deps.ConfigFn()
	if cfg != nil {
		removed = cfg.RemoveMCPServer(name)
		if removed {
			_ = d.deps.SaveConfigFn()
		}
	}
	if d.deps.RemoveMCPServerFn != nil {
		if d.deps.RemoveMCPServerFn(name) {
			removed = true
		}
	}
	if !removed {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("MCP server not found: %s", name)}}}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("MCP server removed: %s", name)}}}
}

func (d *Dispatcher) handleMCPTools() types.Response {
	if d.deps.ToolSpecsFn == nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No MCP tools registered."}}}
	}
	specs := d.deps.ToolSpecsFn()
	var lines []string
	lines = append(lines, "Registered MCP Tools:")
	count := 0
	for _, spec := range specs {
		if strings.HasPrefix(strings.ToLower(spec.Name), "mcp_") {
			count++
			lines = append(lines, fmt.Sprintf("• %s: %s", spec.Name, spec.Summary))
		}
	}
	if count == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No MCP tools registered."}}}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
}

func (d *Dispatcher) handleMCPInfo(args []string) types.Response {
	if len(args) < 1 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /mcp info <server>"}}}
	}
	name := strings.ToLower(args[0])
	var servers []mcp.ServerStatus
	if d.deps.MCPServersFn != nil {
		servers = d.deps.MCPServersFn()
	}
	for _, s := range servers {
		if strings.ToLower(s.Name) == name {
			statusStr := mcpStatus(s)
			lines := []string{
				fmt.Sprintf("Server: %s", s.Name),
				fmt.Sprintf("Transport: %s", s.Transport),
				fmt.Sprintf("Status: %s", statusStr),
				fmt.Sprintf("Tools (%d): %s", s.ToolCount, strings.Join(s.Tools, ", ")),
			}
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
		}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("MCP server not found: %s", args[0])}}}
}

// handleMCPResources lists resources visible to the user. With no
// arguments, it shows every resource across all connected servers. With
// one argument, it filters to the given server.
func (d *Dispatcher) handleMCPResources(args []string) types.Response {
	if d.deps.MCPResourcesFn == nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: msgNoMCPResources}}}
	}
	ctx, cancel := withDispatchTimeout(context.Background())
	defer cancel()
	all := d.deps.MCPResourcesFn(ctx)
	if len(all) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: msgNoMCPResources}}}
	}
	filter := ""
	if len(args) > 0 {
		filter = strings.ToLower(args[0])
	}
	var lines []string
	lines = append(lines, "MCP Resources:")
	count := 0
	for _, r := range all {
		if filter != "" && !strings.Contains(strings.ToLower(r.Name), filter) {
			continue
		}
		count++
		title := r.Name
		if r.Title != "" {
			title = r.Title
		}
		lines = append(lines, fmt.Sprintf("• %s  %s  [%s]", title, r.URI, r.MimeType))
	}
	if count == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: msgNoMCPResources}}}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
}

// handleMCPRead reads a resource from a named MCP server. Usage:
// /mcp read <server> <uri>
func (d *Dispatcher) handleMCPRead(args []string) types.Response {
	if len(args) < 2 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /mcp read <server> <uri>"}}}
	}
	if d.deps.MCPResourceReadFn == nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Resource reads are not available."}}}
	}
	ctx, cancel := withDispatchTimeout(context.Background())
	defer cancel()
	serverName := args[0]
	uri := args[1]
	result, err := d.deps.MCPResourceReadFn(ctx, serverName, uri)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Read failed: %v", err)}}}
	}
	var lines []string
	for i, c := range result.Contents {
		header := fmt.Sprintf("[%d] %s (%s)", i+1, c.URI, c.MimeType)
		if c.Text != "" {
			header += "\n" + c.Text
		} else if c.Blob != "" {
			header += "\n<binary blob, " + humanBlobSize(int64(len(c.Blob))) + ">"
		}
		lines = append(lines, header)
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
}

// handleMCPPrompts lists prompt templates exposed by the connected servers.
func (d *Dispatcher) handleMCPPrompts(args []string) types.Response {
	if d.deps.MCPPromptsFn == nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: msgNoMCPPrompts}}}
	}
	ctx, cancel := withDispatchTimeout(context.Background())
	defer cancel()
	all := d.deps.MCPPromptsFn(ctx)
	if len(all) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: msgNoMCPPrompts}}}
	}
	filter := ""
	if len(args) > 0 {
		filter = strings.ToLower(args[0])
	}
	var lines []string
	lines = append(lines, "MCP Prompts:")
	count := 0
	for _, p := range all {
		if filter != "" && !strings.Contains(strings.ToLower(p.Name), filter) {
			continue
		}
		count++
		desc := p.Description
		if desc == "" {
			desc = "(no description)"
		}
		argList := make([]string, 0, len(p.Arguments))
		for _, a := range p.Arguments {
			label := a.Name
			if a.Required {
				label += "*"
			}
			argList = append(argList, label)
		}
		argSuffix := ""
		if len(argList) > 0 {
			argSuffix = "  args: " + strings.Join(argList, ", ")
		}
		lines = append(lines, fmt.Sprintf("• %s — %s%s", p.Name, desc, argSuffix))
	}
	if count == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: msgNoMCPPrompts}}}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
}

// handleMCPGetPrompt fetches and renders a prompt. Usage:
// /mcp prompt <server> <name> [key=value ...]
func (d *Dispatcher) handleMCPGetPrompt(args []string) types.Response {
	if len(args) < 2 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /mcp prompt <server> <name> [key=value ...]"}}}
	}
	if d.deps.MCPGetPromptFn == nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Prompts are not available."}}}
	}
	ctx, cancel := withDispatchTimeout(context.Background())
	defer cancel()
	serverName := args[0]
	name := args[1]
	parsed := make(map[string]string)
	for _, kv := range args[2:] {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Invalid argument %q (expected key=value)", kv)}}}
		}
		parsed[kv[:eq]] = kv[eq+1:]
	}
	result, err := d.deps.MCPGetPromptFn(ctx, serverName, name, parsed)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Get prompt failed: %v", err)}}}
	}
	var lines []string
	if result.Description != "" {
		lines = append(lines, result.Description)
	}
	for i, m := range result.Messages {
		header := fmt.Sprintf("[%d] %s:", i+1, m.Role)
		if m.Content.Text != "" {
			header += "\n" + m.Content.Text
		} else if m.Content.Type != "" {
			header += fmt.Sprintf("\n<%s block>", m.Content.Type)
		}
		lines = append(lines, header)
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n\n")}}}
}

// mcpStatus renders the human-readable status of a server.
func mcpStatus(s mcp.ServerStatus) string {
	if !s.Connected {
		return "Disconnected"
	}
	if s.Err != nil {
		return fmt.Sprintf("Error (%v)", s.Err)
	}
	return "Connected"
}

// mcpServerFromArgs builds a server config from the /mcp add arguments.
// The transport is inferred: an explicit http(s) marker followed by a URL
// selects HTTP, an http(s) URL selects HTTP directly, and anything else is
// treated as a stdio command. The second return value reports whether the
// arguments were valid for the chosen transport.
func mcpServerFromArgs(name string, args []string) (config.MCPServerConfig, bool) {
	if strings.EqualFold(args[0], "http") || strings.EqualFold(args[0], "https") {
		if len(args) < 2 {
			return config.MCPServerConfig{}, false
		}
		return config.MCPServerConfig{
			Name:      name,
			Transport: "http",
			URL:       args[1],
		}, true
	}
	if strings.HasPrefix(strings.ToLower(args[0]), "http://") || strings.HasPrefix(strings.ToLower(args[0]), "https://") {
		return config.MCPServerConfig{
			Name:      name,
			Transport: "http",
			URL:       args[0],
		}, true
	}
	return config.MCPServerConfig{
		Name:      name,
		Transport: "stdio",
		Command:   args[0],
		Args:      args[1:],
	}, true
}

// withDispatchTimeout returns a context bounded to a short timeout. The
// dispatcher handlers run synchronously in the UI loop, so each MCP
// round-trip must not block the TUI indefinitely.
func withDispatchTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 10*time.Second)
}

// humanBlobSize formats a base64-decoded byte count for display. The MCP
// payload is base64 so the raw character count would mislead; we keep
// the formula straightforward (3/4 of the length).
func humanBlobSize(b64Len int64) string {
	bytes := b64Len * 3 / 4
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/(1024*1024))
	}
}
