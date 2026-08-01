package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ElicitRequest is an elicitation/create request carried inside an
// InputRequiredResult (spec 2026-07-28) or sent as a server request by
// legacy servers.
type ElicitRequest struct {
	Mode            string          `json:"mode,omitempty"`
	Message         string          `json:"message"`
	RequestedSchema json.RawMessage `json:"requestedSchema,omitempty"`
	// ContextualInfo is set by legacy servers (2025-11-25 URL mode); it is
	// passed through to the elicitation callback.
	ContextualInfo json.RawMessage `json:"contextualInfo,omitempty"`
}

// ElicitResult is the client's response to an elicitation request.
type ElicitResult struct {
	Action  string          `json:"action"` // "accept" | "decline" | "cancel"
	Content json.RawMessage `json:"content,omitempty"`
}

// Elicitation response actions (spec 2026-07-28).
const (
	ElicitActionAccept  = "accept"
	ElicitActionDecline = "decline"
	ElicitActionCancel  = "cancel"
)

// ElicitationFn gathers structured information from the user on behalf of a
// server. The callback receives the server name (for display), the request
// message, and the requested schema; it returns the user's answer.
type ElicitationFn func(ctx context.Context, serverName string, req ElicitRequest) (*ElicitResult, error)

// handleInputRequest dispatches one entry of an InputRequiredResult to the
// handler for its method (spec 2026-07-28 MRTR). Elicitation, sampling, and
// roots are supported; anything else is rejected so the server knows the
// client does not implement it.
func (m *Manager) handleInputRequest(ctx context.Context, s *managedServer, method string, params json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "elicitation/create":
		var req ElicitRequest
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, &RPCError{Code: ErrCodeInvalidParams, Message: "invalid elicitation/create params"}
			}
		}
		m.mu.Lock()
		fn := m.elicitationFn
		m.mu.Unlock()
		if fn == nil {
			// Decline rather than fail: the server can continue without it.
			return json.Marshal(ElicitResult{Action: ElicitActionDecline})
		}
		res, err := fn(ctx, s.cfg.Name, req)
		if err != nil {
			return nil, err
		}
		if res == nil {
			return json.Marshal(ElicitResult{Action: ElicitActionCancel})
		}
		return json.Marshal(res)

	case "sampling/createMessage":
		m.mu.Lock()
		samplingFn := m.samplingFn
		m.mu.Unlock()
		if samplingFn == nil {
			return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "sampling not supported by host"}
		}
		var cmp CreateMessageParams
		if err := json.Unmarshal(params, &cmp); err != nil {
			return nil, &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		}
		res, err := samplingFn(ctx, cmp)
		if err != nil {
			return nil, err
		}
		return json.Marshal(res)

	case "roots/list":
		m.mu.Lock()
		rootsFn := m.rootsFn
		m.mu.Unlock()
		if rootsFn == nil {
			return json.Marshal(ListRootsResult{Roots: []Root{}})
		}
		roots, err := rootsFn(ctx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(ListRootsResult{Roots: roots})
	}
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: fmt.Sprintf("input request %q not supported by client", method)}
}

// onInputRequests is the ClientConfig.OnInputRequests implementation: it
// resolves each input request through handleInputRequest and returns the
// responses map the client will attach to the retried request.
func (m *Manager) onInputRequests(ctx context.Context, s *managedServer, requests map[string]InputRequest) (map[string]json.RawMessage, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	responses := make(map[string]json.RawMessage, len(requests))
	for key, req := range requests {
		res, err := m.handleInputRequest(ctx, s, req.Method, req.Params)
		if err != nil {
			return nil, err
		}
		responses[key] = res
	}
	return responses, nil
}
