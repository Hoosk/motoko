package provider

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestMergeChatToolCallDeltasBuildsPendingCall(t *testing.T) {
	acc := map[int]*chatCompletionToolCall{}
	mergeChatToolCallDeltas(acc, []chatCompletionToolCallDelta{{Index: 0, ID: "call_1", Type: "function", Function: chatCompletionToolFunction{Name: "bash", Arguments: `{"input":"ls`}}}, nil)
	mergeChatToolCallDeltas(acc, []chatCompletionToolCallDelta{{Index: 0, Function: chatCompletionToolFunction{Arguments: ` -F"}`}}}, nil)
	resp := responseFromChatCompletion(chatCompletionResponse{Choices: []chatCompletionChoice{{Message: chatCompletionMessage{ToolCalls: sortedChatToolCalls(acc)}}}})
	if len(resp.PendingCalls) != 1 || resp.PendingCalls[0].Name != "bash" || resp.PendingCalls[0].Input != "ls -F" {
		t.Fatalf("unexpected streamed tool response %#v", resp)
	}
	if len(resp.OutputItems) != 1 {
		t.Fatalf("expected assistant tool call item preserved, got %#v", resp.OutputItems)
	}
}

func TestPostJSONStreamParsesSSEPayloads(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"message\":\"ho\"}\n\ndata: {\"message\":\"la\"}\n\ndata: [DONE]\n\n"
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       ioNopCloser{bytes.NewBufferString(body)},
		}, nil
	})}
	var events []string
	err := postJSONStream(context.Background(), client, "http://example.test", map[string]string{"hello": "world"}, nil, func(data string) error {
		events = append(events, data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 stream events, got %#v", events)
	}
}

func TestPostJSONStreamReturnsProviderErrorBody(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 400,
			Header:     make(http.Header),
			Body:       ioNopCloser{bytes.NewBufferString("bad request")},
		}, nil
	})}
	err := postJSONStream(context.Background(), client, "http://example.test", map[string]string{}, nil, func(data string) error { return nil })
	if err == nil || err.Error() != "provider error 400: bad request" {
		t.Fatalf("unexpected stream error %v", err)
	}
}

