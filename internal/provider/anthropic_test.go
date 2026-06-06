package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestToSDKMessagesSkipsSystemAndNormalizesUnknownRole(t *testing.T) {
	got := toSDKMessages([]ConversationItem{{Role: RoleSystem, Content: "sys"}, {Role: "weird", Content: "hola"}, {Role: RoleAssistant, Content: "ok"}})
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Role != anthropic.MessageParamRoleUser || got[0].Content[0].OfText.Text != "hola" {
		t.Fatalf("unexpected first message: %#v", got[0])
	}
	if got[1].Role != anthropic.MessageParamRoleAssistant || got[1].Content[0].OfText.Text != "ok" {
		t.Fatalf("unexpected second message: %#v", got[1])
	}
}

func TestResponseFromSDKJoinsTextParts(t *testing.T) {
	sdkMsg := &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: "uno"},
			{Type: "tool_use", ID: "call_1", Name: "bash", Input: json.RawMessage(`{"input":"ls"}`)},
			{Type: "text", Text: "dos"},
		},
	}
	resp := responseFromSDK(sdkMsg)
	if resp.FinalText != "" {
		t.Fatalf("expected empty final text since tool use is present, got %q", resp.FinalText)
	}
	// Let's test with text only
	sdkMsgTextOnly := &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: "uno"},
			{Type: "text", Text: "dos"},
		},
	}
	respText := responseFromSDK(sdkMsgTextOnly)
	if respText.FinalText != "uno\ndos" {
		t.Fatalf("unexpected anthropic text %q", respText.FinalText)
	}
}

func TestToSDKMessagesToolCalling(t *testing.T) {
	// 1. Text message
	messages := []ConversationItem{
		UserText("hello"),
		AssistantText("world"),
	}
	got := toSDKMessages(messages)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Role != anthropic.MessageParamRoleUser || got[0].Content[0].OfText.Text != "hello" {
		t.Fatalf("unexpected user message: %#v", got[0])
	}
	if got[1].Role != anthropic.MessageParamRoleAssistant || got[1].Content[0].OfText.Text != "world" {
		t.Fatalf("unexpected assistant message: %#v", got[1])
	}

	// 2. Assistant tool call
	call := ToolInvocation{
		Kind:   InvokeCustomTool,
		Name:   "bash",
		Input:  "ls",
		CallID: "call_abc",
	}
	messages = []ConversationItem{
		{Role: RoleAssistant, Content: formatAssistantToolCallContent(call)},
	}
	got = toSDKMessages(messages)
	if len(got) != 1 || got[0].Role != anthropic.MessageParamRoleAssistant {
		t.Fatalf("expected 1 assistant message, got %#v", got)
	}
	blocks := got[0].Content
	if len(blocks) != 1 {
		t.Fatalf("expected content blocks slice, got %#v", got[0].Content)
	}
	b := blocks[0]
	if b.OfToolUse == nil || b.OfToolUse.ID != "call_abc" || b.OfToolUse.Name != "bash" {
		t.Fatalf("unexpected tool_use block: %#v", b.OfToolUse)
	}

	// 3. Tool result
	resultItem := ToolResultForInvocation(call, "result_text")
	messages = []ConversationItem{resultItem}
	got = toSDKMessages(messages)
	if len(got) != 1 || got[0].Role != anthropic.MessageParamRoleUser {
		t.Fatalf("expected 1 user message (anthropic tool results are user messages), got %#v", got)
	}
	blocks = got[0].Content
	if len(blocks) != 1 {
		t.Fatalf("expected content blocks slice, got %#v", got[0].Content)
	}
	b = blocks[0]
	if b.OfToolResult == nil || b.OfToolResult.ToolUseID != "call_abc" {
		t.Fatalf("unexpected tool_result block: %#v", b.OfToolResult)
	}
	trBlocks := b.OfToolResult.Content
	if len(trBlocks) != 1 || trBlocks[0].OfText == nil || trBlocks[0].OfText.Text != "result_text" {
		t.Fatalf("unexpected tool_result content: %#v", trBlocks)
	}
}

