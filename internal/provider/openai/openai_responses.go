package openai

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	"github.com/Hoosk/motoko/internal/provider"
)

func responseFromOpenAI(resp *responses.Response) provider.Response {
	if resp == nil {
		return provider.Response{}
	}
	text := strings.TrimSpace(resp.OutputText())
	pending := pendingCallsFromOpenAI(resp.Output)
	reasoning := strings.TrimSpace(openAIReasoningText(resp.Output))
	result := provider.FinalizeResponse(text, reasoning, pending, provider.Usage{
		InputTokens:          int(resp.Usage.InputTokens),
		OutputTokens:         int(resp.Usage.OutputTokens),
		TotalTokens:          int(resp.Usage.TotalTokens),
		CacheReadInputTokens: int(resp.Usage.InputTokensDetails.CachedTokens),
		ReasoningTokens:      int(resp.Usage.OutputTokensDetails.ReasoningTokens),
	})
	if len(result.OutputItems) == 0 && result.FinalText != "" {
		result.OutputItems = []provider.ConversationItem{provider.AssistantText(result.FinalText)}
	}
	return result
}

func pendingCallsFromOpenAI(items []responses.ResponseOutputItemUnion) []provider.ToolInvocation {
	var calls []provider.ToolInvocation
	for _, item := range items {
		switch item.Type {
		case "function_call":
			call := item.AsFunctionCall()
			calls = append(calls, openAIFunctionCall(call))
		case "custom_tool_call":
			call := item.AsCustomToolCall()
			calls = append(calls, provider.ToolInvocation{
				Kind:   provider.InvokeCustomTool,
				Name:   strings.TrimSpace(call.Name),
				Input:  strings.TrimSpace(call.Input),
				CallID: strings.TrimSpace(call.CallID),
			})
		}
	}
	return calls
}

func openAIReasoningText(items []responses.ResponseOutputItemUnion) string {
	var parts []string
	for _, item := range items {
		if item.Type != "reasoning" {
			continue
		}
		reasoning := item.AsReasoning()
		for _, summary := range reasoning.Summary {
			text := strings.TrimSpace(summary.Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func openAIFunctionCall(call responses.ResponseFunctionToolCall) provider.ToolInvocation {
	arguments := strings.TrimSpace(call.Arguments)
	invocation := provider.ToolInvocation{
		Kind:   provider.InvokeCustomTool,
		Name:   strings.TrimSpace(call.Name),
		CallID: strings.TrimSpace(call.CallID),
		Raw:    json.RawMessage(call.RawJSON()),
	}
	if arguments == "" {
		return invocation
	}
	invocation.Arguments = json.RawMessage(arguments)
	invocation.Input = openAIInvocationInput(invocation.Arguments)
	if invocation.Input == "" {
		invocation.Input = arguments
	}
	return invocation
}

// ── Raw SSE types ────────────────────────────────────────────────────────────
// Used by streamResponses to parse the /responses SSE stream without relying
// on the openai-go SDK streamer, which cannot handle gateway keep-alive
// comment lines (": keep-alive") emitted by providers like OpenCode Zen.

// rawSSECompletedResponse is the "response" object embedded in a
// response.completed SSE event.
type rawSSECompletedResponse struct {
	Output []rawSSEOutputItem `json:"output"`
	Usage  rawSSEUsage        `json:"usage"`
}

type rawSSEOutputItem struct {
	Arguments string          `json:"arguments"`
	CallID    string          `json:"call_id"`
	Input     string          `json:"input"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Content   []rawSSEContent `json:"content"`
	Summary   []rawSSESummary `json:"summary"`
}

type rawSSEContent struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type rawSSESummary struct {
	Text string `json:"text"`
}

type rawSSEUsage struct {
	InputTokens         int64              `json:"input_tokens"`
	OutputTokens        int64              `json:"output_tokens"`
	TotalTokens         int64              `json:"total_tokens"`
	InputTokensDetails  rawSSETokenDetails `json:"input_tokens_details"`
	OutputTokensDetails rawSSETokenDetails `json:"output_tokens_details"`
}

type rawSSETokenDetails struct {
	CachedTokens    int64 `json:"cached_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

// responseFromRawSSE converts a parsed response.completed payload into a
// provider.Response. It handles message, function_call and reasoning output
// items using only standard Go JSON unmarshaling so it is immune to
// SDK-version changes.
func responseFromRawSSE(resp *rawSSECompletedResponse) provider.Response {
	if resp == nil {
		return provider.Response{}
	}

	var textParts []string
	var reasoningParts []string
	var pending []provider.ToolInvocation

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" && c.Text != "" {
					textParts = append(textParts, c.Text)
				}
			}
		case "reasoning":
			for _, s := range item.Summary {
				if s.Text != "" {
					reasoningParts = append(reasoningParts, s.Text)
				}
			}
		case "function_call":
			args := strings.TrimSpace(item.Arguments)
			inv := provider.ToolInvocation{
				Kind:   provider.InvokeCustomTool,
				Name:   strings.TrimSpace(item.Name),
				CallID: strings.TrimSpace(item.CallID),
			}
			if args != "" {
				inv.Arguments = json.RawMessage(args)
				inv.Input = openAIInvocationInput(inv.Arguments)
				if inv.Input == "" {
					inv.Input = args
				}
			}
			pending = append(pending, inv)
		case "custom_tool_call":
			input := strings.TrimSpace(item.Input)
			if input == "" {
				input = strings.TrimSpace(item.Arguments)
			}
			pending = append(pending, provider.ToolInvocation{
				Kind:   provider.InvokeCustomTool,
				Name:   strings.TrimSpace(item.Name),
				CallID: strings.TrimSpace(item.CallID),
				Input:  input,
			})
		}
	}

	text := strings.TrimSpace(strings.Join(textParts, ""))
	reasoning := strings.TrimSpace(strings.Join(reasoningParts, "\n"))

	return provider.FinalizeResponse(text, reasoning, pending, provider.Usage{
		InputTokens:          int(resp.Usage.InputTokens),
		OutputTokens:         int(resp.Usage.OutputTokens),
		TotalTokens:          int(resp.Usage.TotalTokens),
		CacheReadInputTokens: int(resp.Usage.InputTokensDetails.CachedTokens),
		ReasoningTokens:      int(resp.Usage.OutputTokensDetails.ReasoningTokens),
	})
}
