package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// streamableTestServer is a minimal in-memory MCP server that speaks the
// Streamable HTTP transport: a single endpoint that handles POST and GET.
type streamableTestServer struct {
	postHandler    func(env RPCEnvelope) (any, *RPCError)
	sessionID      string
	pendingSSE     []string
	getConnections int
	mu             sync.Mutex
	initialized    bool
	closed         bool
}

func newStreamableTestServer() *streamableTestServer {
	return &streamableTestServer{
		sessionID:   "test-session-123",
		postHandler: defaultStreamableHandler,
	}
}

func defaultStreamableHandler(env RPCEnvelope) (any, *RPCError) {
	switch env.Method {
	case "initialize":
		return InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities: ServerCapabilities{
				Tools: &struct {
					ListChanged bool `json:"listChanged,omitempty"`
				}{ListChanged: true},
			},
			ServerInfo: Implementation{Name: "streamable-test", Version: "0.0.1"},
		}, nil
	case "tools/list":
		return ListToolsResult{Tools: []Tool{
			{Name: "echo", Description: "echoes input"},
		}}, nil
	case "ping":
		return map[string]any{}, nil
	}
	// Notifications and unknown methods: return (nil, nil) so the server
	// replies 202 Accepted (notification) or generic empty body.
	if env.IsNotification() {
		return nil, nil
	}
	return nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "no handler"}
}

func (s *streamableTestServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.serve)
	return mux
}

