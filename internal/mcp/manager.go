package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ToolRegistrar is the abstraction the manager uses to publish tools into
// the host. It is implemented as a pair of callbacks so the mcp package
// remains free of an import cycle with internal/tools.
//
// Register is called once per (server, tool) that the manager wants to make
// available. Unregister is called when a tool disappears (server shutdown or
// notifications/tools/list_changed) and receives the same prefixed name the
// registrar saw in Register.
type ToolRegistrar struct {
	Register   func(tool ToolAdapter)
	Unregister func(name string) bool
}

// ToolAdapter is the surface a tool must expose to be registered.
type ToolAdapter interface {
	Spec() ToolSpec
	Run(ctx context.Context, args string) (ToolResult, error)
}

// ToolSpec is the tool metadata exposed to the host.
type ToolSpec struct {
	Name        string
	Title       string
	Summary     string
	Description string
	Usage       string
	ReadOnly    bool
}

// ToolResult is the result of a tool invocation.
type ToolResult struct {
	Summary string
	Output  string
	Spec    ToolSpec
}

// pendingRegistration carries the data required to build a RemoteTool during
// a refresh pass.
type pendingRegistration struct {
	manager *Manager
	name    string
	server  string
	tool    Tool
}

// ServerConfig is the configuration of a single MCP server. The full struct
// (with command/args/env/url/headers) is declared in the config package; we
// only need the transport-agnostic fields here.
type ServerConfig struct {
	Headers   map[string]string
	Name      string
	Transport string
	Command   string
	URL       string
	Args      []string
	Env       []string
	Disabled  bool
}

// Manager owns the set of connected MCP servers and synchronises their tools
// with the host's tool registry.
type Manager struct {
	registry          ToolRegistrar
	servers           map[string]*managedServer
	onResourceUpdated func(serverName string, uri string)
	rootsFn           func(ctx context.Context) ([]Root, error)
	samplingFn        func(ctx context.Context, params CreateMessageParams) (*CreateMessageResult, error)
	timeout           time.Duration
	mu                sync.Mutex
}

// managedServer wraps a Client with the bookkeeping required to track its
// currently-registered tools, resources and prompts.
type managedServer struct {
	err       error
	client    *Client
	tools     map[string]bool
	cancel    context.CancelFunc
	resources []Resource
	templates []ResourceTemplate
	prompts   []Prompt
	cfg       ServerConfig
}

// ManagerConfig configures the manager.
type ManagerConfig struct {
	Capabilities ClientCapabilities
	Registry     ToolRegistrar
	RootsFn      func(ctx context.Context) ([]Root, error)
	SamplingFn   func(ctx context.Context, params CreateMessageParams) (*CreateMessageResult, error)
	ClientInfo   Implementation
	Timeout      time.Duration
}

// NewManager creates a manager. The given registry receives the tools
// exposed by every successfully-started server. The ClientInfo and
// Capabilities are reserved for future customisation; in phase 1 we use
// the Motoko defaults regardless of the values passed.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	_ = cfg.ClientInfo
	_ = cfg.Capabilities
	return &Manager{
		registry:   cfg.Registry,
		timeout:    cfg.Timeout,
		servers:    make(map[string]*managedServer),
		rootsFn:    cfg.RootsFn,
		samplingFn: cfg.SamplingFn,
	}
}

// lookupServer returns a snapshot of the managed server with the given name.
func (m *Manager) lookupServer(name string) (*managedServer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[name]
	if !ok {
		return nil, fmt.Errorf("mcp: unknown server %q", name)
	}
	return s, nil
}

// Start launches the given servers. Already-running servers with the same
// name are replaced. Each server runs on its own goroutine.
func (m *Manager) Start(ctx context.Context, servers []ServerConfig) {
	if m == nil {
		return
	}
	for _, cfg := range servers {
		if cfg.Disabled {
			continue
		}
		m.startOne(ctx, cfg)
	}
}

