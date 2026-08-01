package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

// modernFakeServer speaks the stateless 2026-07-28 protocol: it answers
// server/discover and tools/list without any initialize handshake.
type modernFakeServer struct {
	writer      io.WriteCloser
	cancel      context.CancelFunc
	done        chan struct{}
	seenMeta    []map[string]any
	supportsVer []string
	mu          sync.Mutex
}

func runModernFakeServer(reader io.Reader, writer io.WriteCloser, supportsVer []string) *modernFakeServer {
	_, cancel := context.WithCancel(context.Background())
	s := &modernFakeServer{
		writer:      writer,
		cancel:      cancel,
		done:        make(chan struct{}),
		supportsVer: supportsVer,
	}
	go func() {
		defer close(s.done)
		defer func() {
			if c, ok := writer.(io.Closer); ok {
				_ = c.Close()
			}
		}()
		br := bufio.NewReader(reader)
		for {
			line, err := br.ReadBytes('\n')
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
			s.handle(env)
		}
	}()
	return s
}

func (s *modernFakeServer) handle(env RPCEnvelope) {
	if env.Method != "" && env.IsRequest() {
		// Record the top-level _meta metadata (stateless protocol).
		if len(env.Meta) > 0 {
			var meta map[string]any
			_ = json.Unmarshal(env.Meta, &meta)
			s.mu.Lock()
			s.seenMeta = append(s.seenMeta, meta)
			s.mu.Unlock()
		}
	}

	var result any
	switch env.Method {
	case "server/discover":
		result = DiscoverResult{
			ProtocolVersions: s.supportsVer,
			ServerInfo:       Implementation{Name: "modern", Version: "1.0"},
		}
	case "tools/list":
		result = ListToolsResult{Tools: []Tool{
			{Name: "echo", Description: "echo"},
		}}
	case "ping", "notifications/initialized":
		// modern servers have no ping; fall through to empty
		result = map[string]any{}
	default:
		// Reply with an error so the client treats this as unsupported.
		s.write(env, nil, &RPCError{Code: ErrCodeMethodNotFound, Message: "no"})
		return
	}
	s.write(env, result, nil)
}

func (s *modernFakeServer) write(env RPCEnvelope, result any, errObj *RPCError) {
	id := "null"
	if env.ID != nil {
		id = string(env.ID.Raw())
	}
	resp := map[string]any{jsonRPCField: jsonRPCVersion, "id": json.RawMessage(id)}
	if errObj != nil {
		resp["error"] = errObj
	} else {
		raw, _ := json.Marshal(result)
		resp["result"] = json.RawMessage(raw)
	}
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = s.writer.Write(data)
}

func (s *modernFakeServer) Shutdown() {
	s.cancel()
	_ = s.writer.Close()
	// Give the client's read loop a moment to observe EOF.
	select {
	case <-s.done:
	case <-time.After(200 * time.Millisecond):
	}
}

func newPipeClientTransport(t *testing.T, serverWriter io.WriteCloser, clientWriter io.WriteCloser, clientReader io.Reader) (*pipeTransport, *Client, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	transport := newPipeTransport(
		bufio.NewReader(clientReader),
		clientWriter,
		clientWriter,
	)
	client := NewClient(ClientConfig{
		Transport:      transport,
		ClientInfo:     Implementation{Name: "test", Version: "0"},
		RequestTimeout: 5 * time.Second,
	})
	client.Start(ctx)
	return transport, client, cancel
}

// closeClient closes the client, which closes its write end of the pipe so
// the fake server's read loop unblocks and can be shut down cleanly.
func closeClient(c *Client) {
	if c != nil {
		_ = c.Close()
	}
}

