package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

// pipeTransport is an in-memory transport backed by an io.Pipe. Both
// endpoints share the same pipeTransport; the pair transports bytes in each
// direction. We use it to drive the Client/Server protocol without spawning
// real subprocesses.
type pipeTransport struct {
	reader *bufio.Reader
	writer io.WriteCloser
	closer io.Closer
}

func newPipeTransport(r *bufio.Reader, w io.WriteCloser, c io.Closer) *pipeTransport {
	return &pipeTransport{reader: r, writer: w, closer: c}
}

func (p *pipeTransport) Send(_ context.Context, payload []byte) error {
	if _, err := p.writer.Write(payload); err != nil {
		return err
	}
	if _, err := p.writer.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}

func (p *pipeTransport) Recv(ctx context.Context) ([]byte, error) {
	done := make(chan struct {
		err  error
		line []byte
	}, 1)
	go func() {
		line, err := p.reader.ReadBytes('\n')
		done <- struct {
			err  error
			line []byte
		}{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		if r.err != nil {
			if errors.Is(r.err, io.EOF) {
				return nil, io.EOF
			}
			return nil, r.err
		}
		out := r.line
		for len(out) > 0 && (out[len(out)-1] == '\n' || out[len(out)-1] == '\r') {
			out = out[:len(out)-1]
		}
		return out, nil
	}
}

func (p *pipeTransport) Close() error { return p.closer.Close() }

// fakeServer is a tiny MCP server: it reads requests line-by-line and emits
// canned responses. It lives in a goroutine and is closed via context
// cancellation.
type fakeServer struct {
	serverWriter io.Closer
	cancel       context.CancelFunc
	done         chan struct{}
	tools        []Tool
	mu           sync.Mutex
	closed       bool
	pingErr      bool
}

func (f *fakeServer) Shutdown() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	if f.serverWriter != nil {
		_ = f.serverWriter.Close()
	}
	f.cancel()
	<-f.done
	f.mu.Unlock()
}

func runFakeServer(ctx context.Context, serverReader io.Reader, serverWriter io.WriteCloser, tools []Tool, pingErr bool) *fakeServer {
	ctx, cancel := context.WithCancel(ctx)
	srv := &fakeServer{
		cancel:       cancel,
		done:         make(chan struct{}),
		tools:        tools,
		pingErr:      pingErr,
		serverWriter: serverWriter,
	}
	go func() {
		defer func() {
			if c, ok := serverWriter.(io.Closer); ok {
				_ = c.Close()
			}
			close(srv.done)
		}()
		reader := bufio.NewReader(serverReader)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line, err := reader.ReadBytes('\n')
			if err != nil {
				return
			}
			line = trimNewline(line)
			if len(line) == 0 {
				continue
			}
			var env RPCEnvelope
			if err := json.Unmarshal(line, &env); err != nil {
				continue
			}
			switch {
			case env.IsRequest():
				srv.handleRequest(serverWriter, env)
			case env.IsNotification():
				// notifications are ignored by the fake server.
			}
		}
	}()
	return srv
}