// Stop shuts every server down. After Stop returns, no more tools belonging
// to MCP servers remain in the registry.
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	servers := make([]*managedServer, 0, len(m.servers))
	for _, s := range m.servers {
		servers = append(servers, s)
	}
	m.servers = make(map[string]*managedServer)
	m.mu.Unlock()
	for _, s := range servers {
		m.unregisterServerTools(s)
		if s.cancel != nil {
			s.cancel()
		}
		if s.client != nil {
			_ = s.client.Close()
		}
	}
}

// Servers returns a snapshot of the currently-tracked servers along with
// their status. Useful for `/mcp servers` and TUI status panels.
func (m *Manager) Servers() []ServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ServerStatus, 0, len(names))
	for _, name := range names {
		s := m.servers[name]
		tools := make([]string, 0, len(s.tools))
		for t := range s.tools {
			tools = append(tools, t)
		}
		sort.Strings(tools)
		out = append(out, ServerStatus{
			Name:      name,
			Transport: s.cfg.Transport,
			Connected: s.client != nil,
			ToolCount: len(s.tools),
			Tools:     tools,
			Err:       s.err,
		})
	}
	return out
}

// ServerStatus is the snapshot exposed by Servers.
type ServerStatus struct {
	Err       error
	Name      string
	Transport string
	Tools     []string
	ToolCount int
	Connected bool
}

func (m *Manager) startOne(parent context.Context, cfg ServerConfig) {
	if cfg.Name == "" {
		// Spec doesn't require names but we need them to deduplicate.
		cfg.Name = deriveName(cfg)
	}

	serverCtx, cancel := context.WithCancel(parent)
	m.mu.Lock()
	if existing, ok := m.servers[cfg.Name]; ok {
		// Replace: tear down old client first.
		m.unregisterServerTools(existing)
		if existing.cancel != nil {
			existing.cancel()
		}
		if existing.client != nil {
			_ = existing.client.Close()
		}
	}
	entry := &managedServer{
		cfg:    cfg,
		tools:  make(map[string]bool),
		cancel: cancel,
	}
	m.servers[cfg.Name] = entry
	m.mu.Unlock()

	go m.runServer(serverCtx, entry)
}

func (m *Manager) runServer(ctx context.Context, s *managedServer) {
	backoff := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		transport, err := buildTransport(s.cfg)
		if err != nil {
			m.markErr(s, fmt.Errorf("transport: %w", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, 30*time.Second)
				continue
			}
		}

		client := NewClient(ClientConfig{
			Transport:      transport,
			ClientInfo:     defaultClientInfo(),
			Capabilities:   defaultClientCapabilities(),
			RequestTimeout: m.timeout,
			OnNotification: func(method string, params json.RawMessage) {
				m.handleNotification(s, method, params)
			},
			OnRequest: func(ctx context.Context, method string, params json.RawMessage) (any, error) {
				return m.handleInboundRequest(ctx, s, method, params)
			},
		})
		client.Start(ctx)

		if _, err := client.Initialize(ctx); err != nil {
			m.markErr(s, fmt.Errorf("initialize: %w", err))
			_ = client.Close()
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, 30*time.Second)
				continue
			}
		}

		m.mu.Lock()
		s.client = client
		s.err = nil
		m.mu.Unlock()

		// Reset backoff on successful connection
		backoff = 1 * time.Second

		caps := client.ServerCapabilities()
		if caps.Tools != nil {
			if err := m.refreshTools(ctx, s); err != nil {
				m.markErr(s, err)
			}
		}
		if caps.Resources != nil {
			if err := m.refreshResources(ctx, s); err != nil {
				m.markErr(s, err)
			}
			if err := m.refreshTemplates(ctx, s); err != nil {
				m.markErr(s, err)
			}
		}
		if caps.Prompts != nil {
			if err := m.refreshPrompts(ctx, s); err != nil {
				m.markErr(s, err)
			}
		}

		// Block until context is cancelled or client exits
		select {
		case <-ctx.Done():
			return
		case <-client.doneCh:
			m.mu.Lock()
			s.client = nil
			m.unregisterServerTools(s)
			m.mu.Unlock()
		}
	}
}