func TestResponseFromSDKToolUse(t *testing.T) {
	rawJSON := `{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"model": "claude-3-5-sonnet",
		"content": [
			{"type": "text", "text": "Let me check"},
			{"type": "tool_use", "id": "call_123", "name": "grep", "input": {"input": "todo"}}
		],
		"usage": {
			"input_tokens": 10,
			"output_tokens": 20
		}
	}`
	var sdkMsg anthropic.Message
	if err := json.Unmarshal([]byte(rawJSON), &sdkMsg); err != nil {
		t.Fatal(err)
	}

	resp := responseFromSDK(&sdkMsg)
	if resp.FinalText != "" {
		t.Fatalf("expected empty final text since tool use is present, got %q", resp.FinalText)
	}
	if len(resp.PendingCalls) != 1 {
		t.Fatalf("expected 1 pending call, got %d", len(resp.PendingCalls))
	}
	pc := resp.PendingCalls[0]
	if pc.Name != "grep" || pc.CallID != "call_123" || pc.Input != "todo" {
		t.Fatalf("unexpected pending call: %#v", pc)
	}
}

func TestToSDKTools(t *testing.T) {
	tools := ToolSet{
		Local: []LocalToolDefinition{
			{Name: "bash", Description: "run bash cmd", InputHint: "command"},
		},
	}
	serialized := toSDKTools(tools)
	if len(serialized) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(serialized))
	}
	toolUnion := serialized[0]
	if toolUnion.OfTool == nil {
		t.Fatalf("expected OfTool to be set, got %#v", toolUnion)
	}
	tool := toolUnion.OfTool
	if tool.Name != "bash" || tool.Description.Value != "run bash cmd" {
		t.Fatalf("unexpected tool properties: %#v", tool)
	}
	if tool.InputSchema.Required[0] != "input" {
		t.Fatalf("unexpected input schema required: %#v", tool.InputSchema.Required)
	}
}

func TestAnthropicClientCheckAdaptiveThinking(t *testing.T) {
	// Case 1: Model supports adaptive thinking according to /v1/models response
	var callCount int
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		if !strings.HasSuffix(req.URL.Path, "/v1/models/claude-test-model") {
			return &http.Response{
				StatusCode: 404,
				Body:       ioNopCloser{bytes.NewBufferString("not found")},
			}, nil
		}
		responseJSON := `{
			"id": "claude-test-model",
			"capabilities": {
				"thinking": {
					"supported": true,
					"types": {
						"adaptive": {
							"supported": true
						}
					}
				}
			}
		}`
		headers := make(http.Header)
		headers.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: 200,
			Header:     headers,
			Body:       ioNopCloser{bytes.NewBufferString(responseJSON)},
		}, nil
	})}

	client := newAnthropicClient(config.ProviderConfig{
		Preset:  config.ProviderPresetAnthropic,
		BaseURL: "http://api.example.com",
		APIKey:  "test-key",
		Model:   "claude-test-model",
	})
	aClient := client.(*anthropicClient)
	aClient.httpClient = httpClient // swap client transport client
	sdkClient := anthropic.NewClient(
		option.WithAPIKey(aClient.apiKey),
		option.WithBaseURL(aClient.baseURL),
		option.WithHTTPClient(httpClient),
	)
	aClient.sdkClient = &sdkClient

	// First call should make an HTTP request
	isAdaptive := aClient.checkAdaptiveThinking(context.Background())
	if !isAdaptive {
		t.Fatal("expected model to support adaptive thinking")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	// Second call should return cached value (no HTTP request)
	isAdaptive2 := aClient.checkAdaptiveThinking(context.Background())
	if !isAdaptive2 {
		t.Fatal("expected cached value to be true")
	}
	if callCount != 1 {
		t.Fatalf("expected call count to remain 1, got %d", callCount)
	}

	// Case 2: HTTP fails, should fallback to model name check
	clientFallback := newAnthropicClient(config.ProviderConfig{
		Preset:  config.ProviderPresetAnthropic,
		BaseURL: "http://api.example.com",
		APIKey:  "test-key",
		Model:   "claude-opus-4-7", // fallback should return true
	})
	aClientFallback := clientFallback.(*anthropicClient)
	fallbackHTTPClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       ioNopCloser{bytes.NewBufferString("error")},
		}, nil
	})}
	sdkClientFallback := anthropic.NewClient(
		option.WithAPIKey(aClientFallback.apiKey),
		option.WithBaseURL(aClientFallback.baseURL),
		option.WithHTTPClient(fallbackHTTPClient),
	)
	aClientFallback.sdkClient = &sdkClientFallback

	if !aClientFallback.checkAdaptiveThinking(context.Background()) {
		t.Fatal("expected fallback to detect adaptive thinking based on model name prefix")
	}
}