func (s *streamableTestServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		http.Error(w, "closed", http.StatusServiceUnavailable)
		return
	}
	s.mu.Unlock()

	switch r.Method {
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodGet:
		s.handleGet(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *streamableTestServer) handlePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	env, err := DecodeMessage(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// First request must be initialize; from then on require the session id.
	s.mu.Lock()
	if env.Method == "initialize" {
		s.initialized = true
	}
	if s.initialized && env.Method != "initialize" {
		got := r.Header.Get("Mcp-Session-Id")
		if got != s.sessionID {
			s.mu.Unlock()
			http.Error(w, "missing session", http.StatusBadRequest)
			return
		}
	}
	s.mu.Unlock()

	result, errObj := s.postHandler(env)
	idRaw := json.RawMessage("null")
	if env.ID != nil {
		idRaw = env.ID.Raw()
	}
	if errObj != nil {
		raw, _ := json.Marshal(map[string]any{
			jsonRPCField: jsonRPCVersion,
			"id":         idRaw,
			"error":      errObj,
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
		return
	}
	if result == nil {
		// Notification: 202 with no body.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	raw, _ := json.Marshal(result)
	resp := map[string]any{
		jsonRPCField: jsonRPCVersion,
		"id":         idRaw,
		"result":     json.RawMessage(raw),
	}
	data, _ := json.Marshal(resp)

	w.Header().Set("Content-Type", "application/json")
	if env.Method == "initialize" {
		w.Header().Set("Mcp-Session-Id", s.sessionID)
		w.Header().Set("MCP-Protocol-Version", ProtocolVersion)
	}
	_, _ = w.Write(data)
}

func (s *streamableTestServer) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") != "text/event-stream" {
		http.Error(w, "accept required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if r.Header.Get("Mcp-Session-Id") != s.sessionID {
		s.mu.Unlock()
		http.Error(w, "missing session", http.StatusBadRequest)
		return
	}
	s.getConnections++
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
		s.mu.Lock()
		if len(s.pendingSSE) > 0 {
			msg := s.pendingSSE[0]
			s.pendingSSE = s.pendingSSE[1:]
			s.mu.Unlock()
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			if flusher != nil {
				flusher.Flush()
			}
			continue
		}
		s.mu.Unlock()
		_, _ = fmt.Fprintf(w, ": keepalive\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (s *streamableTestServer) push(msg string) {
	s.mu.Lock()
	s.pendingSSE = append(s.pendingSSE, msg)
	s.mu.Unlock()
}

func (s *streamableTestServer) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func TestStreamableInitialize(t *testing.T) {
	srv := newStreamableTestServer()
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	transport := NewStreamableTransport(StreamableConfig{
		Endpoint: ts.URL + "/mcp",
	})
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewClient(ClientConfig{
		Transport:      transport,
		ClientInfo:     Implementation{Name: "t"},
		RequestTimeout: 5 * time.Second,
	})
	client.Start(ctx)

	result, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if result.ServerInfo.Name != "streamable-test" {
		t.Errorf("unexpected server info: %+v", result.ServerInfo)
	}

	tools, err := client.ListAllTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Errorf("unexpected tools: %+v", tools)
	}

	// Confirm the transport captured the session id and is sending it back.
	transport.mu.Lock()
	sessionID := transport.sessionID
	protocol := transport.protocol
	transport.mu.Unlock()
	if sessionID != "test-session-123" {
		t.Errorf("expected session id to be captured, got %q", sessionID)
	}
	if protocol != ProtocolVersion {
		t.Errorf("expected protocol to be captured, got %q", protocol)
	}
}

func TestStreamableNotificationAccepted(t *testing.T) {
	srv := newStreamableTestServer()
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	transport := NewStreamableTransport(StreamableConfig{Endpoint: ts.URL + "/mcp"})
	defer transport.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewClient(ClientConfig{Transport: transport, ClientInfo: Implementation{Name: "t"}})
	client.Start(ctx)
	_, _ = client.Initialize(ctx)

	// Send a notification (no id). The spec says the server returns 202.
	if err := client.Send(ctx, "notifications/ping", nil); err != nil {
		t.Fatalf("send notification: %v", err)
	}
}

func TestStreamableRecvFromGetStream(t *testing.T) {
	srv := newStreamableTestServer()
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	transport := NewStreamableTransport(StreamableConfig{Endpoint: ts.URL + "/mcp"})
	defer transport.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The client's readLoop will see the pushed notification. Use the
	// OnNotification callback to assert it arrived.
	gotNotification := make(chan string, 1)
	client := NewClient(ClientConfig{
		Transport:  transport,
		ClientInfo: Implementation{Name: "t"},
		OnNotification: func(method string, _ json.RawMessage) {
			select {
			case gotNotification <- method:
			default:
			}
		},
	})
	client.Start(ctx)
	_, _ = client.Initialize(ctx)

	// Give the GET stream time to open on the first Recv.
	time.Sleep(100 * time.Millisecond)

	// Push a server-initiated notification through the GET stream.
	push := `{"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info","data":"hello"}}`
	srv.push(push)

	select {
	case method := <-gotNotification:
		if method != "notifications/message" {
			t.Errorf("expected notifications/message, got %q", method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server-initiated notification did not arrive through the GET stream")
	}

	// The server should have accepted the GET stream connection.
	srv.mu.Lock()
	conns := srv.getConnections
	srv.mu.Unlock()
	if conns == 0 {
		t.Errorf("expected the test server to receive the GET stream request, got %d", conns)
	}
}

func TestStreamableCloseUnblocksRecv(t *testing.T) {
	srv := newStreamableTestServer()
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	transport := NewStreamableTransport(StreamableConfig{Endpoint: ts.URL + "/mcp"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewClient(ClientConfig{Transport: transport, ClientInfo: Implementation{Name: "t"}})
	client.Start(ctx)
	_, _ = client.Initialize(ctx)

	// Give the GET stream time to open.
	time.Sleep(100 * time.Millisecond)

	// Close the transport; Recv should return ErrTransportClosed quickly.
	_ = transport.Close()
	start := time.Now()
	_, err := transport.Recv(ctx)
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Errorf("Recv took too long after close: %s", elapsed)
	}
	if err == nil {
		t.Fatal("expected error after close")
	}
}

// TestStreamableModernModeHeaders verifies that on a modern (stateless)
// connection the transport sends Mcp-Method / Mcp-Name headers and no
// Mcp-Session-Id, per spec 2026-07-28.
func TestStreamableModernModeHeaders(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotName, gotSession, gotVersion string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Header.Get("Mcp-Method")
		gotName = r.Header.Get("Mcp-Name")
		gotSession = r.Header.Get("Mcp-Session-Id")
		gotVersion = r.Header.Get("MCP-Protocol-Version")
		mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		env, err := DecodeMessage(body)
		if err != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		var result any
		switch env.Method {
		case "tools/call":
			result = CallToolResult{Content: []ContentBlock{{Type: "text", Text: "ok"}}}
		default:
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		resp := map[string]any{
			jsonRPCField: jsonRPCVersion,
			"id":         env.ID.Raw(),
			"result":     result,
		}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	transport := NewStreamableTransport(StreamableConfig{Endpoint: srv.URL})
	defer transport.Close()

	// Simulate a modern negotiated connection.
	transport.SetProtocol(ProtocolVersionModern)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewClient(ClientConfig{
		Transport:      transport,
		ClientInfo:     Implementation{Name: "t"},
		RequestTimeout: 5 * time.Second,
	})
	client.Start(ctx)

	var result CallToolResult
	if err := client.Request(ctx, "tools/call", CallToolParams{Name: "echo", Arguments: json.RawMessage(`{}`)}, &result); err != nil {
		t.Fatalf("call: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != "tools/call" {
		t.Errorf("expected Mcp-Method tools/call, got %q", gotMethod)
	}
	if gotName != "echo" {
		t.Errorf("expected Mcp-Name echo, got %q", gotName)
	}
	if gotSession != "" {
		t.Errorf("expected no Mcp-Session-Id in modern mode, got %q", gotSession)
	}
	if gotVersion != ProtocolVersionModern {
		t.Errorf("expected MCP-Protocol-Version %s, got %q", ProtocolVersionModern, gotVersion)
	}
}

// TestStreamableLegacyModeSendsSession verifies the legacy path still sends
// the session id header when one was captured.
func TestStreamableLegacyModeSendsSession(t *testing.T) {
	var mu sync.Mutex
	var gotSession string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotSession = r.Header.Get("Mcp-Session-Id")
		mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		env, err := DecodeMessage(body)
		if err != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		resp := map[string]any{
			jsonRPCField: jsonRPCVersion,
			"id":         env.ID.Raw(),
			"result":     map[string]any{},
		}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	transport := NewStreamableTransport(StreamableConfig{Endpoint: srv.URL})
	defer transport.Close()

	// Legacy connection: session captured from an earlier response.
	transport.SetSession("legacy-session-9")
	transport.SetProtocol("2025-11-25")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := NewClient(ClientConfig{Transport: transport, ClientInfo: Implementation{Name: "t"}})
	client.Start(ctx)
	if err := client.Request(ctx, "ping", nil, nil); err != nil {
		t.Fatalf("ping: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotSession != "legacy-session-9" {
		t.Errorf("expected legacy session header, got %q", gotSession)
	}
}

// TestEncodeHeaderValue covers the base64 sentinel encoding rules.
func TestEncodeHeaderValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"us-west1", "us-west1"},
		{"Hello, 世界", "=?base64?SGVsbG8sIOS4lueVjA==?="},
		{"=?base64?literal?=", "=?base64?PT9iYXNlNjQ/bGl0ZXJhbD89?="},
		{"padded ", "=?base64?cGFkZGVkIA==?="},
	}
	for _, tc := range cases {
		if got := encodeHeaderValue(tc.in); got != tc.want {
			t.Errorf("encodeHeaderValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
