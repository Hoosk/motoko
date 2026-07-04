package openai

import (
	"encoding/json"
	"strings"
	"testing"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
)

func TestChatStreamingIncludesUsage(t *testing.T) {
	params := openai.ChatCompletionNewParams{
		Model:       openai.ChatModel("gpt-4.1-mini"),
		Messages:    []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("system")},
		Temperature: param.NewOpt(0.2),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.NewOpt(true),
		},
	}
	if !params.StreamOptions.IncludeUsage.Value {
		t.Fatal("expected stream options to include usage")
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"stream_options":{"include_usage":true}`) {
		t.Fatalf("expected stream_options include_usage in payload, got %s", string(encoded))
	}
	params.PromptCacheKey = param.NewOpt("sess-123")
	encoded, err = json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"prompt_cache_key":"sess-123"`) {
		t.Fatalf("expected prompt_cache_key in payload, got %s", string(encoded))
	}
}

func TestMessageSerializationHelpers(t *testing.T) {
	items := toResponsesInputItems([]provider.Message{{Role: "user", Content: "hola"}, {Role: "assistant", Content: "mundo"}})
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
	params := buildResponseParams("gpt-4.1-mini", "system", []provider.ConversationItem{provider.UserText("hola")}, provider.ToolSet{}, 0, "")
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
	params := buildResponseParams("o1-preview", "system", []provider.ConversationItem{provider.AssistantText("hola")}, provider.ToolSet{}, 24576, "")
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
	params := buildResponseParams("gpt-4.1-mini", "system", nil, provider.ToolSet{Local: []provider.LocalToolDefinition{{Name: "bash", Description: "Run shell", InputHint: "bash <cmd>"}}}, 0, "")
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
	items := toResponsesInputItems([]provider.ConversationItem{{Role: "otro", Content: "hola"}})
	if len(items) != 1 || items[0].OfMessage == nil {
		t.Fatalf("unexpected response input items %#v", items)
	}
	if items[0].OfMessage.Role != responses.EasyInputMessageRoleUser {
		t.Fatalf("expected user role, got %#v", items[0].OfMessage)
	}
}

func TestResponsesInputItemsSerializeToolResultsAsFunctionOutputs(t *testing.T) {
	item := provider.ToolResultForInvocation(provider.ToolInvocation{Name: "read", CallID: "call_123"}, "contenido")
	items := toResponsesInputItems([]provider.ConversationItem{item})
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
	items := toResponsesInputItems(provider.AssistantToolCallItems([]provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "bash", Input: "ls -F", CallID: "call_789"}}))
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

func TestResponseFromChatCompletionKeepsReasoningAndToolCallsTogether(t *testing.T) {
	resp := responseFromChatCompletion(chatCompletionResponse{
		Choices: []chatCompletionChoice{{Message: chatCompletionMessage{
			Content:          "checking",
			ReasoningContent: "need to inspect files",
			ToolCalls: []chatCompletionToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: chatCompletionToolFunction{
					Name:      "glob",
					Arguments: `{"input":"**/*.go"}`,
				},
			}},
		}}},
	})
	if len(resp.OutputItems) != 1 {
		t.Fatalf("expected one assistant turn, got %#v", resp.OutputItems)
	}
	item := resp.OutputItems[0]
	if item.Content != "checking" || item.ReasoningContent != "need to inspect files" || len(item.ToolCalls) != 1 {
		t.Fatalf("unexpected assistant turn %#v", item)
	}
	if resp.FinalText != "" {
		t.Fatalf("expected empty final text when tool calls are pending, got %q", resp.FinalText)
	}
}

