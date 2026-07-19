package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Hoosk/motoko/internal/tracelog"
)

// Client is a single MCP session over a Transport. It is safe to call
// Request/Send concurrently from multiple goroutines; the inbound reader
// runs on its own goroutine started by Start.
type Client struct {
	serverCaps     ServerCapabilities
	capabilities   ClientCapabilities
	transport      Transport
	stopCh         chan struct{}
	pending        map[string]chan rpcResult
	doneCh         chan struct{}
	onNotification func(method string, params json.RawMessage)
	onRequest      func(ctx context.Context, method string, params json.RawMessage) (any, error)
	sema           chan struct{}
	clientInfo     Implementation
	serverInfo     Implementation
	protocol       string
	nextID         atomic.Int64
	timeout        time.Duration
	mu             sync.Mutex
	initialized    bool
	closed         bool
}

type rpcResult struct {
	err      error
	envelope RPCEnvelope
}

// ClientConfig configures a new Client.
type ClientConfig struct {
	Capabilities   ClientCapabilities
	Transport      Transport
	OnNotification func(method string, params json.RawMessage)
	OnRequest      func(ctx context.Context, method string, params json.RawMessage) (any, error)
	ClientInfo     Implementation
	RequestTimeout time.Duration
}

// NewClient wraps a transport in a Client. Call Start to launch the inbound
// reader; call Initialize to perform the handshake before issuing other
// requests.
func NewClient(cfg ClientConfig) *Client {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 30 * time.Second
	}
	return &Client{
		transport:      cfg.Transport,
		clientInfo:     cfg.ClientInfo,
		capabilities:   cfg.Capabilities,
		timeout:        cfg.RequestTimeout,
		pending:        make(map[string]chan rpcResult),
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
		onNotification: cfg.OnNotification,
		onRequest:      cfg.OnRequest,
		sema:           make(chan struct{}, 10),
	}
}

// Start launches the inbound reader goroutine. The reader runs until Close
// is called, the transport returns an error, or the context is cancelled.
func (c *Client) Start(ctx context.Context) {
	go c.readLoop(ctx)
}

// Close shuts down the inbound reader and the transport.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.stopCh)
	c.mu.Unlock()
	err := c.transport.Close()
	<-c.doneCh
	return err
}

// Initialize performs the MCP handshake. It sends an `initialize` request,
// validates the response, and emits the `notifications/initialized`
// notification.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    c.capabilities,
		ClientInfo:      c.clientInfo,
	}
	var result InitializeResult
	if err := c.Request(ctx, "initialize", params, &result); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.serverInfo = result.ServerInfo
	c.serverCaps = result.Capabilities
	c.protocol = result.ProtocolVersion
	c.initialized = true
	c.mu.Unlock()

	// notifications/initialized
	if err := c.Send(ctx, "notifications/initialized", nil); err != nil {
		return nil, fmt.Errorf("mcp: send initialized: %w", err)
	}
	return &result, nil
}

// Ping issues a `ping` request to verify the session is alive.
func (c *Client) Ping(ctx context.Context) error {
	return c.Request(ctx, "ping", nil, nil)
}