func TestMergeChatToolCallDeltasRetainsThoughtSignature(t *testing.T) {
	acc := map[int]*chatCompletionToolCall{}

	delta1 := chatCompletionToolCallDelta{
		Index: 0,
		ID:    "call_123",
		Type:  "function",
		Raw:   []byte(`{"index":0,"id":"call_123","type":"function","thought_signature":"sig_xyz_123","extra_content":{"google":{"signature":"foo"}}}`),
	}
	delta2 := chatCompletionToolCallDelta{
		Index: 0,
		Raw:   []byte(`{"index":0,"function":{"name":"read","arguments":"{\"in"}}`),
	}
	delta3 := chatCompletionToolCallDelta{
		Index: 0,
		Raw:   []byte(`{"index":0,"function":{"arguments":"put\":\"test.txt\"}"}}`),
	}

	mergeChatToolCallDeltas(acc, []chatCompletionToolCallDelta{delta1}, nil)
	mergeChatToolCallDeltas(acc, []chatCompletionToolCallDelta{delta2}, nil)
	mergeChatToolCallDeltas(acc, []chatCompletionToolCallDelta{delta3}, nil)

	calls := sortedChatToolCalls(acc)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	rawJSON := string(calls[0].Raw)
	if !strings.Contains(rawJSON, "sig_xyz_123") {
		t.Errorf("expected Raw JSON to contain thought_signature, got: %s", rawJSON)
	}
	if !strings.Contains(rawJSON, "extra_content") || !strings.Contains(rawJSON, "foo") {
		t.Errorf("expected Raw JSON to contain nested extra_content, got: %s", rawJSON)
	}
	if !strings.Contains(rawJSON, `"name":"read"`) {
		t.Errorf("expected Raw JSON to contain function name, got: %s", rawJSON)
	}
	if !strings.Contains(rawJSON, `"arguments":"{\\\"input\\\":\\\"test.txt\\\"}"`) && !strings.Contains(rawJSON, `"arguments":"{\"input\":\"test.txt\"}"`) {
		t.Errorf("expected Raw JSON to contain merged function arguments, got: %s", rawJSON)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type ioNopCloser struct{ *bytes.Buffer }

func (ioNopCloser) Close() error { return nil }

func TestAnthropicStreamPreservesOriginalBlockOrder(t *testing.T) {
	sseData := "event: message_start\n" +
		"data: {\"type\": \"message_start\", \"message\": {\"id\": \"msg_123\", \"type\": \"message\", \"role\": \"assistant\", \"model\": \"claude-3-5-sonnet\", \"content\": [], \"usage\": {\"input_tokens\": 10, \"output_tokens\": 0}}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\": \"content_block_start\", \"index\": 2, \"content_block\": {\"type\": \"tool_use\", \"id\": \"call_2\", \"name\": \"grep\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\": \"content_block_delta\", \"index\": 2, \"delta\": {\"type\": \"input_json_delta\", \"partial_json\": \"{\\\"query\\\":\\\"test\\\"}\"}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\": \"content_block_start\", \"index\": 0, \"content_block\": {\"type\": \"tool_use\", \"id\": \"call_0\", \"name\": \"bash\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\": \"content_block_delta\", \"index\": 0, \"delta\": {\"type\": \"input_json_delta\", \"partial_json\": \"{\\\"command\\\":\\\"ls\\\"}\"}}\n\n" +
		"event: content_block_start\n" +
		"data: {\"type\": \"content_block_start\", \"index\": 1, \"content_block\": {\"type\": \"tool_use\", \"id\": \"call_1\", \"name\": \"view\"}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"type\": \"content_block_delta\", \"index\": 1, \"delta\": {\"type\": \"input_json_delta\", \"partial_json\": \"{\\\"path\\\":\\\"a.txt\\\"}\"}}\n\n" +
		"event: message_delta\n" +
		"data: {\"type\": \"message_delta\", \"delta\": {\"stop_reason\": \"tool_use\"}, \"usage\": {\"output_tokens\": 20}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\": \"message_stop\"}\n\n"

	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		headers.Set("Content-Type", "text/event-stream")
		return &http.Response{
			StatusCode: 200,
			Header:     headers,
			Body:       ioNopCloser{bytes.NewBufferString(sseData)},
		}, nil
	})}

	client := newAnthropicClient(config.ProviderConfig{
		Preset:  config.ProviderPresetAnthropic,
		BaseURL: "http://api.example.com",
		APIKey:  "test-key",
		Model:   "claude-3-5-sonnet",
	})
	aClient := client.(*anthropicClient)
	aClient.httpClient = httpClient // swap client transport client
	sdkClient := anthropic.NewClient(
		option.WithAPIKey(aClient.apiKey),
		option.WithBaseURL(aClient.baseURL),
		option.WithHTTPClient(httpClient),
	)
	aClient.sdkClient = &sdkClient

	resp, err := aClient.StreamComplete(context.Background(), "system", []ConversationItem{UserText("hello")}, ToolSet{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.PendingCalls) != 3 {
		t.Fatalf("expected 3 pending calls, got %d", len(resp.PendingCalls))
	}

	// Verify order: index 0 (bash), index 1 (view), index 2 (grep)
	if resp.PendingCalls[0].Name != "bash" || resp.PendingCalls[0].CallID != "call_0" {
		t.Errorf("expected first call to be bash (call_0), got name %q, ID %q", resp.PendingCalls[0].Name, resp.PendingCalls[0].CallID)
	}
	if resp.PendingCalls[1].Name != "view" || resp.PendingCalls[1].CallID != "call_1" {
		t.Errorf("expected second call to be view (call_1), got name %q, ID %q", resp.PendingCalls[1].Name, resp.PendingCalls[1].CallID)
	}
	if resp.PendingCalls[2].Name != "grep" || resp.PendingCalls[2].CallID != "call_2" {
		t.Errorf("expected third call to be grep (call_2), got name %q, ID %q", resp.PendingCalls[2].Name, resp.PendingCalls[2].CallID)
	}
}
