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

const testModelID = "claude-test-model"

// newLimitsTestClient builds a client whose HTTP layer answers the models
// endpoint with modelJSON (404 when empty) and any other request with
// handler's response. Request bodies of non-model calls are captured in *bodies.
func newLimitsTestClient(t *testing.T, thinkingBudget int, modelJSON string, handler func() (string, string), bodies *[]string) *anthropicClient {
	t.Helper()
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		if strings.HasSuffix(req.URL.Path, "/v1/models/"+testModelID) {
			if modelJSON == "" {
				return &http.Response{StatusCode: 404, Header: headers, Body: ioNopCloser{bytes.NewBufferString("not found")}}, nil
			}
			headers.Set("Content-Type", "application/json")
			return &http.Response{StatusCode: 200, Header: headers, Body: ioNopCloser{bytes.NewBufferString(modelJSON)}}, nil
		}
		if bodies != nil && req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			*bodies = append(*bodies, string(b))
		}
		contentType, body := handler()
		headers.Set("Content-Type", contentType)
		return &http.Response{StatusCode: 200, Header: headers, Body: ioNopCloser{bytes.NewBufferString(body)}}, nil
	})}
	client := NewClient(config.ProviderConfig{
		Preset:         config.ProviderPresetAnthropic,
		BaseURL:        "http://api.example.com",
		APIKey:         "test-key",
		Model:          testModelID,
		ThinkingBudget: thinkingBudget,
	})
	c := client.(*anthropicClient)
	sdkClient := sdk.NewClient(
		option.WithAPIKey(c.APIKey()),
		option.WithBaseURL(c.BaseURL()),
		option.WithHTTPClient(httpClient),
	)
	c.sdkClient = &sdkClient
	return c
}

func modelJSON(maxTokens int, adaptive bool) string {
	a := "false"
	if adaptive {
		a = "true"
	}
	return `{"id":"` + testModelID + `","max_tokens":` + itoa(maxTokens) + `,"capabilities":{"thinking":{"supported":true,"types":{"adaptive":{"supported":` + a + `}}}}}`
}

func itoa(n int) string {
	return strings.TrimSpace(strings.Join(strings.Fields(strings.Repeat(" ", 0)+fmtInt(n)), ""))
}

func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for ; n > 0; n /= 10 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
	}
	return string(digits)
}

func noHandler() (string, string) { return "application/json", "{}" }

func TestBuildMessageParamsUsesModelMaxOutputTokens(t *testing.T) {
	c := newLimitsTestClient(t, 0, modelJSON(64000, true), noHandler, nil)
	params, _ := c.buildMessageParams(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{})
	if params.MaxTokens != 64000 {
		t.Fatalf("expected max_tokens 64000 from model metadata, got %d", params.MaxTokens)
	}
}

func TestBuildMessageParamsFallsBackWhenModelLimitUnknown(t *testing.T) {
	c := newLimitsTestClient(t, 0, "", noHandler, nil)
	params, _ := c.buildMessageParams(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{})
	if params.MaxTokens != defaultMaxOutputTokens {
		t.Fatalf("expected fallback %d, got %d", defaultMaxOutputTokens, params.MaxTokens)
	}
}

func TestBuildMessageParamsKeepsMaxTokensAboveThinkingBudget(t *testing.T) {
	// Unknown limit: max_tokens grows past the configured budget.
	c := newLimitsTestClient(t, 20000, "", noHandler, nil)
	params, _ := c.buildMessageParams(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{})
	if params.MaxTokens <= 20000 {
		t.Fatalf("max_tokens %d must exceed the configured thinking budget 20000", params.MaxTokens)
	}

	// Known small limit: the budget is clamped below max_tokens.
	c = newLimitsTestClient(t, 20000, modelJSON(8192, false), noHandler, nil)
	params, _ = c.buildMessageParams(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{})
	if params.MaxTokens != 8192 {
		t.Fatalf("expected model limit 8192, got %d", params.MaxTokens)
	}
	if params.Thinking.OfEnabled == nil || params.Thinking.OfEnabled.BudgetTokens >= params.MaxTokens {
		t.Fatalf("thinking budget must stay below max_tokens, got %#v", params.Thinking.OfEnabled)
	}
}

