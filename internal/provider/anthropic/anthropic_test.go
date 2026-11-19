package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type ioNopCloser struct {
	io.Reader
}

func (ioNopCloser) Close() error { return nil }

func TestToSDKMessagesSkipsSystemAndNormalizesUnknownRole(t *testing.T) {
	got := toSDKMessages([]provider.ConversationItem{{Role: provider.RoleSystem, Content: "sys"}, {Role: "weird", Content: "hola"}, {Role: provider.RoleAssistant, Content: "ok"}})
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Role != sdk.MessageParamRoleUser || got[0].Content[0].OfText.Text != "hola" {
		t.Fatalf("unexpected first message: %#v", got[0])
	}
	if got[1].Role != sdk.MessageParamRoleAssistant || got[1].Content[0].OfText.Text != "ok" {
		t.Fatalf("unexpected second message: %#v", got[1])
	}
}

func TestResponseFromSDKJoinsTextParts(t *testing.T) {
	sdkMsg := &sdk.Message{
		Content: []sdk.ContentBlockUnion{
			{Type: "text", Text: "uno"},
			{Type: "tool_use", ID: "call_1", Name: "bash", Input: json.RawMessage(`{"input":"ls"}`)},
			{Type: "text", Text: "dos"},
		},
	}
	resp := responseFromSDK(sdkMsg)
	if resp.FinalText != "" {
		t.Fatalf("expected empty final text since tool use is present, got %q", resp.FinalText)
	}
	sdkMsgTextOnly := &sdk.Message{
		Content: []sdk.ContentBlockUnion{
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
	messages := []provider.ConversationItem{
		provider.UserText("hello"),
		provider.AssistantText("world"),
	}
	got := toSDKMessages(messages)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Role != sdk.MessageParamRoleUser || got[0].Content[0].OfText.Text != "hello" {
		t.Fatalf("unexpected user message: %#v", got[0])
	}
	if got[1].Role != sdk.MessageParamRoleAssistant || got[1].Content[0].OfText.Text != "world" {
		t.Fatalf("unexpected assistant message: %#v", got[1])
	}

	call := provider.ToolInvocation{
		Kind:   provider.InvokeCustomTool,
		Name:   "bash",
		Input:  "ls",
		CallID: "call_abc",
	}
	messages = []provider.ConversationItem{
		provider.AssistantTurn("", "", []provider.ToolInvocation{call}),
	}
	got = toSDKMessages(messages)
	if len(got) != 1 || got[0].Role != sdk.MessageParamRoleAssistant {
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

	resultItem := provider.ToolResultForInvocation(call, "result_text")
	messages = []provider.ConversationItem{resultItem}
	got = toSDKMessages(messages)
	if len(got) != 1 || got[0].Role != sdk.MessageParamRoleUser {
		t.Fatalf("expected 1 user message, got %#v", got)
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

func TestToSDKMessagesSetsCacheControlOnTurnBoundaryAndLastMessage(t *testing.T) {
	assertTurnBoundaryCacheControl(
		t,
		toSDKMessages(toolUseCacheControlMessages()),
		func(msg sdk.MessageParam) sdk.CacheControlEphemeralParam { return msg.Content[0].OfText.CacheControl },
		func(msg sdk.MessageParam) bool {
			return msg.Content[0].OfToolUse != nil && msg.Content[0].OfToolUse.CacheControl != (sdk.CacheControlEphemeralParam{})
		},
	)
}

func TestToSDKBetaMessagesSetsCacheControlOnTurnBoundaryAndLastMessage(t *testing.T) {
	assertTurnBoundaryCacheControl(
		t,
		toSDKBetaMessages(toolUseCacheControlMessages()),
		func(msg sdk.BetaMessageParam) sdk.BetaCacheControlEphemeralParam {
			return msg.Content[0].OfText.CacheControl
		},
		func(msg sdk.BetaMessageParam) bool {
			return msg.Content[0].OfToolUse != nil && msg.Content[0].OfToolUse.CacheControl != (sdk.BetaCacheControlEphemeralParam{})
		},
	)
}

func toolUseCacheControlMessages() []provider.ConversationItem {
	call := provider.ToolInvocation{
		Kind:   provider.InvokeCustomTool,
		Name:   "bash",
		Input:  "ls",
		CallID: "call_abc",
	}
	return []provider.ConversationItem{
		provider.UserText("first"),
		provider.AssistantTurn("", "", []provider.ToolInvocation{call}),
		provider.UserText("last"),
	}
}

// assertTurnBoundaryCacheControl checks [user, assistant tool_use, user]: the
// tool_use right before the latest user turn and the last message are marked,
// the first message (no turn before it) is not.
func assertTurnBoundaryCacheControl[Msg any, Cache comparable](t *testing.T, got []Msg, firstCache func(Msg) Cache, secondHasToolUseCache func(Msg) bool) {
	t.Helper()
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	var zero Cache
	if firstCache(got[0]) != zero {
		t.Fatalf("expected first message cache control to be empty, got %#v", firstCache(got[0]))
	}
	if !secondHasToolUseCache(got[1]) {
		t.Fatalf("expected turn-boundary tool_use cache control, got %#v", got[1])
	}
	if firstCache(got[2]) == zero {
		t.Fatalf("expected last message cache control to be set, got %#v", got[2])
	}
}

func agentLoopMessages() []provider.ConversationItem {
	call1 := provider.ToolInvocation{Kind: provider.InvokeCustomTool, Name: "bash", Input: "ls", CallID: "call_1"}
	call2 := provider.ToolInvocation{Kind: provider.InvokeCustomTool, Name: "bash", Input: "pwd", CallID: "call_2"}
	return []provider.ConversationItem{
		provider.UserText("turn 1"),                                      // 0
		provider.AssistantText("answer 1"),                               // 1 <- before previous turn
		provider.UserText("turn 2"),                                      // 2
		provider.AssistantTurn("", "", []provider.ToolInvocation{call1}), // 3
		provider.ToolResultForInvocation(call1, "out 1"),                 // 4
		provider.AssistantText("answer 2"),                               // 5 <- before current turn
		provider.UserText("<environment>...</context>\n\nturn 3"),        // 6
		provider.AssistantTurn("", "", []provider.ToolInvocation{call2}), // 7
		provider.ToolResultForInvocation(call2, "out 2"),                 // 8 <- last
	}
}

func TestMessageCacheBreakpointsAgentLoop(t *testing.T) {
	got := messageCacheBreakpoints(agentLoopMessages())
	want := map[int]bool{1: true, 5: true, 8: true}
	for i, marked := range got {
		if marked != want[i] {
			t.Fatalf("message %d: expected breakpoint=%t, got %t (all: %v)", i, want[i], marked, got)
		}
	}
	// Tool results and tool calls are not turn starts.
	if isTurnStart(agentLoopMessages()[4]) || isTurnStart(agentLoopMessages()[3]) {
		t.Fatal("tool traffic must not count as a turn start")
	}
	if len(messageCacheBreakpoints(nil)) != 0 {
		t.Fatal("expected no breakpoints for empty messages")
	}
	if single := messageCacheBreakpoints([]provider.ConversationItem{provider.UserText("hi")}); !single[0] {
		t.Fatal("expected the only message to be marked")
	}
}

// TestBuildMessageParamsStaysWithinBreakpointLimit guards the API limit of 4
// cache_control breakpoints per request (system + 3 message marks, no tools).
func TestBuildMessageParamsStaysWithinBreakpointLimit(t *testing.T) {
	client := NewClient(config.ProviderConfig{
		Preset:  config.ProviderPresetAnthropic,
		BaseURL: "http://api.example.com",
		APIKey:  "test-key",
		Model:   "claude-test-model",
	}).(*anthropicClient)
	tools := provider.ToolSet{Local: []provider.LocalToolDefinition{
		{Name: "bash", Description: "run", InputHint: "cmd"},
		{Name: "read", Description: "read", InputHint: "path"},
	}}
	params, _ := client.buildMessageParams(context.Background(), "system prompt", agentLoopMessages(), tools)
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(body), `"cache_control"`); n != 4 {
		t.Fatalf("expected exactly 4 cache_control breakpoints, got %d: %s", n, body)
	}
	for _, tool := range params.Tools {
		if tool.OfTool.CacheControl != (sdk.CacheControlEphemeralParam{}) {
			t.Fatalf("expected no tool-level breakpoint, got %#v", tool.OfTool)
		}
	}
	if params.System[0].CacheControl == (sdk.CacheControlEphemeralParam{}) {
		t.Fatal("expected system prompt breakpoint")
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
	var sdkMsg sdk.Message
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
	tools := provider.ToolSet{
		Local: []provider.LocalToolDefinition{
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

func TestToSDKToolsUsesRawSchema(t *testing.T) {
	raw := []byte(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "file to read"},
			"limit": {"type": "integer"}
		},
		"required": ["path"]
	}`)
	tools := provider.ToolSet{
		Local: []provider.LocalToolDefinition{
			{Name: "read", Description: "read a file", Schema: raw},
		},
	}
	serialized := toSDKTools(tools)
	if len(serialized) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(serialized))
	}
	tool := serialized[0].OfTool
	if tool == nil {
		t.Fatal("expected OfTool")
	}
	if got := tool.InputSchema.Required; len(got) != 1 || got[0] != "path" {
		t.Errorf("expected required [path], got %v", got)
	}
	props, ok := tool.InputSchema.Properties.(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", tool.InputSchema.Properties)
	}
	if _, ok := props["limit"]; !ok {
		t.Errorf("expected limit property to survive, got %v", props)
	}
	if _, ok := props["path"]; !ok {
		t.Errorf("expected path property to survive, got %v", props)
	}
	// additionalProperties-style extras must survive via ExtraFields.
	body, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"required":["path"]`) && !strings.Contains(string(body), `"required": ["path"]`) {
		t.Errorf("expected required path in serialized schema, got %s", string(body))
	}
}

func TestAnthropicClientCheckAdaptiveThinking(t *testing.T) {
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

	client := NewClient(config.ProviderConfig{
		Preset:  config.ProviderPresetAnthropic,
		BaseURL: "http://api.example.com",
		APIKey:  "test-key",
		Model:   "claude-test-model",
	})
	aClient := client.(*anthropicClient)
	sdkClient := sdk.NewClient(
		option.WithAPIKey(aClient.APIKey()),
		option.WithBaseURL(aClient.BaseURL()),
		option.WithHTTPClient(httpClient),
	)
	aClient.sdkClient = &sdkClient

	isAdaptive := aClient.checkAdaptiveThinking(context.Background())
	if !isAdaptive {
		t.Fatal("expected model to support adaptive thinking")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	isAdaptive2 := aClient.checkAdaptiveThinking(context.Background())
	if !isAdaptive2 {
		t.Fatal("expected cached value to be true")
	}
	if callCount != 1 {
		t.Fatalf("expected call count to remain 1, got %d", callCount)
	}

	clientFallback := NewClient(config.ProviderConfig{
		Preset:  config.ProviderPresetAnthropic,
		BaseURL: "http://api.example.com",
		APIKey:  "test-key",
		Model:   "claude-opus-4-7",
	})
	aClientFallback := clientFallback.(*anthropicClient)
	fallbackHTTPClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       ioNopCloser{bytes.NewBufferString("error")},
		}, nil
	})}
	sdkClientFallback := sdk.NewClient(
		option.WithAPIKey(aClientFallback.APIKey()),
		option.WithBaseURL(aClientFallback.BaseURL()),
		option.WithHTTPClient(fallbackHTTPClient),
	)
	aClientFallback.sdkClient = &sdkClientFallback

	if !aClientFallback.checkAdaptiveThinking(context.Background()) {
		t.Fatal("expected fallback to detect adaptive thinking")
	}
}
