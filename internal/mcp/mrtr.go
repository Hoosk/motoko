package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Hoosk/motoko/internal/tracelog"
)

// InputRequest is one entry of an InputRequiredResult.inputRequests map
// (spec 2026-07-28 MRTR). The method is one of elicitation/create,
// sampling/createMessage, or roots/list; params is passed to the matching
// handler verbatim.
type InputRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// InputRequiredResult is a result with resultType "input_required" returned
// by a server instead of the final answer, asking the client to gather
// additional input and retry (spec 2026-07-28 MRTR).
type InputRequiredResult struct {
	InputRequests map[string]InputRequest `json:"inputRequests,omitempty"`
	RequestState  string                  `json:"requestState,omitempty"`
}

// inputRequiredHeader is the minimal decode of any result used to detect the
// resultType discriminant.
type inputRequiredHeader struct {
	ResultType string `json:"resultType"`
}

// OnInputRequests is invoked when a server returns an InputRequiredResult.
// It receives the map of input requests and must return the corresponding
// input responses keyed by the same identifiers. Implementations decide
// which request types they support (elicitation, sampling, roots).
type OnInputRequests func(ctx context.Context, requests map[string]InputRequest) (map[string]json.RawMessage, error)

// maxMRTRIterations bounds the retry loop for a single Request so a server
// that keeps answering input_required cannot spin the client forever.
const maxMRTRIterations = 5

// isInputRequired reports whether the raw result bytes carry the
// input_required result type discriminant.
func isInputRequired(result json.RawMessage) bool {
	if len(result) == 0 {
		return false
	}
	var h inputRequiredHeader
	if err := json.Unmarshal(result, &h); err != nil {
		return false
	}
	return h.ResultType == "input_required"
}

// handleInputRequired processes an InputRequiredResult: it gathers the input
// via the callback, then retries the original request with inputResponses and
// requestState appended to the parameters. Returns the final result bytes.
func (c *Client) handleInputRequired(ctx context.Context, method string, originalParams json.RawMessage, irr InputRequiredResult) (json.RawMessage, error) {
	if c.onInputRequests == nil {
		return nil, &RPCError{
			Code:    ErrCodeInvalidParams,
			Message: "server requested input the client does not support",
		}
	}

	responses, err := c.onInputRequests(ctx, irr.InputRequests)
	if err != nil {
		return nil, fmt.Errorf("mcp: gather input for %s: %w", method, err)
	}

	// Merge the original params with inputResponses + requestState.
	var params map[string]any
	if len(originalParams) > 0 {
		if err = json.Unmarshal(originalParams, &params); err != nil {
			return nil, fmt.Errorf("mcp: decode params for %s retry: %w", method, err)
		}
	}
	if params == nil {
		params = make(map[string]any)
	}
	if responses != nil {
		params["inputResponses"] = responses
	}
	if irr.RequestState != "" {
		params["requestState"] = irr.RequestState
	}

	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal retry params for %s: %w", method, err)
	}
	return c.retryRequest(ctx, method, raw)
}

// retryRequest sends a request whose params are already JSON-encoded, waits
// for the response, and returns the result bytes. Unlike Request it does not
// recurse into MRTR (the caller controls the iteration budget).
func (c *Client) retryRequest(ctx context.Context, method string, paramsJSON json.RawMessage) (json.RawMessage, error) {
	payload, err := c.buildRequestRaw(method, paramsJSON)
	if err != nil {
		return nil, err
	}
	idRaw, _ := payload["id"].(json.RawMessage)
	if idRaw == nil {
		return nil, fmt.Errorf("mcp: request %q missing id", method)
	}
	tracelog.Logf("MCP: Sending MRTR retry %q (id: %s)", method, string(idRaw))

	respCh := make(chan rpcResult, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrTransportClosed
	}
	c.pending[string(idRaw)] = respCh
	c.mu.Unlock()
	defer c.removePending(string(idRaw))

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if err := c.transport.Send(ctx, data); err != nil {
		return nil, err
	}

	reqCtx := ctx
	if _, ok := ctx.Deadline(); !ok && c.timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	select {
	case <-c.stopCh:
		return nil, ErrTransportClosed
	case <-reqCtx.Done():
		return nil, reqCtx.Err()
	case res := <-respCh:
		if res.err != nil {
			return nil, res.err
		}
		if res.envelope.Error != nil {
			return nil, res.envelope.Error
		}
		return res.envelope.Result, nil
	}
}

// buildRequestRaw is like buildRequest but takes pre-encoded params.
func (c *Client) buildRequestRaw(method string, paramsJSON json.RawMessage) (map[string]any, error) {
	id := c.nextID.Add(1)
	idRaw, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		jsonRPCField: jsonRPCVersion,
		"id":         json.RawMessage(idRaw),
		methodField:  method,
	}
	if len(paramsJSON) > 0 {
		payload["params"] = paramsJSON
	}
	if meta := c.buildMeta(); meta != nil {
		raw, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal _meta for %s: %w", method, err)
		}
		payload[metaField] = json.RawMessage(raw)
	}
	return payload, nil
}