func (m *Manager) markErr(s *managedServer, err error) {
	m.mu.Lock()
	s.err = err
	m.mu.Unlock()
}

func (m *Manager) handleNotification(s *managedServer, method string, params json.RawMessage) {
	switch method {
	case "notifications/tools/list_changed":
		m.mu.Lock()
		client := s.client
		m.mu.Unlock()
		if client == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()
		if err := m.refreshTools(ctx, s); err != nil {
			m.markErr(s, err)
		}
	case "notifications/resources/list_changed":
		m.mu.Lock()
		client := s.client
		m.mu.Unlock()
		if client == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()
		if err := m.refreshResources(ctx, s); err != nil {
			m.markErr(s, err)
		}
		if err := m.refreshTemplates(ctx, s); err != nil {
			m.markErr(s, err)
		}
	case "notifications/prompts/list_changed":
		m.mu.Lock()
		client := s.client
		m.mu.Unlock()
		if client == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()
		if err := m.refreshPrompts(ctx, s); err != nil {
			m.markErr(s, err)
		}
	case "notifications/resources/updated":
		var updated struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(params, &updated); err == nil {
			m.mu.Lock()
			cb := m.onResourceUpdated
			name := s.cfg.Name
			m.mu.Unlock()
			if cb != nil {
				cb(name, updated.URI)
			}
		}
	case "notifications/message", "notifications/progress", "notifications/cancelled":
		// Phase 1: log to stderr-equivalent; the manager could forward to a
		// host logger in a later phase. No-op for now.
	}
}

func (m *Manager) refreshTools(ctx context.Context, s *managedServer) error {
	client := s.client
	if client == nil {
		return nil
	}
	tools, err := client.ListAllTools(ctx)
	if err != nil {
		return err
	}
	// Compute new set with prefix.
	newSet := make(map[string]bool, len(tools))
	pending := make([]pendingRegistration, 0, len(tools))
	for _, t := range tools {
		registeredName := ToolPrefix(s.cfg.Name, t.Name)
		newSet[registeredName] = true
		pending = append(pending, pendingRegistration{
			name:    registeredName,
			server:  s.cfg.Name,
			tool:    t,
			manager: m,
		})
	}

	// Remove tools that disappeared.
	m.mu.Lock()
	old := s.tools
	m.mu.Unlock()
	for name := range old {
		if !newSet[name] {
			m.unregister(name)
		}
	}
	// Add new / replacement tools.
	m.mu.Lock()
	s.tools = newSet
	m.mu.Unlock()
	for _, reg := range pending {
		m.register(NewRemoteToolAdapter(reg.server, reg.name, reg.tool, reg.manager))
	}
	return nil
}

func (m *Manager) unregisterServerTools(s *managedServer) {
	if s == nil {
		return
	}
	for name := range s.tools {
		m.unregister(name)
	}
}

