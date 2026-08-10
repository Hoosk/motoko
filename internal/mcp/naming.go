package mcp

import "strings"

// ToolPrefix produces the registry name for a tool exposed by a given server.
// We prefix with the server name to avoid collisions across servers and with
// the local tool catalog. The original (unprefixed) name is preserved in the
// remote tool's spec so the MCP server sees it unchanged.
func ToolPrefix(serverName, toolName string) string {
	serverSlug := slugify(serverName)
	tool := sanitizeToolName(toolName)
	return "mcp_" + serverSlug + "_" + tool
}

// sanitizeToolName enforces the tool-name format from the MCP spec (2026-07-28):
// 1-128 characters, case-sensitive, only A-Za-z0-9 `_` `-` `.`. Names that
// contain characters outside that set (e.g. a server exposing "my tool") are
// normalized by replacing each invalid rune with `_`; the name is truncated to
// 128 chars and never returns empty. The original name is kept in the remote
// tool's spec so the server still sees its own name on tools/call.
func sanitizeToolName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.TrimLeft(b.String(), "._-")
	if name == "" {
		return "tool"
	}
	if len(name) > 128 {
		name = name[:128]
	}
	return name
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('_')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}

func deriveName(cfg ServerConfig) string {
	if cfg.Name != "" {
		return cfg.Name
	}
	if cfg.Command != "" {
		return cfg.Command
	}
	if cfg.URL != "" {
		return cfg.URL
	}
	return "mcp-server"
}
