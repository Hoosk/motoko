package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/openai/openai-go/v3/responses"
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

func TestMessageSerializationHelpers(t *testing.T) {
	// OpenAI Responses API: toResponsesInputItems maps messages to input item params.
	items := toResponsesInputItems([]Message{{Role: "user", Content: "hola"}, {Role: "assistant", Content: "mundo"}})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].OfMessage == nil || items[0].OfMessage.Role != responses.EasyInputMessageRoleUser {
		t.Fatalf("expected user role on first item, got %#v", items[0])
	}
	if items[1].OfMessage == nil || items[1].OfMessage.Role != responses.EasyInputMessageRoleAssistant {
		t.Fatalf("expected assistant role on second item, got %#v", items[1])
	}

	anthropic := toAnthropicMessages([]Message{{Role: "system", Content: "ignored"}, {Role: "user", Content: "hola"}})
	if len(anthropic) != 1 || anthropic[0]["role"] != "user" {
		t.Fatalf("unexpected anthropic messages %#v", anthropic)
	}
	gemini, err := NewClient(config.ProviderConfig{Preset: config.ProviderPresetGemini, APIKey: "k", Model: "gemini-3.5-flash"})
	if err != nil {
		t.Fatal(err)
	}
	geminiClient, ok := gemini.(*geminiClient)
	if !ok {
		t.Fatalf("expected gemini client, got %T", gemini)
	}
	if !strings.Contains(geminiClient.baseURL, "v1beta") {
		t.Fatalf("expected gemini base url to contain v1beta, got %q", geminiClient.baseURL)
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

func TestBuildResponseParamsUsesTemperatureForNonReasoningModels(t *testing.T) {
	params := buildResponseParams("gpt-4.1-mini", "system", []ConversationItem{UserText("hola")}, 8192)
	if params.Temperature.Value != 0.2 {
		t.Fatalf("expected temperature for non-reasoning model, got %#v", params.Temperature)
	}
	if params.Reasoning.Effort != "" {
		t.Fatalf("expected no reasoning effort, got %#v", params.Reasoning)
	}
	if params.Instructions.Value != "system" {
		t.Fatalf("unexpected instructions %#v", params.Instructions)
	}
	if len(params.Input.OfInputItemList) != 1 {
		t.Fatalf("expected one input item, got %#v", params.Input)
	}
}

func TestBuildResponseParamsUsesReasoningForOpenAIReasoningModels(t *testing.T) {
	params := buildResponseParams("gpt-5.4", "system", []ConversationItem{AssistantText("hola")}, 24576)
	if params.Reasoning.Effort != "high" {
		t.Fatalf("expected high reasoning effort, got %#v", params.Reasoning)
	}
	if len(params.Input.OfInputItemList) != 1 || params.Input.OfInputItemList[0].OfMessage == nil {
		t.Fatalf("unexpected input items %#v", params.Input)
	}
	if params.Input.OfInputItemList[0].OfMessage.Role != responses.EasyInputMessageRoleAssistant {
		t.Fatalf("expected assistant role, got %#v", params.Input.OfInputItemList[0].OfMessage)
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

func TestToAnthropicMessagesSkipsSystemAndNormalizesUnknownRole(t *testing.T) {
	got := toAnthropicMessages([]ConversationItem{{Role: RoleSystem, Content: "sys"}, {Role: "weird", Content: "hola"}, {Role: RoleAssistant, Content: "ok"}})
	want := []map[string]string{{"role": RoleUser, "content": "hola"}, {"role": RoleAssistant, "content": "ok"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected anthropic messages %#v", got)
	}
}

func TestAnthropicResponseTextJoinsTextParts(t *testing.T) {
	resp := anthropicResponse{Content: []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{{Type: "text", Text: "uno"}, {Type: "tool_use", Text: "skip"}, {Type: "text", Text: "dos"}}}
	if got := resp.Text(); got != "uno\ndos" {
		t.Fatalf("unexpected anthropic text %q", got)
	}
}

func TestThinkingHelpersMapThresholds(t *testing.T) {
	if !isOpenAIReasoningModel("gpt-5.5") || isOpenAIReasoningModel("gpt-4.1") {
		t.Fatal("unexpected openai reasoning model classification")
	}
	if got := budgetToReasoningEffort(65536); got != "xhigh" {
		t.Fatalf("unexpected reasoning effort %q", got)
	}
	if !isAnthropicAdaptiveThinkingModel("claude-opus-4-7") || isAnthropicAdaptiveThinkingModel("claude-sonnet-4") {
		t.Fatal("unexpected anthropic adaptive classification")
	}
	if !isGemini3Model("gemini-3-pro") || isGemini3Model("gemini-2.5-pro") {
		t.Fatal("unexpected gemini 3 classification")
	}
	if got := budgetToGeminiThinkingLevel(8192); got != "medium" {
		t.Fatalf("unexpected gemini thinking level %q", got)
	}
}

func TestResponsesInputItemsNormalizeUnknownRoleToUser(t *testing.T) {
	items := toResponsesInputItems([]ConversationItem{{Role: "otro", Content: "hola"}})
	if len(items) != 1 || items[0].OfMessage == nil {
		t.Fatalf("unexpected response input items %#v", items)
	}
	if items[0].OfMessage.Role != responses.EasyInputMessageRoleUser {
		t.Fatalf("expected user role, got %#v", items[0].OfMessage)
	}
}

func TestBuildResponseParamsLeavesReasoningEmptyWithoutBudget(t *testing.T) {
	params := buildResponseParams("o4-mini", "system", nil, 0)
	if params.Reasoning.Effort != "" {
		t.Fatalf("expected empty reasoning effort without budget, got %#v", params.Reasoning)
	}
}
