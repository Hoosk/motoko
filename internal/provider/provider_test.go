package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
)

func TestNewClientUsesNormalizedProviderKinds(t *testing.T) {
	client, err := NewClient(config.ProviderConfig{Preset: config.ProviderPresetOpenRouter, Model: "openai/gpt-4.1", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.(*openAIClient); !ok {
		t.Fatalf("expected openAIClient for openrouter preset, got %T", client)
	}
}

func TestResponseFromTextKeepsUsageForPlainText(t *testing.T) {
	resp := responseFromText("hola", Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5})
	if resp.FinalText != "hola" || resp.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected text response %#v", resp)
	}
}

func TestToolResultForInvocationUsesFallbackToolName(t *testing.T) {
	item := ToolResultForInvocation(ToolInvocation{}, "ok")
	if !strings.Contains(item.Content, "tool_name=tool") {
		t.Fatalf("expected fallback tool name in %q", item.Content)
	}
}

func TestPostJSONReturnsProviderErrorBeforeDecode(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Header:     make(http.Header),
			Body:       ioNopCloser{bytes.NewBufferString("upstream failed")},
		}, nil
	})}
	err := postJSON(context.Background(), client, "http://example.test", map[string]string{"a": "b"}, nil, &struct{}{})
	if err == nil || err.Error() != "provider error 500: upstream failed" {
		t.Fatalf("unexpected postJSON error %v", err)
	}
}

func TestGetJSONReturnsProviderStatusWithoutBody(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 502,
			Header:     make(http.Header),
			Body:       ioNopCloser{bytes.NewBuffer(nil)},
		}, nil
	})}
	err := getJSON(context.Background(), client, "http://example.test", nil, &struct{}{})
	if err == nil || err.Error() != "provider error 502" {
		t.Fatalf("unexpected getJSON error %v", err)
	}
}

func TestNewBaseClientNormalizesFieldsAndStatus(t *testing.T) {
	client := newBaseClient(" openai ", " https://api.example.com/v1/ ", " key ", " gpt-4.1 ")
	if client.providerName != " openai " {
		t.Fatalf("expected provider name preserved, got %q", client.providerName)
	}
	if client.baseURL != "https://api.example.com/v1" {
		t.Fatalf("expected normalized baseURL, got %q", client.baseURL)
	}
	if client.apiKey != "key" || client.model != "gpt-4.1" {
		t.Fatalf("unexpected normalized credentials %#v", client)
	}
	if !client.Configured() || !client.listReady() {
		t.Fatalf("expected ready/configured client %#v", client)
	}
	if client.Summary() != " openai :gpt-4.1" {
		t.Fatalf("unexpected summary %q", client.Summary())
	}
}

func TestNormalizeConversationRoleFallsBackToUser(t *testing.T) {
	if got := normalizeConversationRole(" TOOL "); got != RoleUser {
		t.Fatalf("expected fallback user role, got %q", got)
	}
}

func TestFormatToolResultContentIncludesMetadata(t *testing.T) {
	call := ToolInvocation{Name: "read", Input: "README.md", Arguments: []byte(`{"path":"README.md"}`), CallID: "abc123"}
	content := formatToolResultContent(call, "ok")
	if !strings.Contains(content, "tool_name=read") || !strings.Contains(content, "call_id=abc123") || !strings.Contains(content, "tool_input=README.md") {
		t.Fatalf("expected tool metadata in %q", content)
	}
	encoded := base64.StdEncoding.EncodeToString(call.Arguments)
	if !strings.Contains(content, "tool_arguments_base64="+encoded) {
		t.Fatalf("expected encoded arguments in %q", content)
	}
	if !strings.Contains(content, "tool_output:\nok") {
		t.Fatalf("expected tool output in %q", content)
	}
}

func TestParseToolResultContentRoundTripsMetadata(t *testing.T) {
	call := ToolInvocation{Name: "read", Input: "README.md", Arguments: []byte(`{"input":"README.md"}`), CallID: "call_123", Raw: []byte(`{"raw":"metadata"}`)}
	parsedCall, output := parseToolResultContent(formatToolResultContent(call, "ok"))
	if parsedCall.Name != call.Name || parsedCall.Input != call.Input || parsedCall.CallID != call.CallID {
		t.Fatalf("unexpected parsed tool call %#v", parsedCall)
	}
	if string(parsedCall.Arguments) != string(call.Arguments) {
		t.Fatalf("unexpected parsed arguments %s", string(parsedCall.Arguments))
	}
	if string(parsedCall.Raw) != string(call.Raw) {
		t.Fatalf("unexpected parsed raw %s", string(parsedCall.Raw))
	}
	if output != "ok" {
		t.Fatalf("unexpected parsed output %q", output)
	}
}

func TestAssistantToolCallContentRoundTrips(t *testing.T) {
	call := ToolInvocation{Kind: InvokeCustomTool, Name: "read", Input: "README.md", Arguments: []byte(`{"input":"README.md"}`), CallID: "call_123", Raw: []byte(`{"id":"call_123","type":"function","function":{"name":"read","arguments":"{\"input\":\"README.md\"}"},"thought_signature":"sig"}`)}
	parsed, ok := parseAssistantToolCallContent(formatAssistantToolCallContent(call))
	if !ok {
		t.Fatal("expected assistant tool call metadata")
	}
	if parsed.Name != call.Name || parsed.Input != call.Input || parsed.CallID != call.CallID {
		t.Fatalf("unexpected parsed call %#v", parsed)
	}
	if string(parsed.Arguments) != string(call.Arguments) {
		t.Fatalf("unexpected parsed arguments %s", string(parsed.Arguments))
	}
	if string(parsed.Raw) != string(call.Raw) {
		t.Fatalf("unexpected parsed raw payload %s", string(parsed.Raw))
	}
}

func TestDecodeJSONResponseAllowsEmptyBodyWhenOutNil(t *testing.T) {
	resp := &http.Response{StatusCode: 204, Body: ioNopCloser{bytes.NewBuffer(nil)}}
	if err := decodeJSONResponse(resp, nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDecodeJSONResponseDecodesSuccessBody(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: ioNopCloser{bytes.NewBufferString(`{"message":"hola"}`)}}
	var out struct {
		Message string `json:"message"`
	}
	if err := decodeJSONResponse(resp, &out); err != nil {
		t.Fatalf("unexpected decode error %v", err)
	}
	if out.Message != "hola" {
		t.Fatalf("unexpected decoded body %#v", out)
	}
}

func TestThinkingHelpersMapThresholds(t *testing.T) {
	if !isOpenAIReasoningModel("gpt-5.5") || isOpenAIReasoningModel("gpt-4.1") {
		t.Fatal("unexpected openai reasoning model classification")
	}
	if got := budgetToReasoningEffort(65536); got != "xhigh" {
		t.Fatalf("unexpected reasoning effort %q", got)
	}
	if !isAnthropicAdaptiveThinkingModel("claude-opus-4-7") || !isAnthropicAdaptiveThinkingModel("claude-opus-4-8") || isAnthropicAdaptiveThinkingModel("claude-sonnet-4") {
		t.Fatal("unexpected anthropic adaptive classification")
	}
	if !isGemini3Model("gemini-3-pro") || isGemini3Model("gemini-2.5-pro") {
		t.Fatal("unexpected gemini 3 classification")
	}
	if got := budgetToGeminiThinkingLevel(8192); got != "medium" {
		t.Fatalf("unexpected gemini thinking level %q", got)
	}
}
