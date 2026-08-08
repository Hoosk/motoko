package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func (m *Manager) handleInboundRequest(ctx context.Context, s *managedServer, method string, params json.RawMessage) (any, error) {
	switch method {
	case "roots/list":
		m.mu.Lock()
		rootsFn := m.rootsFn
		m.mu.Unlock()
		if rootsFn == nil {
			return ListRootsResult{Roots: []Root{}}, nil
		}
		roots, err := rootsFn(ctx)
		if err != nil {
			return nil, err
		}
		return ListRootsResult{Roots: roots}, nil

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
		return samplingFn(ctx, cmp)

	case "elicitation/create":
		// Legacy path (protocol <= 2025-11-25): servers send the request
		// directly instead of the MRTR result. Modern servers go through
		// OnInputRequests.
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
			return ElicitResult{Action: ElicitActionDecline}, nil
		}
		res, err := fn(ctx, s.cfg.Name, req)
		if err != nil {
			return nil, err
		}
		if res == nil {
			return ElicitResult{Action: ElicitActionCancel}, nil
		}
		return res, nil

	default:
		return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: fmt.Sprintf("method %q not supported", method)}
	}
}
