package provider

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3/responses"
)

func responseFromOpenAI(resp *responses.Response) Response {
	if resp == nil {
		return Response{}
	}
	result := Response{
		FinalText:   strings.TrimSpace(resp.OutputText()),
		OutputItems: outputItemsFromOpenAI(resp.Output),
		Usage: Usage{
			InputTokens:           int(resp.Usage.InputTokens),
			OutputTokens:          int(resp.Usage.OutputTokens),
			TotalTokens:           int(resp.Usage.TotalTokens),
			CacheReadInputTokens:  int(resp.Usage.InputTokensDetails.CachedTokens),
			ReasoningTokens:       int(resp.Usage.OutputTokensDetails.ReasoningTokens),
		},
	}
	result.PendingCalls = pendingCallsFromOpenAI(resp.Output)
	if len(result.PendingCalls) > 0 {
		result.OutputItems = append(result.OutputItems, assistantToolCallItems(result.PendingCalls)...)
		result.FinalText = ""
	}
	if len(result.OutputItems) == 0 && result.FinalText != "" {
		result.OutputItems = []ConversationItem{AssistantText(result.FinalText)}
	}
	return result
}

func pendingCallsFromOpenAI(items []responses.ResponseOutputItemUnion) []ToolInvocation {
	var calls []ToolInvocation
	for _, item := range items {
		switch item.Type {
		case "function_call":
			call := item.AsFunctionCall()
			calls = append(calls, openAIFunctionCall(call))
		case "custom_tool_call":
			call := item.AsCustomToolCall()
			calls = append(calls, ToolInvocation{
				Kind:   InvokeCustomTool,
				Name:   strings.TrimSpace(call.Name),
				Input:  strings.TrimSpace(call.Input),
				CallID: strings.TrimSpace(call.CallID),
			})
		}
	}
	return calls
}

func outputItemsFromOpenAI(items []responses.ResponseOutputItemUnion) []ConversationItem {
	var result []ConversationItem
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		message := item.AsMessage()
		text := strings.TrimSpace(openAIMessageText(message))
		if text == "" {
			continue
		}
		result = append(result, AssistantText(text))
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

func openAIFunctionCall(call responses.ResponseFunctionToolCall) ToolInvocation {
	arguments := strings.TrimSpace(call.Arguments)
	invocation := ToolInvocation{
		Kind:   InvokeCustomTool,
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

func openAIInvocationInput(arguments json.RawMessage) string {
	var parsed struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal(arguments, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Input)
}
