package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/Hoosk/motoko/internal/tracelog"
)

// Protocol versions supported by this client, newest first. The legacy
// const ProtocolVersion ("2025-11-25") is what the initialize handshake
// advertises; modern servers (2026-07-28 and later) negotiate via
// server/discover and are driven in stateless mode.
const (
	ProtocolVersionModern = "2026-07-28"
)

// SupportedProtocolVersions lists the protocol revisions this client can
// speak, ordered newest-first. Negotiation picks the highest version the
// server also supports.
var SupportedProtocolVersions = []string{
	ProtocolVersionModern,
	ProtocolVersion,
}

// DiscoveryCapabilities mirrors the feature list a server reports via
// server/discover (spec 2026-07-28). Unknown fields are ignored.
type DiscoveryCapabilities struct {
	Extensions map[string]any `json:"extensions,omitempty"`
}

// DiscoverResult is the response of `server/discover`.
type DiscoverResult struct {
	Capabilities     DiscoveryCapabilities      `json:"capabilities,omitempty"`
	Meta             map[string]json.RawMessage `json:"_meta,omitempty"`
	ServerInfo       Implementation             `json:"serverInfo,omitempty"`
	Instructions     string                     `json:"instructions,omitempty"`
	ProtocolVersions []string                   `json:"protocolVersions"`
}

// metaField is the canonical name of the per-request metadata field that
// carries protocol version, client identity, and client capabilities in the
// stateless protocol.
const metaField = "_meta"

const (
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientInfo         = "io.modelcontextprotocol/clientInfo"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
)

// Negotiate performs protocol negotiation for a fresh connection:
//
//  1. Try `server/discover`. A modern server answers with its supported
//     protocol versions; we pick the highest mutual version and record the
//     server identity + capabilities.
//  2. If the probe fails for any reason other than context cancellation or
//     deadline (MethodNotFound, InvalidParams, another RPC error, an HTTP
//     error, transport failure), fall back to the classic `initialize`
//     handshake used by protocol revisions up to 2025-11-25. If that fails
//     too, the returned error mentions both failures.
//
// After Negotiate returns, callers can query NegotiatedProtocol to learn
// which mode the connection is running in.
func (c *Client) Negotiate(ctx context.Context) error {
	// Probe with server/discover. The probe itself is version-agnostic; the
	// server decides based on its own implementation.
	var discover DiscoverResult
	err := c.Request(ctx, "server/discover", nil, &discover)

	switch {
	case err == nil:
		// A modern server. Pick the highest mutual version.
		version, ok := highestMutualVersion(discover.ProtocolVersions)
		if !ok {
			return fmt.Errorf("mcp: server %q supports none of %v", discover.ServerInfo.Name, SupportedProtocolVersions)
		}
		c.mu.Lock()
		c.protocol = version
		c.serverInfo = discover.ServerInfo
		c.initialized = true // stateless mode: no handshake needed
		c.mu.Unlock()
		tracelog.Logf("MCP: negotiated protocol %s via server/discover", version)
		if setter, ok := c.transport.(ProtocolSetter); ok {
			setter.SetProtocol(version)
		}
		return nil

	case ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("mcp: server/discover probe failed: %w", err)

	default:
		// Legacy servers reject the unknown pre-initialize method in many
		// ways (-32601, -32602, other RPC errors, HTTP 400). Try the classic
		// handshake instead of giving up.
		tracelog.Logf("MCP: server/discover probe failed, falling back to initialize (%v)", err)
		if _, initErr := c.Initialize(ctx); initErr != nil {
			return fmt.Errorf("mcp: server/discover probe failed: %v; initialize fallback failed: %w", err, initErr)
		}
		return nil
	}
}

// highestMutualVersion picks the newest protocol version shared between the
// client's supported list and the server's advertised list.
func highestMutualVersion(serverVersions []string) (string, bool) {
	want := make(map[string]bool, len(serverVersions))
	for _, v := range serverVersions {
		want[v] = true
	}
	for _, v := range SupportedProtocolVersions {
		if want[v] {
			return v, true
		}
	}
	return "", false
}

// buildMeta returns the _meta object for a request in modern (stateless)
// mode, or nil when running a legacy connection.
func (c *Client) buildMeta() map[string]any {
	c.mu.Lock()
	modern := c.protocol == ProtocolVersionModern
	protocol := c.protocol
	clientInfo := c.clientInfo
	capabilities := c.capabilities
	c.mu.Unlock()
	if !modern {
		return nil
	}
	meta := map[string]any{
		metaProtocolVersion: protocol,
		metaClientInfo: map[string]any{
			"name":    clientInfo.Name,
			"version": clientInfo.Version,
		},
		metaClientCapabilities: modernClientCapabilities(capabilities),
	}
	return meta
}

// modernClientCapabilities filters the legacy capability set to what the
// stateless protocol accepts. Roots, sampling, and logging are deprecated in
// 2026-07-28, so they are not advertised on modern connections; elicitation
// is implemented (legacy request + MRTR) and remains a modern capability.
func modernClientCapabilities(in ClientCapabilities) map[string]any {
	out := make(map[string]any)
	if in.Experimental != nil {
		out["experimental"] = in.Experimental
	}
	if in.Elicitation != nil {
		out["elicitation"] = in.Elicitation
	}
	return out
}

// sortedVersions is a small helper for deterministic tests.
func sortedVersions(vs []string) []string {
	out := append([]string(nil), vs...)
	sort.Strings(out)
	return out
}
