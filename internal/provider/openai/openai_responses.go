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
	result := provider.Response{
		FinalText:   strings.TrimSpace(resp.OutputText()),
		OutputItems: outputItemsFromOpenAI(resp.Output),
		Usage: provider.Usage{
			InputTokens:          int(resp.Usage.InputTokens),
			OutputTokens:         int(resp.Usage.OutputTokens),
			TotalTokens:          int(resp.Usage.TotalTokens),
			CacheReadInputTokens: int(resp.Usage.InputTokensDetails.CachedTokens),
			ReasoningTokens:      int(resp.Usage.OutputTokensDetails.ReasoningTokens),
		},
	}
	result.PendingCalls = pendingCallsFromOpenAI(resp.Output)
	if len(result.PendingCalls) > 0 {
		result.OutputItems = append(result.OutputItems, provider.AssistantToolCallItems(result.PendingCalls)...)
		result.FinalText = ""
	}
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

func outputItemsFromOpenAI(items []responses.ResponseOutputItemUnion) []provider.ConversationItem {
	var result []provider.ConversationItem
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		message := item.AsMessage()
		text := strings.TrimSpace(openAIMessageText(message))
		if text == "" {
			continue
		}
		result = append(result, provider.AssistantText(text))
	}
	return result
}

func openAIMessageText(message responses.ResponseOutputMessage) string {
	var parts []string
	for _, content := range message.Content {
		switch content.Type {
		case "output_text":
			text := strings.TrimSpace(content.Text)
			if text != "" {
				parts = append(parts, text)
			}
		case "refusal":
			text := strings.TrimSpace(content.Refusal)
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