func TestChatMessagesReuseRawAssistantToolCallPayload(t *testing.T) {
	raw := []byte(`{"id":"call_789","type":"function","function":{"name":"bash","arguments":"{\"input\":\"ls -F\"}"},"thought_signature":"sig"}`)
	messages := toChatMessages(provider.AssistantToolCallItems([]provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "bash", Input: "ls -F", CallID: "call_789", Raw: raw}}))
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
	messages := toChatMessages([]provider.ConversationItem{provider.UserText("hello")})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0]["role"] != provider.RoleUser || messages[0]["content"] != "hello" {
		t.Fatalf("unexpected serialized message: %#v", messages[0])
	}

	callItems := provider.AssistantToolCallItems([]provider.ToolInvocation{
		{Kind: provider.InvokeCustomTool, Name: "grep", Arguments: json.RawMessage(`{"pattern": "func"}`), CallID: "call_abc"},
	})
	messages = toChatMessages(callItems)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg := messages[0]
	if msg["role"] != provider.RoleAssistant {
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

	withReasoning := provider.AssistantTurn("", "thinking...", []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "grep", Arguments: json.RawMessage(`{"pattern": "func"}`), CallID: "call_reasoning"}})
	messages = toChatMessages([]provider.ConversationItem{withReasoning})
	if messages[0]["reasoning_content"] != "thinking..." {
		t.Fatalf("expected reasoning_content to be serialized, got %#v", messages[0])
	}

	resultItem := provider.ToolResultForInvocation(provider.ToolInvocation{Name: "grep", CallID: "call_abc"}, "found 3 matches")
	messages = toChatMessages([]provider.ConversationItem{resultItem})
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg = messages[0]
	if msg["role"] != provider.RoleTool {
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
	params := buildResponseParams("o4-mini", "system", nil, provider.ToolSet{}, 0, "")
	if params.Reasoning.Effort != "" {
		t.Fatalf("expected empty reasoning effort without budget, got %#v", params.Reasoning)
	}
}

func TestBuildResponseParamsIncludesPromptCacheKey(t *testing.T) {
	params := buildResponseParams("gpt-4.1-mini", "system", []provider.ConversationItem{provider.UserText("hola")}, provider.ToolSet{}, 0, "sess-123")
	if params.PromptCacheKey.Value != "sess-123" {
		t.Fatalf("expected prompt cache key, got %#v", params.PromptCacheKey)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"prompt_cache_key":"sess-123"`) {
		t.Fatalf("expected prompt_cache_key in responses payload, got %s", string(encoded))
	}
}

func TestOpenAIClientUseChatCompletions(t *testing.T) {
	client := NewClient(config.ProviderConfig{
		Preset:  config.ProviderPresetOpenAI,
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "key",
		Model:   "gpt-4",
	})
	if client.(*openAIClient).useChatCompletions {
		t.Fatal("expected useChatCompletions false for OpenAI preset")
	}

	clientComp := NewClient(config.ProviderConfig{
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

	clientLM := NewClient(config.ProviderConfig{
		Preset:  config.ProviderPresetLMStudio,
		BaseURL: "http://localhost:1234/v1",
		APIKey:  "key",
		Model:   "llama",
	})
	if clientLM.(*openAIClient).useChatCompletions {
		t.Fatal("expected useChatCompletions false for LMStudio preset")
	}
}

func TestToSDKChatMessagesAndTools(t *testing.T) {
	sdkMsgs := toSDKChatMessages([]provider.ConversationItem{provider.UserText("hi")})
	if len(sdkMsgs) != 1 {
		t.Fatalf("expected 1 sdk message, got %d", len(sdkMsgs))
	}
	if sdkMsgs[0].OfUser == nil || sdkMsgs[0].OfUser.Content.OfString.Value != "hi" {
		t.Fatalf("unexpected user message mapping: %#v", sdkMsgs[0])
	}

	resultItem := provider.ToolResultForInvocation(provider.ToolInvocation{Name: "ls", CallID: "call_999"}, "file.go")
	sdkMsgs = toSDKChatMessages([]provider.ConversationItem{resultItem})
	if len(sdkMsgs) != 1 {
		t.Fatalf("expected 1 sdk message, got %d", len(sdkMsgs))
	}
	if sdkMsgs[0].OfTool == nil || sdkMsgs[0].OfTool.Content.OfString.Value != "file.go" || sdkMsgs[0].OfTool.ToolCallID != "call_999" {
		t.Fatalf("unexpected tool message mapping: %#v", sdkMsgs[0])
	}

	tools := provider.ToolSet{Local: []provider.LocalToolDefinition{{Name: "ls", Description: "List files", InputHint: "dir"}}}
	sdkTools := toSDKChatTools(tools)
	if len(sdkTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(sdkTools))
	}
	if sdkTools[0].OfFunction == nil || sdkTools[0].OfFunction.Function.Name != "ls" {
		t.Fatalf("unexpected tool mapping: %#v", sdkTools[0])
	}
}
