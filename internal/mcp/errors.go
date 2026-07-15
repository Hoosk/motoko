package mcp

import (
	"errors"
	"fmt"
)

// ErrTransportClosed is returned when an operation fails because the
// underlying transport is no longer connected.
var ErrTransportClosed = errors.New("mcp: transport closed")

// ErrRequestCancelled is returned when a pending request is cancelled via
// notifications/cancelled. Callers should map it to a friendly message.
var ErrRequestCancelled = errors.New("mcp: request cancelled")

// ProtocolError wraps an error returned by the server that violates the
// protocol (malformed message, version mismatch, etc.).
type ProtocolError struct {
	Cause  error
	Reason string
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("mcp: protocol error: %s: %v", e.Reason, e.Cause)
	}
	return "mcp: protocol error: " + e.Reason
}

func (e *ProtocolError) Unwrap() error { return e.Cause }
