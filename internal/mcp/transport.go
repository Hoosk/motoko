package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
)

// Transport is the bidirectional byte-stream used by an MCP session. Messages
// are individual JSON-RPC payloads, newline-delimited. The transport is
// responsible for encoding/decoding that framing; the Client above it just
// reads and writes JSON-RPC envelopes.
type Transport interface {
	// Send writes a single JSON-RPC payload to the server.
	Send(ctx context.Context, payload []byte) error

	// Recv returns the next JSON-RPC payload from the server. It must block
	// until a message arrives, the context is cancelled, or the transport is
	// closed. Returning io.EOF is treated as a clean disconnect.
	Recv(ctx context.Context) ([]byte, error)

	// Close shuts the transport down. It MUST be safe to call multiple times
	// and MUST NOT block indefinitely.
	Close() error
}

// Encoder writes a JSON-RPC envelope onto a transport.
func EncodeMessage(v any) ([]byte, error) {
	return json.Marshal(v)
}

// DecodeMessage parses a JSON-RPC payload into an envelope.
func DecodeMessage(payload []byte) (RPCEnvelope, error) {
	var env RPCEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return RPCEnvelope{}, err
	}
	return env, nil
}

// isEOF reports whether err signals a clean end of stream.
func isEOF(err error) bool { return errors.Is(err, io.EOF) }