func (m *Manager) refreshResources(ctx context.Context, s *managedServer) error {
	client := s.client
	if client == nil {
		return nil
	}
	resources, err := client.ListAllResources(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s.resources = resources
	m.mu.Unlock()
	return nil
}

func (m *Manager) refreshTemplates(ctx context.Context, s *managedServer) error {
	client := s.client
	if client == nil {
		return nil
	}
	templates, err := client.ListAllResourceTemplates(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s.templates = templates
	m.mu.Unlock()
	return nil
}

func (m *Manager) refreshPrompts(ctx context.Context, s *managedServer) error {
	client := s.client
	if client == nil {
		return nil
	}
	prompts, err := client.ListAllPrompts(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s.prompts = prompts
	m.mu.Unlock()
	return nil
}

// ListResources returns all cached resources across all connected servers.
func (m *Manager) ListResources(ctx context.Context) []Resource {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Resource
	for _, s := range m.servers {
		all = append(all, s.resources...)
	}
	return all
}

// ListResourceTemplates returns all cached resource templates across all connected servers.
func (m *Manager) ListResourceTemplates(ctx context.Context) []ResourceTemplate {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []ResourceTemplate
	for _, s := range m.servers {
		all = append(all, s.templates...)
	}
	return all
}

// ReadResource reads a resource from the specified server.
func (m *Manager) ReadResource(ctx context.Context, serverName string, uri string) (*ReadResourceResult, error) {
	s, err := m.lookupServer(serverName)
	if err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, fmt.Errorf("mcp: server %q not connected", serverName)
	}
	return s.client.ReadResource(ctx, uri)
}

// SubscribeResource subscribes to updates for a resource on the specified server.
func (m *Manager) SubscribeResource(ctx context.Context, serverName string, uri string) error {
	s, err := m.lookupServer(serverName)
	if err != nil {
		return err
	}
	if s.client == nil {
		return fmt.Errorf("mcp: server %q not connected", serverName)
	}
	return s.client.Subscribe(ctx, uri)
}

// UnsubscribeResource unsubscribes from updates for a resource on the specified server.
func (m *Manager) UnsubscribeResource(ctx context.Context, serverName string, uri string) error {
	s, err := m.lookupServer(serverName)
	if err != nil {
		return err
	}
	if s.client == nil {
		return fmt.Errorf("mcp: server %q not connected", serverName)
	}
	return s.client.Unsubscribe(ctx, uri)
}

// ListPrompts returns all cached prompts across all connected servers.
func (m *Manager) ListPrompts(ctx context.Context) []Prompt {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Prompt
	for _, s := range m.servers {
		all = append(all, s.prompts...)
	}
	return all
}

// GetPrompt retrieves a prompt from the specified server.
func (m *Manager) GetPrompt(ctx context.Context, serverName string, promptName string, arguments map[string]string) (*GetPromptResult, error) {
	s, err := m.lookupServer(serverName)
	if err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, fmt.Errorf("mcp: server %q not connected", serverName)
	}
	return s.client.GetPrompt(ctx, promptName, arguments)
}

func (m *Manager) register(t ToolAdapter) {
	if m.registry.Register == nil {
		return
	}
	m.registry.Register(t)
}

func (m *Manager) unregister(name string) {
	if m.registry.Unregister == nil {
		return
	}
	m.registry.Unregister(name)
}

// ToolPrefix produces the registry name for a tool exposed by a given server.
// We prefix with the server name to avoid collisions across servers and with
// the local tool catalog. The original (unprefixed) name is preserved in the
// remote tool's spec so the MCP server sees it unchanged.
func ToolPrefix(serverName, toolName string) string {
	serverSlug := slugify(serverName)
	return "mcp_" + serverSlug + "_" + toolName
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

func buildTransport(cfg ServerConfig) (Transport, error) {
	switch strings.ToLower(cfg.Transport) {
	case "stdio", "":
		return NewStdioTransport(StdioConfig{
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
		})
	case "http", "https", "sse":
		return NewHTTPTransport(cfg.URL, cfg.Headers, 0), nil
	default:
		return nil, fmt.Errorf("mcp: transport %q not supported", cfg.Transport)
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
	return ClientCapabilities{
		Roots:       &roots,
		Sampling:    &sampling,
		Elicitation: &elicitation,
	}
}

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

	default:
		return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: fmt.Sprintf("method %q not supported", method)}
	}
}

// NotifyRootsChanged sends a notifications/roots/list_changed notification to all connected servers.
func (m *Manager) NotifyRootsChanged(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.servers {
		if s.client != nil {
			_ = s.client.Send(ctx, "notifications/roots/list_changed", nil)
		}
	}
	return nil
}
