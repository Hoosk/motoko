package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/openai/openai-go/v3/responses"
)

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

func TestBuildResponseParamsUsesTemperatureForNonReasoningModels(t *testing.T) {
	params := buildResponseParams("gpt-4.1-mini", "system", []ConversationItem{UserText("hola")}, ToolSet{}, 8192)
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
	params := buildResponseParams("gpt-5.4", "system", []ConversationItem{AssistantText("hola")}, ToolSet{}, 24576)
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

func TestBuildResponseParamsIncludesTools(t *testing.T) {
	params := buildResponseParams("gpt-4.1-mini", "system", nil, ToolSet{Local: []LocalToolDefinition{{Name: "bash", Description: "Run shell", InputHint: "bash <cmd>"}}}, 0)
	if len(params.Tools) != 1 {
		t.Fatalf("expected one tool, got %#v", params.Tools)
	}
	if params.MaxToolCalls.Value != 1 || params.ParallelToolCalls.Value {
		t.Fatalf("unexpected tool execution params %#v %#v", params.MaxToolCalls, params.ParallelToolCalls)
	}
	if params.ToolChoice.OfToolChoiceMode.Value != responses.ToolChoiceOptionsAuto {
		t.Fatalf("expected auto tool choice, got %#v", params.ToolChoice)
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

func TestResponsesInputItemsSerializeToolResultsAsFunctionOutputs(t *testing.T) {
	item := ToolResultForInvocation(ToolInvocation{Name: "read", CallID: "call_123"}, "contenido")
	items := toResponsesInputItems([]ConversationItem{item})
	if len(items) != 1 {
		t.Fatalf("expected one input item, got %#v", items)
	}
	encoded, err := json.Marshal(items[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"type":"function_call_output"`) || !strings.Contains(text, `"call_id":"call_123"`) || !strings.Contains(text, `"output":"contenido"`) {
		t.Fatalf("unexpected function call output payload %s", text)
	}
}

func TestResponsesInputItemsSerializeAssistantToolCalls(t *testing.T) {
	items := toResponsesInputItems(assistantToolCallItems([]ToolInvocation{{Kind: InvokeCustomTool, Name: "bash", Input: "ls -F", CallID: "call_789"}}))
	if len(items) != 1 {
		t.Fatalf("expected one input item, got %#v", items)
	}
	encoded, err := json.Marshal(items[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"type":"function_call"`) || !strings.Contains(text, `"call_id":"call_789"`) || !strings.Contains(text, `"name":"bash"`) {
		t.Fatalf("unexpected function call payload %s", text)
	}
}

func TestResponseFromChatCompletionMapsPromptAndCompletionTokens(t *testing.T) {
	resp := responseFromChatCompletion(chatCompletionResponse{
		Choices: []chatCompletionChoice{{Message: chatCompletionMessage{Content: "hola"}}},
		Usage:   chatCompletionUsage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18},
	})
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 7 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("unexpected chat completion usage %#v", resp.Usage)
	}
}

func TestChatMessagesReuseRawAssistantToolCallPayload(t *testing.T) {
	raw := []byte(`{"id":"call_789","type":"function","function":{"name":"bash","arguments":"{\"input\":\"ls -F\"}"},"thought_signature":"sig"}`)
	messages := toChatMessages(assistantToolCallItems([]ToolInvocation{{Kind: InvokeCustomTool, Name: "bash", Input: "ls -F", CallID: "call_789", Raw: raw}}), false)
	if len(messages) != 1 {
		t.Fatalf("expected one chat message, got %#v", messages)
	}
	toolCalls, ok := messages[0]["tool_calls"].([]map[string]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected raw tool call payload, got %#v", messages[0])
	}
	if toolCalls[0]["thought_signature"] != "sig" {
		t.Fatalf("expected raw thought signature preserved, got %#v", toolCalls[0])
	}
}

func TestBuildResponseParamsLeavesReasoningEmptyWithoutBudget(t *testing.T) {
	params := buildResponseParams("o4-mini", "system", nil, ToolSet{}, 0)
	if params.Reasoning.Effort != "" {
		t.Fatalf("expected empty reasoning effort without budget, got %#v", params.Reasoning)
	}
}

func TestOpenAIClientForceChat(t *testing.T) {
	// Preset OpenAI -> forceChat is false
	client := newOpenAIClient(config.ProviderConfig{
		Preset:  config.ProviderPresetOpenAI,
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "key",
		Model:   "gpt-4",
	})
	if client.(*openAIClient).forceChat {
		t.Fatal("expected forceChat false for OpenAI preset")
	}

	// Preset OpenAICompatible -> forceChat is true
	clientComp := newOpenAIClient(config.ProviderConfig{
		Preset:  config.ProviderPresetOpenAICompatible,
		BaseURL: "http://localhost:11434/v1",
		APIKey:  "key",
		Model:   "llama",
	})
	if !clientComp.(*openAIClient).forceChat {
		t.Fatal("expected forceChat true for OpenAICompatible preset")
	}
}
