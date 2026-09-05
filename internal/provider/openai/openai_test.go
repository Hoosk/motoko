package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
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
	if params.MaxToolCalls.Value != 8 || !params.ParallelToolCalls.Value {
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

func TestToSDKChatMessagesNeverMarshalNullContent(t *testing.T) {
	poisonedRaw := json.RawMessage(`{"function":{"arguments":"{\"path\": \"notas.md\", \"content\": \"x\"}","name":null},"id":null,"type":"function"}`)
	items := []provider.ConversationItem{
		provider.UserText("resume work"),
		{
			Role:             provider.RoleAssistant,
			Content:          "",
			ReasoningContent: "thinking",
			ToolCalls: []provider.ToolInvocation{
				{
					Kind:   provider.InvokeCustomTool,
					Name:   "read",
					CallID: "call_1",
					Raw:    json.RawMessage(`{"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"input\":\"index.html\"}"}}`),
				},
				{Kind: provider.InvokeCustomTool, Name: "glob", CallID: "call_2"},
			},
		},
		{Role: provider.RoleTool, ToolCallID: "call_1", Content: strings.Repeat("x", 11000)},
		{Role: provider.RoleTool, ToolCallID: "call_2", Content: "2 matches"},
		{
			Role:    provider.RoleAssistant,
			Content: "",
			ToolCalls: []provider.ToolInvocation{
				{Kind: provider.InvokeCustomTool, Name: "write", CallID: "call_3", Input: `{"path": "notas.md"}`, Raw: poisonedRaw},
			},
		},
		{Role: provider.RoleTool, ToolCallID: "call_3", Content: "tool error: change rejected by user: notas.md"},
		{Role: provider.RoleAssistant, ReasoningContent: "reasoning only"},
		provider.AssistantText("done"),
	}

	for _, builder := range []func([]provider.ConversationItem) []map[string]any{toChatMessages} {
		mapped := builder(items)
		raw, err := json.Marshal(mapped)
		if err != nil {
			t.Fatal(err)
		}
		if err := rejectNullValues(raw); err != nil {
			t.Fatalf("toChatMessages emitted null: %v", err)
		}
	}

	sdkMessages := append([]openai.ChatCompletionMessageParamUnion{openai.SystemMessage("system prompt")}, toSDKChatMessages(items)...)
	raw, err := json.Marshal(openai.ChatCompletionNewParams{
		Model:    "deepseek-v4-flash",
		Messages: sdkMessages,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectNullValues(raw); err != nil {
		t.Fatalf("toSDKChatMessages emitted null: %v", err)
	}
}

func rejectNullValues(raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	var firstNull string
	var walk func(any, string)
	walk = func(v any, path string) {
		if firstNull != "" {
			return
		}
		switch typed := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for k := range typed {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(typed[k], path+"."+k)
			}
		case []any:
			for i, item := range typed {
				walk(item, fmt.Sprintf("%s[%d]", path, i))
			}
		case nil:
			firstNull = path
		}
	}
	walk(value, "$")
	if firstNull != "" {
		return fmt.Errorf("null at %s", firstNull)
	}
	return nil
}

func TestMergeChatToolCallDeltasIgnoresNulls(t *testing.T) {
	acc := make(map[int]*chatCompletionToolCall)
	mapped := make(map[int]int)
	mergeChatToolCallDeltas(acc, []chatCompletionToolCallDelta{
		{Index: 0, ID: "call_x", Type: "function", Function: chatCompletionToolFunction{Name: "write"}, Raw: json.RawMessage(`{"index":0,"id":"call_x","type":"function","function":{"name":"write","arguments":""}}`)},
		{Index: 0, Function: chatCompletionToolFunction{Arguments: `{"a":1}`}, Raw: json.RawMessage(`{"index":0,"id":null,"function":{"name":null,"arguments":"{\"a\":1}"}}`)},
	}, mapped)

	call := acc[0]
	if call == nil {
		t.Fatal("expected accumulated tool call")
	}
	if call.ID != "call_x" || call.Function.Name != "write" || call.Function.Arguments != `{"a":1}` {
		t.Fatalf("unexpected accumulated call %#v", call)
	}
	rawID := call.RawMap["id"]
	rawName := call.RawMap["function"].(map[string]any)["name"]
	if rawID != "call_x" || rawName != "write" {
		t.Fatalf("expected null deltas to be ignored, got id=%v name=%v", rawID, rawName)
	}
}

func TestToChatMessagesSanitizesPoisonedRawToolCall(t *testing.T) {
	item := provider.ConversationItem{
		Role: provider.RoleAssistant,
		ToolCalls: []provider.ToolInvocation{{
			Kind:   provider.InvokeCustomTool,
			Name:   "write",
			CallID: "call_a42",
			Input:  `{"path": "notas.md"}`,
			Raw:    json.RawMessage(`{"function":{"arguments":"{\"path\": \"notas.md\"}","name":null},"id":null,"type":"function"}`),
		}},
	}
	messages := toChatMessages([]provider.ConversationItem{item})
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectNullValues(raw); err != nil {
		t.Fatalf("expected sanitized tool call, %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	call := decoded[0]["tool_calls"].([]any)[0].(map[string]any)
	if call["id"] != "call_a42" {
		t.Fatalf("expected id repaired from call id, got %v", call["id"])
	}
	fn := call["function"].(map[string]any)
	if fn["name"] != "write" {
		t.Fatalf("expected name repaired from call name, got %v", fn["name"])
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

func TestResponseFromRawSSEText(t *testing.T) {
	resp := &rawSSECompletedResponse{
		Output: []rawSSEOutputItem{
			{
				Type: "message",
				Content: []rawSSEContent{
					{Type: "output_text", Text: "Hello, world!"},
				},
			},
		},
		Usage: rawSSEUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}
	got := responseFromRawSSE(resp)
	if got.FinalText != "Hello, world!" {
		t.Errorf("expected FinalText=%q, got %q", "Hello, world!", got.FinalText)
	}
	if got.Usage.InputTokens != 10 || got.Usage.OutputTokens != 5 {
		t.Errorf("unexpected usage: %+v", got.Usage)
	}
}

func TestResponseFromRawSSEFunctionCall(t *testing.T) {
	resp := &rawSSECompletedResponse{
		Output: []rawSSEOutputItem{
			{
				Type:      "function_call",
				Name:      "bash",
				CallID:    "call_1",
				Arguments: `{"command":"ls"}`,
			},
		},
		Usage: rawSSEUsage{InputTokens: 20, OutputTokens: 10, TotalTokens: 30},
	}
	got := responseFromRawSSE(resp)
	if len(got.PendingCalls) != 1 {
		t.Fatalf("expected 1 pending call, got %d", len(got.PendingCalls))
	}
	call := got.PendingCalls[0]
	if call.Name != "bash" || call.CallID != "call_1" {
		t.Errorf("unexpected call: %+v", call)
	}
}

func TestResponseFromRawSSENil(t *testing.T) {
	got := responseFromRawSSE(nil)
	if got.FinalText != "" || len(got.PendingCalls) != 0 {
		t.Errorf("expected empty response for nil input, got %+v", got)
	}
}

func TestStreamResponsesHandlesKeepAlive(t *testing.T) {
	// Simulate the Zen SSE format: keep-alive comment + events with event: type lines.
	sseBody := ": keep-alive\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\"}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"Hello world\"}]}],\"usage\":{\"input_tokens\":5,\"output_tokens\":3,\"total_tokens\":8}}}\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	var deltas []string
	onDelta := func(d provider.Delta) error {
		if d.Content != "" {
			deltas = append(deltas, d.Content)
		}
		return nil
	}

	var completed *rawSSECompletedResponse
	err := postJSONStream(context.Background(), srv.Client(), srv.URL, map[string]any{}, map[string]string{}, func(data string) error {
		var ev struct {
			Response *rawSSECompletedResponse `json:"response"`
			Type     string                   `json:"type"`
			Delta    string                   `json:"delta"`
		}
		if jsonErr := json.Unmarshal([]byte(data), &ev); jsonErr != nil {
			return nil
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				return onDelta(provider.Delta{Content: ev.Delta})
			}
		case "response.completed":
			completed = ev.Response
		}
		return nil
	})
	if err != nil {
		t.Fatalf("postJSONStream error: %v", err)
	}

	if len(deltas) != 2 || deltas[0] != "Hello" || deltas[1] != " world" {
		t.Errorf("unexpected deltas: %v", deltas)
	}
	if completed == nil || len(completed.Output) == 0 {
		t.Fatalf("expected completed response, got nil or empty")
	}
	resp := responseFromRawSSE(completed)
	if resp.FinalText != "Hello world" {
		t.Errorf("expected FinalText=%q, got %q", "Hello world", resp.FinalText)
	}
}