func TestNegotiateModernServer(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	_, client, cancel := newPipeClientTransport(t, serverWriter, clientWriter, clientReader)
	defer cancel()

	srv := runModernFakeServer(serverReader, serverWriter, []string{"2026-07-28", "2025-11-25"})
	defer srv.Shutdown()
	defer closeClient(client)

	if err := client.Negotiate(context.Background()); err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	if got := client.NegotiatedProtocol(); got != "2026-07-28" {
		t.Errorf("expected 2026-07-28, got %q", got)
	}

	// Subsequent requests carry _meta.
	all, err := client.ListAllTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(all) != 1 || all[0].Name != "echo" {
		t.Errorf("unexpected tools: %+v", all)
	}

	srv.mu.Lock()
	metas := srv.seenMeta
	srv.mu.Unlock()
	if len(metas) == 0 {
		t.Fatal("expected _meta on requests")
	}
	last := metas[len(metas)-1]
	if last[metaProtocolVersion] != "2026-07-28" {
		t.Errorf("expected protocol version in _meta, got %v", last[metaProtocolVersion])
	}
	info, ok := last[metaClientInfo].(map[string]any)
	if !ok || info["name"] != "test" {
		t.Errorf("expected clientInfo in _meta, got %v", last[metaClientInfo])
	}
}

func TestNegotiatePrefersHighestMutual(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	_, client, cancel := newPipeClientTransport(t, serverWriter, clientWriter, clientReader)
	defer cancel()

	srv := runModernFakeServer(serverReader, serverWriter, []string{"2025-11-25"})
	defer srv.Shutdown()
	defer closeClient(client)

	if err := client.Negotiate(context.Background()); err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	if got := client.NegotiatedProtocol(); got != "2025-11-25" {
		t.Errorf("expected 2025-11-25, got %q", got)
	}
}

func TestNegotiateFallsBackToInitialize(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	// Legacy fake server: answers initialize, replies MethodNotFound to
	// server/discover.
	_, client, cancel := newPipeClientTransport(t, serverWriter, clientWriter, clientReader)
	defer cancel()

	tools := []Tool{{Name: "legacy_tool"}}
	srv := runFakeServer(context.Background(), serverReader, serverWriter, tools, false)
	defer srv.Shutdown()
	defer closeClient(client)

	if err := client.Negotiate(context.Background()); err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	if got := client.NegotiatedProtocol(); got != ProtocolVersion {
		t.Errorf("expected legacy %s, got %q", ProtocolVersion, got)
	}
	all, err := client.ListAllTools(context.Background())
	if err != nil {
		t.Fatalf("list tools after fallback: %v", err)
	}
	if len(all) != 1 || all[0].Name != "legacy_tool" {
		t.Errorf("unexpected tools: %+v", all)
	}
}

func TestNegotiateNoMutualVersion(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	_, client, cancel := newPipeClientTransport(t, serverWriter, clientWriter, clientReader)
	defer cancel()

	srv := runModernFakeServer(serverReader, serverWriter, []string{"2099-01-01"})
	defer srv.Shutdown()
	defer closeClient(client)

	err := client.Negotiate(context.Background())
	if err == nil {
		t.Fatal("expected error for no mutual version")
	}
}

func TestHighestMutualVersion(t *testing.T) {
	cases := []struct {
		want   string
		server []string
		ok     bool
	}{
		{want: "2026-07-28", server: []string{"2026-07-28"}, ok: true},
		{want: "2025-11-25", server: []string{"2025-11-25"}, ok: true},
		{server: []string{"2025-06-18"}},
		{want: "2026-07-28", server: []string{"2025-06-18", "2025-11-25", "2026-07-28"}, ok: true},
		{server: nil},
	}
	for _, tc := range cases {
		got, ok := highestMutualVersion(tc.server)
		if got != tc.want || ok != tc.ok {
			t.Errorf("highestMutualVersion(%v) = (%q, %v), want (%q, %v)",
				tc.server, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSortedVersions(t *testing.T) {
	got := sortedVersions([]string{"b", "a", "c"})
	if fmt.Sprint(got) != "[a b c]" {
		t.Errorf("expected sorted, got %v", got)
	}
}
