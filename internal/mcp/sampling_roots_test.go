package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestClientInboundRequest(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer serverWriter.Close()
		scanner := bufio.NewScanner(serverReader)
		if scanner.Scan() {
			var initReq RPCEnvelope
			_ = json.Unmarshal(scanner.Bytes(), &initReq)
			resp := map[string]any{
				"jsonrpc": jsonRPCVersion,
				"id":      initReq.ID.Raw(),
				"result": InitializeResult{
					ProtocolVersion: ProtocolVersion,
					Capabilities:    ServerCapabilities{},
					ServerInfo:      Implementation{Name: "fake", Version: "0"},
				},
			}
			data, _ := json.Marshal(resp)
			_, _ = serverWriter.Write(append(data, '\n'))
		}

		_ = scanner.Scan() // Ignore initialized notification

		rootsReq := map[string]any{
			"jsonrpc": jsonRPCVersion,
			"id":      "roots-req-id",
			"method":  "roots/list",
		}
		data, _ := json.Marshal(rootsReq)
		_, _ = serverWriter.Write(append(data, '\n'))

		if scanner.Scan() {
			var rootsResp RPCEnvelope
			_ = json.Unmarshal(scanner.Bytes(), &rootsResp)
			if string(rootsResp.ID.Raw()) != `"roots-req-id"` {
				fmt.Printf("unexpected response ID: %s\n", string(rootsResp.ID.Raw()))
			}
		}

		samplingReq := map[string]any{
			"jsonrpc": jsonRPCVersion,
			"id":      "sampling-req-id",
			"method":  "sampling/createMessage",
			"params": CreateMessageParams{
				Messages: []SamplingMessage{
					{Content: ContentBlock{Type: "text", Text: "hello"}, Role: "user"},
				},
				MaxTokens: 10,
			},
		}
		data, _ = json.Marshal(samplingReq)
		_, _ = serverWriter.Write(append(data, '\n'))

		if scanner.Scan() {
			var samplingResp RPCEnvelope
			_ = json.Unmarshal(scanner.Bytes(), &samplingResp)
			if string(samplingResp.ID.Raw()) != `"sampling-req-id"` {
				fmt.Printf("unexpected response ID: %s\n", string(samplingResp.ID.Raw()))
			}
		}
	}()

	transport := newPipeTransport(
		bufio.NewReader(clientReader),
		clientWriter,
		clientWriter,
	)

	onRequestCalledRoots := make(chan bool, 1)
	onRequestCalledSampling := make(chan bool, 1)

	client := NewClient(ClientConfig{
		Transport:      transport,
		ClientInfo:     Implementation{Name: "test", Version: "0"},
		RequestTimeout: 2 * time.Second,
		OnRequest: func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			switch method {
			case "roots/list":
				onRequestCalledRoots <- true
				return ListRootsResult{Roots: []Root{{URI: "file:///workspace", Name: "workspace"}}}, nil
			case "sampling/createMessage":
				onRequestCalledSampling <- true
				return CreateMessageResult{
					Content: ContentBlock{Type: "text", Text: "response text"},
					Model:   "test-model",
					Role:    "assistant",
				}, nil
			default:
				return nil, fmt.Errorf("unknown method")
			}
		},
	})
	client.Start(ctx)
	defer client.Close()

	_, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	select {
	case <-onRequestCalledRoots:
		// success
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for roots/list request handler")
	}

	select {
	case <-onRequestCalledSampling:
		// success
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for sampling/createMessage request handler")
	}
}

