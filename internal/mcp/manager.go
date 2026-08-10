package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/tracelog"
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
	// InputSchema is the raw inputSchema advertised by the server, used by
	// the host to describe the tool natively to the LLM.
	InputSchema json.RawMessage
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
	elicitationFn     ElicitationFn
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
	Capabilities  ClientCapabilities
	Registry      ToolRegistrar
	RootsFn       func(ctx context.Context) ([]Root, error)
	SamplingFn    func(ctx context.Context, params CreateMessageParams) (*CreateMessageResult, error)
	ElicitationFn ElicitationFn
	ClientInfo    Implementation
	Timeout       time.Duration
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
		registry:      cfg.Registry,
		timeout:       cfg.Timeout,
		servers:       make(map[string]*managedServer),
		rootsFn:       cfg.RootsFn,
		samplingFn:    cfg.SamplingFn,
		elicitationFn: cfg.ElicitationFn,
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

// StopServer shuts down a single server by name and unregisters its tools.
func (m *Manager) StopServer(name string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	s, ok := m.servers[name]
	if ok {
		delete(m.servers, name)
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	m.unregisterServerTools(s)
	if s.cancel != nil {
		s.cancel()
	}
	if s.client != nil {
		_ = s.client.Close()
	}
	return true
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

		transport, cleanup, err := buildTransport(s.cfg)
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
			OnInputRequests: func(ctx context.Context, requests map[string]InputRequest) (map[string]json.RawMessage, error) {
				return m.onInputRequests(ctx, s, requests)
			},
		})
		client.Start(ctx)

		if err := client.Negotiate(ctx); err != nil {
			m.markErr(s, fmt.Errorf("negotiate: %w", err))
			_ = client.Close()
			if cleanup != nil {
				cleanup()
			}
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
		if err := m.refreshTools(ctx, s); err != nil && caps.Tools != nil {
			m.markErr(s, err)
		}
		if err := m.refreshResources(ctx, s); err != nil && caps.Resources != nil {
			m.markErr(s, err)
		}
		if caps.Resources != nil {
			if err := m.refreshTemplates(ctx, s); err != nil {
				m.markErr(s, err)
			}
		}
		if err := m.refreshPrompts(ctx, s); err != nil && caps.Prompts != nil {
			m.markErr(s, err)
		}

		// In the stateless protocol, change notifications arrive on a
		// long-lived subscriptions/listen stream instead of the legacy GET
		// stream. Open it (fire-and-forget) with the notification types we
		// know how to react to.
		if client.NegotiatedProtocol() == ProtocolVersionModern {
			filter := SubscriptionFilter{
				ToolsListChanged:     true,
				PromptsListChanged:   true,
				ResourcesListChanged: true,
			}
			if err := client.OpenSubscriptionStream(ctx, filter); err != nil {
				tracelog.Logf("MCP: failed to open subscriptions/listen on %q: %v", s.cfg.Name, err)
			}
		}

		// Block until context is cancelled or client exits
		select {
		case <-ctx.Done():
			if cleanup != nil {
				cleanup()
			}
			return
		case <-client.doneCh:
			if cleanup != nil {
				cleanup()
			}
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
