package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRemoteToolRunForwardsCall(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tools := []Tool{{
		Name:        "add",
		Description: "Add two numbers",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}
	srv := runFakeServer(ctx, serverReader, serverWriter, tools, false)
	defer srv.Shutdown()

	client := NewClient(ClientConfig{
		Transport:  newPipeTransport(bufio.NewReader(clientReader), clientWriter, clientWriter),
		ClientInfo: Implementation{Name: "test", Version: "0"},
	})
	client.Start(ctx)
	defer client.Close()
	_, _ = client.Initialize(ctx)

	manager := &Manager{
		registry: ToolRegistrar{Register: func(ToolAdapter) {}, Unregister: func(string) bool { return true }},
		timeout:  2 * time.Second,
		servers:  map[string]*managedServer{},
	}
	manager.servers["fake"] = &managedServer{cfg: ServerConfig{Name: "fake"}, client: client}

	adapter := NewRemoteToolAdapter("fake", "mcp_fake_add", tools[0], manager)
	out, err := adapter.Run(ctx, `{"a":1}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.Output == "" {
		t.Fatalf("expected output, got empty")
	}
	if out.Summary == "" {
		t.Fatalf("expected summary, got empty")
	}
	if !contains([]byte(out.Output), "called add") {
		t.Errorf("output missing 'called add': %q", out.Output)
	}
}

func TestRemoteToolAcceptsBareStringArgs(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tools := []Tool{{Name: "echo", InputSchema: json.RawMessage(`{}`)}}
	srv := runFakeServer(ctx, serverReader, serverWriter, tools, false)
	defer srv.Shutdown()

	client := NewClient(ClientConfig{
		Transport:  newPipeTransport(bufio.NewReader(clientReader), clientWriter, clientWriter),
		ClientInfo: Implementation{Name: "t"},
	})
	client.Start(ctx)
	defer client.Close()
	_, _ = client.Initialize(ctx)

	manager := &Manager{
		registry: ToolRegistrar{Register: func(ToolAdapter) {}, Unregister: func(string) bool { return true }},
		timeout:  2 * time.Second,
		servers:  map[string]*managedServer{},
	}
	manager.servers["s"] = &managedServer{cfg: ServerConfig{Name: "s"}, client: client}

	adapter := NewRemoteToolAdapter("s", "mcp_s_echo", tools[0], manager)
	if _, err := adapter.Run(ctx, "hi"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestManagerRegistersAndUnregisters(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tools := []Tool{
		{Name: "alpha", InputSchema: json.RawMessage(`{}`)},
		{Name: "beta", InputSchema: json.RawMessage(`{}`), Annotations: &ToolAnnotations{ReadOnlyHint: true}},
	}
	srv := runFakeServer(ctx, serverReader, serverWriter, tools, false)
	defer srv.Shutdown()

	transport := newPipeTransport(bufio.NewReader(clientReader), clientWriter, clientWriter)
	client := NewClient(ClientConfig{
		Transport:  transport,
		ClientInfo: Implementation{Name: "t"},
	})
	client.Start(ctx)
	defer client.Close()
	_, _ = client.Initialize(ctx)

	var (
		mu    sync.Mutex
		regs  []ToolAdapter
		unreg []string
	)
	registrar := ToolRegistrar{
		Register: func(t ToolAdapter) { mu.Lock(); regs = append(regs, t); mu.Unlock() },
		Unregister: func(name string) bool {
			mu.Lock()
			unreg = append(unreg, name)
			mu.Unlock()
			return true
		},
	}
	m := NewManager(ManagerConfig{Registry: registrar, Timeout: 2 * time.Second})

	// Manually replicate what startOne + runServer would do but synchronously.
	entry := &managedServer{
		cfg:    ServerConfig{Name: "fake", Transport: "stdio"},
		client: client,
		tools:  map[string]bool{},
	}
	m.mu.Lock()
	m.servers["fake"] = entry
	m.mu.Unlock()

	if err := m.refreshTools(ctx, entry); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	mu.Lock()
	if len(regs) != 2 {
		t.Fatalf("expected 2 registered tools, got %d", len(regs))
	}
	mu.Unlock()

	// simulate list_changed with one fewer tool.
	tools = tools[:1]
	srv.tools = tools
	tools[0].Name = "alpha"
	if err := m.refreshTools(ctx, entry); err != nil {
		t.Fatalf("refresh 2: %v", err)
	}
	mu.Lock()
	if len(unreg) != 1 {
		t.Fatalf("expected 1 unregister, got %d", len(unreg))
	}
	if unreg[0] != ToolPrefix("fake", "beta") {
		t.Errorf("unexpected unregister name: %s", unreg[0])
	}
	mu.Unlock()

	// stopping should unregister the remaining one.
	m.unregisterServerTools(entry)
	mu.Lock()
	if len(unreg) != 2 {
		t.Fatalf("expected 2 unregisters after stop, got %d", len(unreg))
	}
	mu.Unlock()
}

func TestToolPrefixSlugify(t *testing.T) {
	cases := map[string]string{
		"my server":    "mcp_my_server_",
		"My-Server/2":  "mcp_my_server_2_",
		"Alpha   Beta": "mcp_alpha_beta_",
	}
	for in, prefix := range cases {
		if got := ToolPrefix(in, "x"); got != prefix+"x" {
			t.Errorf("ToolPrefix(%q) = %s, want %s", in, got, prefix+"x")
		}
	}
}

func TestToolPrefixSanitizesToolName(t *testing.T) {
	cases := []struct{ server, tool, want string }{
		{"srv", "get_user", "mcp_srv_get_user"},
		{"srv", "my tool", "mcp_srv_my_tool"},
		{"srv", "weird,name!", "mcp_srv_weird_name_"},
		{"srv", "UPPER.Case-Name", "mcp_srv_UPPER.Case-Name"}, // case preserved per spec
	}
	for _, tc := range cases {
		if got := ToolPrefix(tc.server, tc.tool); got != tc.want {
			t.Errorf("ToolPrefix(%q, %q) = %q, want %q", tc.server, tc.tool, got, tc.want)
		}
	}
}

func TestSanitizeToolNameEdgeCases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "tool"},
		{"!!!", "tool"},
		{".", "tool"},
		{"valid_name-1.2", "valid_name-1.2"},
		{"a b", "a_b"},
		// long names are truncated to 128 chars
		{strings.Repeat("x", 200), strings.Repeat("x", 128)},
	}
	for _, tc := range cases {
		if got := sanitizeToolName(tc.in); got != tc.want {
			t.Errorf("sanitizeToolName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeToolNameNeverStartsWithSeparator(t *testing.T) {
	for _, in := range []string{".hidden", "-dash", "_under", "..double"} {
		got := sanitizeToolName(in)
		if strings.HasPrefix(got, ".") || strings.HasPrefix(got, "-") || strings.HasPrefix(got, "_") {
			t.Errorf("sanitizeToolName(%q) = %q must not start with a separator", in, got)
		}
	}
}

func TestRemoteToolHandlesServerError(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Provide no tools in the fake server; the manager will not register
	// any. But we want to exercise the case where the server returns an
	// application-level error from a call. Easiest path: hit a method that
	// returns an RPC error (e.g. unknown method) and verify we surface it.
	srv := runFakeServer(ctx, serverReader, serverWriter, nil, false)
	defer srv.Shutdown()

	client := NewClient(ClientConfig{
		Transport:  newPipeTransport(bufio.NewReader(clientReader), clientWriter, clientWriter),
		ClientInfo: Implementation{Name: "t"},
	})
	client.Start(ctx)
	defer client.Close()
	_, _ = client.Initialize(ctx)
	err := client.Request(ctx, "unknown", nil, nil)
	if err == nil {
		t.Fatalf("expected rpc error")
	}
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *RPCError, got %T", err)
	}
}