func TestManagerInboundRequest(t *testing.T) {
	m := NewManager(ManagerConfig{
		Timeout: 2 * time.Second,
		RootsFn: func(ctx context.Context) ([]Root, error) {
			return []Root{{URI: "file:///workspace", Name: "workspace"}}, nil
		},
		SamplingFn: func(ctx context.Context, params CreateMessageParams) (*CreateMessageResult, error) {
			return &CreateMessageResult{
				Content: ContentBlock{Type: "text", Text: "sampled"},
				Model:   "sampled-model",
				Role:    "assistant",
			}, nil
		},
	})

	ctx := context.Background()

	resRoots, err := m.handleInboundRequest(ctx, nil, "roots/list", nil)
	if err != nil {
		t.Fatalf("handle roots/list failed: %v", err)
	}
	rootsList, ok := resRoots.(ListRootsResult)
	if !ok || len(rootsList.Roots) != 1 || rootsList.Roots[0].Name != "workspace" {
		t.Errorf("unexpected roots list: %+v", resRoots)
	}

	params := CreateMessageParams{
		Messages: []SamplingMessage{
			{Content: ContentBlock{Type: "text", Text: "hi"}, Role: "user"},
		},
		MaxTokens: 5,
	}
	rawParams, _ := json.Marshal(params)
	resSampling, err := m.handleInboundRequest(ctx, nil, "sampling/createMessage", rawParams)
	if err != nil {
		t.Fatalf("handle sampling/createMessage failed: %v", err)
	}
	samplingRes, ok := resSampling.(*CreateMessageResult)
	if !ok || samplingRes.Content.Text != "sampled" {
		t.Errorf("unexpected sampling result: %+v", resSampling)
	}
}

func TestManagerElicitationLegacyInbound(t *testing.T) {
	got := make(chan ElicitRequest, 1)
	m := NewManager(ManagerConfig{
		Timeout: 2 * time.Second,
		ElicitationFn: func(_ context.Context, serverName string, req ElicitRequest) (*ElicitResult, error) {
			if serverName != "srv" {
				t.Errorf("expected server name srv, got %q", serverName)
			}
			got <- req
			return &ElicitResult{Action: "accept", Content: json.RawMessage(`{"name":"octocat"}`)}, nil
		},
	})

	raw, _ := json.Marshal(ElicitRequest{Message: "username?", RequestedSchema: json.RawMessage(`{"type":"object"}`)})
	res, err := m.handleInboundRequest(context.Background(), &managedServer{cfg: ServerConfig{Name: "srv"}}, "elicitation/create", raw)
	if err != nil {
		t.Fatalf("handle elicitation/create: %v", err)
	}
	el, ok := res.(*ElicitResult)
	if !ok || el.Action != "accept" {
		t.Fatalf("unexpected result: %+v", res)
	}
	select {
	case req := <-got:
		if req.Message != "username?" {
			t.Errorf("unexpected message: %q", req.Message)
		}
	default:
		t.Error("elicitation fn was not called")
	}
}

func TestManagerElicitationDeclinesWhenNil(t *testing.T) {
	m := NewManager(ManagerConfig{Timeout: 2 * time.Second})
	raw, _ := json.Marshal(ElicitRequest{Message: "hi"})
	res, err := m.handleInboundRequest(context.Background(), &managedServer{cfg: ServerConfig{Name: "s"}}, "elicitation/create", raw)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	el, ok := res.(ElicitResult)
	if !ok || el.Action != "decline" {
		t.Fatalf("expected decline, got %+v", res)
	}
}

func TestManagerMRTRElicitation(t *testing.T) {
	m := NewManager(ManagerConfig{
		Timeout: 2 * time.Second,
		ElicitationFn: func(_ context.Context, _ string, req ElicitRequest) (*ElicitResult, error) {
			return &ElicitResult{Action: "accept", Content: json.RawMessage(`{"name":"octocat"}`)}, nil
		},
	})
	s := &managedServer{cfg: ServerConfig{Name: "srv"}}
	raw, _ := json.Marshal(ElicitRequest{Message: "username?"})
	responses, err := m.onInputRequests(context.Background(), s, map[string]InputRequest{
		"k1": {Method: "elicitation/create", Params: raw},
	})
	if err != nil {
		t.Fatalf("onInputRequests: %v", err)
	}
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	var el ElicitResult
	if err := json.Unmarshal(responses["k1"], &el); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if el.Action != "accept" {
		t.Errorf("expected accept, got %q", el.Action)
	}
}

func TestManagerMRTRUnsupportedRequest(t *testing.T) {
	m := NewManager(ManagerConfig{Timeout: 2 * time.Second})
	s := &managedServer{cfg: ServerConfig{Name: "srv"}}
	_, err := m.onInputRequests(context.Background(), s, map[string]InputRequest{
		"k1": {Method: "something/else", Params: nil},
	})
	if err == nil {
		t.Fatal("expected error for unsupported input request")
	}
}
