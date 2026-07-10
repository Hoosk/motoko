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