func (f *fakeServer) handleRequest(w io.Writer, env RPCEnvelope) {
	id := env.ID.Raw()
	var (
		result any
		errObj *RPCError
	)
	switch env.Method {
	case "initialize":
		result = InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities: ServerCapabilities{
				Tools: &struct {
					ListChanged bool `json:"listChanged,omitempty"`
				}{ListChanged: true},
				Resources: &struct {
					Subscribe   bool `json:"subscribe,omitempty"`
					ListChanged bool `json:"listChanged,omitempty"`
				}{Subscribe: true, ListChanged: true},
				Prompts: &struct {
					ListChanged bool `json:"listChanged,omitempty"`
				}{ListChanged: true},
			},
			ServerInfo: Implementation{Name: "fake", Version: "0.0.1"},
		}
	case "tools/list":
		result = ListToolsResult{Tools: f.tools}
	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(env.Params, &params); err != nil {
			errObj = &RPCError{Code: ErrCodeInvalidParams, Message: err.Error()}
		} else {
			result = CallToolResult{Content: []ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("called %s", params.Name),
			}}}
		}
	case "resources/list":
		result = ListResourcesResult{
			Resources: []Resource{
				{URI: "file:///test.txt", Name: "test.txt", MimeType: "text/plain"},
			},
		}
	case "resources/templates/list":
		result = ListResourceTemplatesResult{
			ResourceTemplates: []ResourceTemplate{
				{URITemplate: "file:///{path}", Name: "Project Files", MimeType: "text/plain"},
			},
		}
	case "resources/read":
		result = ReadResourceResult{
			Contents: []ResourceContent{
				{URI: "file:///test.txt", MimeType: "text/plain", Text: "Hello, resources!"},
			},
		}
	case "resources/subscribe", "resources/unsubscribe":
		result = map[string]any{}
	case "prompts/list":
		result = ListPromptsResult{
			Prompts: []Prompt{
				{Name: "test-prompt", Description: "A test prompt template"},
			},
		}
	case "prompts/get":
		result = GetPromptResult{
			Description: "Test prompt content",
			Messages: []PromptMessage{
				{
					Role: "user",
					Content: ContentBlock{
						Type: "text",
						Text: "Hello from test prompt!",
					},
				},
			},
		}
	case "ping":
		if f.pingErr {
			errObj = &RPCError{Code: ErrCodeInternalError, Message: "no pings"}
		} else {
			result = map[string]any{}
		}
	default:
		errObj = &RPCError{Code: ErrCodeMethodNotFound, Message: "no"}
	}
	resp := map[string]any{"jsonrpc": jsonRPCVersion, "id": json.RawMessage(id)}
	if errObj != nil {
		resp["error"] = errObj
	} else {
		raw, _ := json.Marshal(result)
		resp["result"] = json.RawMessage(raw)
	}
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = w.Write(data)
}

func TestClientInitializeAndToolsList(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tools := []Tool{
		{
			Name:        "echo",
			Description: "Echoes its input",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
		},
		{
			Name:        "now",
			Description: "Returns a timestamp",
			Annotations: &ToolAnnotations{ReadOnlyHint: true},
		},
	}

	srv := runFakeServer(ctx, serverReader, serverWriter, tools, false)
	defer srv.Shutdown()

	transport := newPipeTransport(
		bufio.NewReader(clientReader),
		clientWriter,
		clientWriter,
	)

	client := NewClient(ClientConfig{
		Transport:  transport,
		ClientInfo: Implementation{Name: "test", Version: "0"},
		Capabilities: ClientCapabilities{
			Roots: &struct {
				ListChanged bool `json:"listChanged,omitempty"`
			}{ListChanged: true},
		},
		RequestTimeout: 2 * time.Second,
	})
	client.Start(ctx)

	init, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if init.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocol version mismatch: %s", init.ProtocolVersion)
	}

	all, err := client.ListAllTools(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(all))
	}

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	result, err := client.CallTool(ctx, "echo", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "called echo" {
		t.Fatalf("unexpected call result: %+v", result)
	}

	if err := client.Close(); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("close: %v", err)
	}
}

func TestClientCallToolError(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := runFakeServer(ctx, serverReader, serverWriter, nil, false)
	defer srv.Shutdown()

	transport := newPipeTransport(
		bufio.NewReader(clientReader),
		clientWriter,
		clientWriter,
	)
	client := NewClient(ClientConfig{Transport: transport, ClientInfo: Implementation{Name: "t"}})
	client.Start(ctx)
	_, _ = client.Initialize(ctx)

	// The fake server replies with method-not-found for unknown methods.
	err := client.Request(ctx, "nope", nil, nil)
	if err == nil {
		t.Fatalf("expected error from unknown method")
	}
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *RPCError, got %T (%v)", err, err)
	}
	if rpcErr.Code != ErrCodeMethodNotFound {
		t.Errorf("expected code -32601, got %d", rpcErr.Code)
	}
	_ = client.Close()
}

func TestClientPingError(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := runFakeServer(ctx, serverReader, serverWriter, nil, true)
	defer srv.Shutdown()

	transport := newPipeTransport(
		bufio.NewReader(clientReader),
		clientWriter,
		clientWriter,
	)
	client := NewClient(ClientConfig{Transport: transport, ClientInfo: Implementation{Name: "t"}})
	client.Start(ctx)
	_, _ = client.Initialize(ctx)
	if err := client.Ping(ctx); err == nil {
		t.Fatalf("expected error from ping")
	}
	_ = client.Close()
}