func TestCompleteCapsMaxTokensForNonStreamingGuard(t *testing.T) {
	const msg = `{"id":"m","type":"message","role":"assistant","model":"x","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	var bodies []string
	c := newLimitsTestClient(t, 0, modelJSON(128000, true), func() (string, string) { return "application/json", msg }, &bodies)
	resp, err := c.Complete(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{})
	if err != nil {
		t.Fatalf("Complete must pass the SDK non-streaming guard: %v", err)
	}
	if resp.FinalText != "ok" {
		t.Fatalf("unexpected text %q", resp.FinalText)
	}
	if len(bodies) != 1 || !strings.Contains(bodies[0], `"max_tokens":21333`) {
		t.Fatalf("expected max_tokens capped at 21333, got %v", bodies)
	}
}

func messageWithBlocks(t *testing.T, stopReason string, blocks ...string) *sdk.Message {
	t.Helper()
	raw := `{"id":"m","type":"message","role":"assistant","model":"x","stop_reason":"` + stopReason + `","content":[` + strings.Join(blocks, ",") + `],"usage":{"input_tokens":1,"output_tokens":1}}`
	var msg sdk.Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	return &msg
}

const (
	toolUseBlock = `{"type":"tool_use","id":"call_1","name":"write","input":{"path":"a.go"}}`
	textBlock    = `{"type":"text","text":"cut off mid-sent"}`
)

func TestResponseFromSDKRejectsTruncatedToolCall(t *testing.T) {
	// A tool_use that is the last block at max_tokens is rejected even though
	// its (partial) input parses as valid JSON.
	_, err := responseFromSDK(messageWithBlocks(t, "max_tokens", textBlock, toolUseBlock))
	if err == nil || !strings.Contains(err.Error(), `"write"`) {
		t.Fatalf("expected truncation error naming the tool, got %v", err)
	}

	// A complete tool call followed by truncated text is accepted.
	resp, err := responseFromSDK(messageWithBlocks(t, "max_tokens", toolUseBlock, textBlock))
	if err != nil {
		t.Fatalf("tool_use followed by text at max_tokens must be accepted: %v", err)
	}
	if len(resp.PendingCalls) != 1 {
		t.Fatalf("expected 1 pending call, got %d", len(resp.PendingCalls))
	}

	// The same call at a normal tool_use stop is accepted.
	if _, err := responseFromSDK(messageWithBlocks(t, "tool_use", toolUseBlock)); err != nil {
		t.Fatalf("tool_use stop must be accepted: %v", err)
	}
}

func TestResponseFromSDKRejectsToolCallsOfRefusal(t *testing.T) {
	_, err := responseFromSDK(messageWithBlocks(t, "refusal", textBlock, toolUseBlock))
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("expected refusal error, got %v", err)
	}
	if _, err := responseFromSDK(messageWithBlocks(t, "refusal", textBlock)); err != nil {
		t.Fatalf("refusal without tool calls must be accepted: %v", err)
	}
}

func streamBody(stopReason, partialJSON string) string {
	events := []string{
		`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"x","usage":{"input_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"write","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` + quote(partialJSON) + `}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"` + stopReason + `"},"usage":{"input_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":200,"output_tokens":50}}`,
		`{"type":"message_stop"}`,
	}
	var sb strings.Builder
	for _, e := range events {
		typ := e[len(`{"type":"`):]
		typ = typ[:strings.Index(typ, `"`)]
		sb.WriteString("event: " + typ + "\ndata: " + e + "\n\n")
	}
	return sb.String()
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func TestStreamCompleteRejectsTruncatedToolCall(t *testing.T) {
	body := streamBody("max_tokens", `{"path":"a.go","content":"pack`)
	c := newLimitsTestClient(t, 0, "", func() (string, string) { return "text/event-stream", body }, nil)
	_, err := c.StreamComplete(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{}, nil)
	if err == nil || !strings.Contains(err.Error(), `"write"`) {
		t.Fatalf("expected truncation error naming the tool, got %v", err)
	}
}

func TestStreamCompleteRejectsCompleteLookingToolCallAtMaxTokens(t *testing.T) {
	body := streamBody("max_tokens", `{"path":"a.go"}`)
	c := newLimitsTestClient(t, 0, "", func() (string, string) { return "text/event-stream", body }, nil)
	_, err := c.StreamComplete(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{}, nil)
	if err == nil || !strings.Contains(err.Error(), `"write"`) {
		t.Fatalf("expected truncation error naming the tool, got %v", err)
	}
}

func TestStreamCompleteAcceptsToolCallFollowedByTextAtMaxTokens(t *testing.T) {
	events := []string{
		`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"x","usage":{"input_tokens":10,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"write","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a.go\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"cut"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":50}}`,
		`{"type":"message_stop"}`,
	}
	var sb strings.Builder
	for _, e := range events {
		typ := e[len(`{"type":"`):]
		typ = typ[:strings.Index(typ, `"`)]
		sb.WriteString("event: " + typ + "\ndata: " + e + "\n\n")
	}
	body := sb.String()
	c := newLimitsTestClient(t, 0, "", func() (string, string) { return "text/event-stream", body }, nil)
	resp, err := c.StreamComplete(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{}, nil)
	if err != nil {
		t.Fatalf("complete tool call followed by text must be accepted: %v", err)
	}
	if len(resp.PendingCalls) != 1 {
		t.Fatalf("expected 1 pending call, got %d", len(resp.PendingCalls))
	}
}

func TestStreamCompleteCountsCachedTokensInInput(t *testing.T) {
	body := streamBody("tool_use", `{"path":"a.go"}`)
	c := newLimitsTestClient(t, 0, "", func() (string, string) { return "text/event-stream", body }, nil)
	resp, err := c.StreamComplete(context.Background(), "sys", []provider.ConversationItem{provider.UserText("hi")}, provider.ToolSet{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	u := resp.Usage
	if u.InputTokens != 1210 || u.CacheReadInputTokens != 1000 || u.CacheWriteInputTokens != 200 || u.OutputTokens != 50 || u.TotalTokens != 1260 {
		t.Fatalf("unexpected usage: %#v", u)
	}
}

func TestResponseFromSDKCountsCachedTokensInInput(t *testing.T) {
	msg := &sdk.Message{Content: []sdk.ContentBlockUnion{{Type: "text", Text: "ok"}}}
	msg.Usage.InputTokens = 10
	msg.Usage.CacheReadInputTokens = 1000
	msg.Usage.CacheCreationInputTokens = 200
	msg.Usage.OutputTokens = 50
	resp, err := responseFromSDK(msg)
	if err != nil {
		t.Fatal(err)
	}
	u := resp.Usage
	if u.InputTokens != 1210 || u.CacheReadInputTokens != 1000 || u.CacheWriteInputTokens != 200 || u.TotalTokens != 1260 {
		t.Fatalf("unexpected usage: %#v", u)
	}
}
