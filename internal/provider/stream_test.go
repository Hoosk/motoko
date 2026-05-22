package provider

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

func TestMergeChatToolCallDeltasBuildsPendingCall(t *testing.T) {
	acc := map[int]*chatCompletionToolCall{}
	mergeChatToolCallDeltas(acc, []chatCompletionToolCallDelta{{Index: 0, ID: "call_1", Type: "function", Function: chatCompletionToolFunction{Name: "bash", Arguments: `{"input":"ls`}}})
	mergeChatToolCallDeltas(acc, []chatCompletionToolCallDelta{{Index: 0, Function: chatCompletionToolFunction{Arguments: ` -F"}`}}})
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type ioNopCloser struct{ *bytes.Buffer }

func (ioNopCloser) Close() error { return nil }
