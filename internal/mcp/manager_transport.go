package mcp

import (
	"fmt"
	"strings"
)

// buildTransport creates a Transport for the given server config. The returned
// cleanup, when non-nil, must be called once the transport is no longer
// needed; it flushes and closes per-transport resources (notably the stderr
// writer for stdio servers).
//
// For HTTP-style transports the Streamable HTTP transport (spec 2025-06-18)
// is tried first. If the endpoint does not support it (returns 4xx on the
// initialize POST), the legacy HTTP+SSE transport from spec 2024-11-05 is
// used as a fallback so older servers keep working.
func buildTransport(cfg ServerConfig) (Transport, func(), error) {
	switch strings.ToLower(cfg.Transport) {
	case "stdio", "":
		stderr := newStderrWriter(cfg.Name)
		t, err := NewStdioTransport(StdioConfig{
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
			Stderr:  stderr,
		})
		if err != nil {
			return nil, nil, err
		}
		return t, func() { _ = stderr.Close() }, nil
	case "sse":
		return NewHTTPTransport(cfg.URL, cfg.Headers, 0), nil, nil
	case "http", "https", "streamable":
		return NewStreamableTransport(StreamableConfig{
			Endpoint: cfg.URL,
			Headers:  cfg.Headers,
		}), nil, nil
	default:
		return nil, nil, fmt.Errorf("mcp: transport %q not supported", cfg.Transport)
	}
}

func defaultClientInfo() Implementation {
	return Implementation{
		Name:        "motoko",
		Title:       "Motoko",
		Version:     "0.1.0",
		Description: "Motoko terminal coding assistant acting as MCP host.",
	}
}

func defaultClientCapabilities() ClientCapabilities {
	roots := struct {
		ListChanged bool `json:"listChanged,omitempty"`
	}{ListChanged: true}
	sampling := struct{}{}
	elicitation := struct{}{}
	// Elicitation is advertised now that both the legacy inbound request and
	// the MRTR input-request paths are implemented (elicitationFn set by the
	// runtime). If the runtime leaves the callback nil, servers that use it
	// receive a decline rather than MethodNotFound.
	return ClientCapabilities{
		Roots:       &roots,
		Sampling:    &sampling,
		Elicitation: &elicitation,
	}
}