// ListTools returns the first page of tools exposed by the server. Pass a
// non-empty cursor to fetch subsequent pages.
func (c *Client) ListTools(ctx context.Context, cursor string) (*ListToolsResult, error) {
	params := struct {
		Cursor string `json:"cursor,omitempty"`
	}{Cursor: cursor}
	var result ListToolsResult
	if err := c.Request(ctx, "tools/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAllTools paginates through ListTools until the server stops returning a
// cursor or the context is cancelled.
func (c *Client) ListAllTools(ctx context.Context) ([]Tool, error) {
	var (
		cursor string
		all    []Tool
	)
	for {
		page, err := c.ListTools(ctx, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Tools...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// CallTool invokes a tool by name. The arguments payload is sent verbatim; the
// caller is responsible for producing a JSON object that matches the tool's
// inputSchema.
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (*CallToolResult, error) {
	if name == "" {
		return nil, fmt.Errorf("mcp: tool name required")
	}
	params := CallToolParams{Name: name, Arguments: arguments}
	var result CallToolResult
	if err := c.Request(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ServerCapabilities returns the capabilities advertised by the server. It is
// only populated after Initialize has been called.
func (c *Client) ServerCapabilities() ServerCapabilities {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serverCaps
}

// ServerInfo returns the server metadata reported during the handshake.
func (c *Client) ServerInfo() Implementation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serverInfo
}

// NegotiatedProtocol returns the protocol version chosen by the server.
func (c *Client) NegotiatedProtocol() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.protocol
}

// Request sends a JSON-RPC request and blocks for the matching response (or
// the configured timeout, whichever comes first). If out is non-nil the
// response result is unmarshalled into it.
func (c *Client) Request(ctx context.Context, method string, params, out any) error {
	if c.sema != nil {
		select {
		case c.sema <- struct{}{}:
			defer func() { <-c.sema }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	payload, err := c.buildRequest(method, params)
	if err != nil {
		return err
	}
	idRaw, _ := payload["id"].(json.RawMessage)
	if idRaw == nil {
		return fmt.Errorf("mcp: request %q missing id", method)
	}

	tracelog.Logf("MCP: Sending request %q (id: %s)", method, string(idRaw))

	respCh := make(chan rpcResult, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrTransportClosed
	}
	c.pending[string(idRaw)] = respCh
	c.mu.Unlock()
	defer c.removePending(string(idRaw))

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := c.transport.Send(ctx, data); err != nil {
		return err
	}

	reqCtx := ctx
	if _, ok := ctx.Deadline(); !ok && c.timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	select {
	case <-c.stopCh:
		return ErrTransportClosed
	case <-reqCtx.Done():
		tracelog.Logf("MCP: Request %q (id: %s) timed out / cancelled: %v", method, string(idRaw), reqCtx.Err())
		return reqCtx.Err()
	case res := <-respCh:
		if res.err != nil {
			tracelog.Logf("MCP: Request %q (id: %s) failed with transport error: %v", method, string(idRaw), res.err)
			return res.err
		}
		if res.envelope.Error != nil {
			tracelog.Logf("MCP: Request %q (id: %s) failed with JSON-RPC error: %s", method, string(idRaw), res.envelope.Error.Error())
			return res.envelope.Error
		}
		tracelog.Logf("MCP: Request %q (id: %s) succeeded", method, string(idRaw))
		if out == nil || len(res.envelope.Result) == 0 {
			return nil
		}
		return json.Unmarshal(res.envelope.Result, out)
	}
}

// Send emits a JSON-RPC notification (no id, no response expected).
func (c *Client) Send(ctx context.Context, method string, params any) error {
	payload, err := c.buildNotification(method, params)
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.transport.Send(ctx, data)
}

// Cancel notifies the server that a request with the given id should be
// cancelled. Spec: the client MUST NOT cancel `initialize`; this method
// trusts the caller.
func (c *Client) Cancel(ctx context.Context, id RequestID, reason string) error {
	params := struct {
		Reason    string          `json:"reason,omitempty"`
		RequestID json.RawMessage `json:"requestId,omitempty"`
	}{RequestID: id.Raw(), Reason: reason}
	return c.Send(ctx, "notifications/cancelled", params)
}

func (c *Client) buildRequest(method string, params any) (map[string]any, error) {
	id := c.nextID.Add(1)
	idRaw, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		jsonRPCField: jsonRPCVersion,
		"id":         json.RawMessage(idRaw),
		"method":     method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal params for %s: %w", method, err)
		}
		payload["params"] = json.RawMessage(raw)
	}
	return payload, nil
}

func (c *Client) buildNotification(method string, params any) (map[string]any, error) {
	payload := map[string]any{
		jsonRPCField: jsonRPCVersion,
		"method":     method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal params for %s: %w", method, err)
		}
		payload["params"] = json.RawMessage(raw)
	}
	return payload, nil
}

func (c *Client) removePending(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

func (c *Client) dispatch(env RPCEnvelope) {
	if env.IsRequest() {
		id := env.ID.Raw()
		if c.onRequest != nil {
			go func() {
				result, err := c.onRequest(context.Background(), env.Method, env.Params)
				var resp map[string]any
				if err != nil {
					code := ErrCodeInternalError
					var rpcErr *RPCError
					if errors.As(err, &rpcErr) {
						code = rpcErr.Code
					}
					resp, _ = buildErrorResponse(id, code, err.Error(), nil)
				} else {
					raw, _ := json.Marshal(result)
					resp = map[string]any{
						jsonRPCField: jsonRPCVersion,
						"id":         json.RawMessage(id),
						"result":     json.RawMessage(raw),
					}
				}
				data, _ := json.Marshal(resp)
				_ = c.transport.Send(context.Background(), data)
			}()
			return
		}
		resp, _ := buildErrorResponse(id, ErrCodeMethodNotFound, "method not supported by client", nil)
		data, _ := json.Marshal(resp)
		_ = c.transport.Send(context.Background(), data)
		return
	}
	if env.IsNotification() {
		if c.onNotification != nil {
			c.onNotification(env.Method, env.Params)
		}
		return
	}
	if !env.IsResponse() && !env.IsErrorResponse() {
		return
	}
	idKey := string(env.ID.Raw())
	c.mu.Lock()
	ch, ok := c.pending[idKey]
	if ok {
		delete(c.pending, idKey)
	}
	c.mu.Unlock()
	if ok {
		ch <- rpcResult{envelope: env}
	}
}

func (c *Client) readLoop(ctx context.Context) {
	defer close(c.doneCh)
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}
		payload, err := c.transport.Recv(readCtx)
		if err != nil {
			if isEOF(err) {
				return
			}
			if readCtx.Err() != nil {
				return
			}
			return
		}
		if len(payload) == 0 {
			continue
		}
		env, err := DecodeMessage(payload)
		if err != nil {
			// malformed frame: ignore per spec (transport-level error
			// recovery is the server's responsibility; we just keep going).
			continue
		}
		c.dispatch(env)
	}
}

func buildErrorResponse(id json.RawMessage, code int, msg string, data any) (map[string]any, error) {
	if id == nil {
		id = json.RawMessage("null")
	}
	return map[string]any{
		jsonRPCField: jsonRPCVersion,
		"id":         id,
		"error":      RPCError{Code: code, Message: msg, Data: data},
	}, nil
}

// ListResources returns the first page of resources exposed by the server.
func (c *Client) ListResources(ctx context.Context, cursor string) (*ListResourcesResult, error) {
	params := struct {
		Cursor string `json:"cursor,omitempty"`
	}{Cursor: cursor}
	var result ListResourcesResult
	if err := c.Request(ctx, "resources/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAllResources paginates through ListResources until completion.
func (c *Client) ListAllResources(ctx context.Context) ([]Resource, error) {
	var (
		cursor string
		all    []Resource
	)
	for {
		page, err := c.ListResources(ctx, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Resources...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// ListResourceTemplates returns the first page of resource templates.
func (c *Client) ListResourceTemplates(ctx context.Context, cursor string) (*ListResourceTemplatesResult, error) {
	params := struct {
		Cursor string `json:"cursor,omitempty"`
	}{Cursor: cursor}
	var result ListResourceTemplatesResult
	if err := c.Request(ctx, "resources/templates/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAllResourceTemplates paginates through ListResourceTemplates until completion.
func (c *Client) ListAllResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	var (
		cursor string
		all    []ResourceTemplate
	)
	for {
		page, err := c.ListResourceTemplates(ctx, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.ResourceTemplates...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// ReadResource fetches the contents of a resource by URI.
func (c *Client) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	params := struct {
		URI string `json:"uri"`
	}{URI: uri}
	var result ReadResourceResult
	if err := c.Request(ctx, "resources/read", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Subscribe registers interest in a resource to receive notifications/resources/updated.
func (c *Client) Subscribe(ctx context.Context, uri string) error {
	params := struct {
		URI string `json:"uri"`
	}{URI: uri}
	return c.Request(ctx, "resources/subscribe", params, nil)
}

// Unsubscribe unregisters interest in a resource.
func (c *Client) Unsubscribe(ctx context.Context, uri string) error {
	params := struct {
		URI string `json:"uri"`
	}{URI: uri}
	return c.Request(ctx, "resources/unsubscribe", params, nil)
}

// ListPrompts returns the first page of prompts.
func (c *Client) ListPrompts(ctx context.Context, cursor string) (*ListPromptsResult, error) {
	params := struct {
		Cursor string `json:"cursor,omitempty"`
	}{Cursor: cursor}
	var result ListPromptsResult
	if err := c.Request(ctx, "prompts/list", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAllPrompts paginates through ListPrompts until completion.
func (c *Client) ListAllPrompts(ctx context.Context) ([]Prompt, error) {
	var (
		cursor string
		all    []Prompt
	)
	for {
		page, err := c.ListPrompts(ctx, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Prompts...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// GetPrompt retrieves a prompt by name and arguments.
func (c *Client) GetPrompt(ctx context.Context, name string, arguments map[string]string) (*GetPromptResult, error) {
	params := struct {
		Arguments map[string]string `json:"arguments,omitempty"`
		Name      string            `json:"name"`
	}{Name: name, Arguments: arguments}
	var result GetPromptResult
	if err := c.Request(ctx, "prompts/get", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
