package provider

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
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
