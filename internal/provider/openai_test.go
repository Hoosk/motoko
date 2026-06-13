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
}

func TestBuildResponseParamsUsesTemperatureForNonReasoningModels(t *testing.T) {
	params := buildResponseParams("gpt-4.1-mini", "system", []ConversationItem{UserText("hola")}, ToolSet{}, 0)
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
	params := buildResponseParams("o1-preview", "system", []ConversationItem{AssistantText("hola")}, ToolSet{}, 24576)
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
	messages := toChatMessages(assistantToolCallItems([]ToolInvocation{{Kind: InvokeCustomTool, Name: "bash", Input: "ls -F", CallID: "call_789", Raw: raw}}))
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

func TestToChatMessagesStructuredFlow(t *testing.T) {
	// Test standard user message
	messages := toChatMessages([]ConversationItem{UserText("hello")})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0]["role"] != RoleUser || messages[0]["content"] != "hello" {
		t.Fatalf("unexpected serialized message: %#v", messages[0])
	}

	// Test assistant tool call without raw payload
	callItems := assistantToolCallItems([]ToolInvocation{
		{Kind: InvokeCustomTool, Name: "grep", Arguments: json.RawMessage(`{"pattern": "func"}`), CallID: "call_abc"},
	})
	messages = toChatMessages(callItems)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg := messages[0]
	if msg["role"] != RoleAssistant {
		t.Fatalf("expected role assistant, got %v", msg["role"])
	}
	if msg["content"] != "" {
		t.Fatalf("expected empty content, got %q", msg["content"])
	}
	toolCalls, ok := msg["tool_calls"].([]map[string]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected tool_calls slice of length 1, got %#v", msg["tool_calls"])
	}
	call := toolCalls[0]
	if call["id"] != "call_abc" || call["type"] != "function" {
		t.Fatalf("unexpected tool call values: %#v", call)
	}
	fn, ok := call["function"].(map[string]any)
	if !ok || fn["name"] != "grep" || fn["arguments"] != `{"pattern": "func"}` {
		t.Fatalf("unexpected function values: %#v", call["function"])
	}

	// Test tool result message (should NOT degrade to text or RoleUser)
	resultItem := ToolResultForInvocation(ToolInvocation{Name: "grep", CallID: "call_abc"}, "found 3 matches")
	messages = toChatMessages([]ConversationItem{resultItem})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg = messages[0]
	if msg["role"] != RoleTool {
		t.Fatalf("expected role tool, got %v", msg["role"])
	}
	if msg["content"] != "found 3 matches" {
		t.Fatalf("unexpected content, got %q", msg["content"])
	}
	if msg["tool_call_id"] != "call_abc" {
		t.Fatalf("expected tool_call_id call_abc, got %v", msg["tool_call_id"])
	}
	if msg["name"] != "grep" {
		t.Fatalf("expected name grep, got %v", msg["name"])
	}
}

func TestBuildResponseParamsLeavesReasoningEmptyWithoutBudget(t *testing.T) {
	params := buildResponseParams("o4-mini", "system", nil, ToolSet{}, 0)
	if params.Reasoning.Effort != "" {
		t.Fatalf("expected empty reasoning effort without budget, got %#v", params.Reasoning)
	}
}

func TestOpenAIClientUseChatCompletions(t *testing.T) {
	// Preset OpenAI -> useChatCompletions is false
	client := newOpenAIClient(config.ProviderConfig{
		Preset:  config.ProviderPresetOpenAI,
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "key",
		Model:   "gpt-4",
	})
	if client.(*openAIClient).useChatCompletions {
		t.Fatal("expected useChatCompletions false for OpenAI preset")
	}

	// Preset OpenAICompatible -> useChatCompletions is true
	clientComp := newOpenAIClient(config.ProviderConfig{
		Preset:  config.ProviderPresetOpenAICompatible,
		BaseURL: "http://localhost:11434/v1",
		APIKey:  "key",
		Model:   "llama",
		UseSDK:  true,
	})
	if !clientComp.(*openAIClient).useChatCompletions {
		t.Fatal("expected useChatCompletions true for OpenAICompatible preset")
	}
	if !clientComp.(*openAIClient).useSDK {
		t.Fatal("expected useSDK true when UseSDK is true in configuration")
	}
}

func TestToSDKChatMessagesAndTools(t *testing.T) {
	// Test toSDKChatMessages with user message
	sdkMsgs := toSDKChatMessages([]ConversationItem{UserText("hi")})
	if len(sdkMsgs) != 1 {
		t.Fatalf("expected 1 sdk message, got %d", len(sdkMsgs))
	}
	if sdkMsgs[0].OfUser == nil || sdkMsgs[0].OfUser.Content.OfString.Value != "hi" {
		t.Fatalf("unexpected user message mapping: %#v", sdkMsgs[0])
	}

	// Test toSDKChatMessages with tool result
	resultItem := ToolResultForInvocation(ToolInvocation{Name: "ls", CallID: "call_999"}, "file.go")
	sdkMsgs = toSDKChatMessages([]ConversationItem{resultItem})
	if len(sdkMsgs) != 1 {
		t.Fatalf("expected 1 sdk message, got %d", len(sdkMsgs))
	}
	if sdkMsgs[0].OfTool == nil || sdkMsgs[0].OfTool.Content.OfString.Value != "file.go" || sdkMsgs[0].OfTool.ToolCallID != "call_999" {
		t.Fatalf("unexpected tool message mapping: %#v", sdkMsgs[0])
	}

	// Test toSDKChatTools
	tools := ToolSet{Local: []LocalToolDefinition{{Name: "bash", Description: "Run command", InputHint: "command"}}}
	sdkTools := toSDKChatTools(tools)
	if len(sdkTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(sdkTools))
	}
	if sdkTools[0].OfFunction == nil || sdkTools[0].OfFunction.Function.Name != "bash" {
		t.Fatalf("unexpected tool mapping: %#v", sdkTools[0])
	}
}
