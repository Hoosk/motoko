package provider

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

type chatCompletionResponse struct {
	Choices []chatCompletionChoice `json:"choices"`
	Usage   chatCompletionUsage    `json:"usage"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
}

func (u chatCompletionUsage) providerUsage() Usage {
	input := u.InputTokens
	if input == 0 {
		input = u.PromptTokens
	}
	output := u.OutputTokens
	if output == 0 {
		output = u.CompletionTokens
	}
	total := u.TotalTokens
	if total == 0 {
		total = input + output
	}
	return Usage{InputTokens: input, OutputTokens: output, TotalTokens: total}
}

type chatCompletionChoice struct {
	Message chatCompletionMessage `json:"message"`
	Delta   chatCompletionDelta   `json:"delta"`
}

type chatCompletionMessage struct {
	Content          string                   `json:"content"`
	ReasoningContent string                   `json:"reasoning_content"`
	ToolCalls        []chatCompletionToolCall `json:"tool_calls"`
}

type chatCompletionDelta struct {
	Content          string                        `json:"content"`
	ReasoningContent string                        `json:"reasoning_content"`
	ToolCalls        []chatCompletionToolCallDelta `json:"tool_calls"`
}

type chatCompletionToolCall struct {
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
	Function chatCompletionToolFunction `json:"function"`
	Raw      json.RawMessage            `json:"-"`
}

func (c *chatCompletionToolCall) UnmarshalJSON(data []byte) error {
	type alias chatCompletionToolCall
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = chatCompletionToolCall(decoded)
	c.Raw = append(c.Raw[:0], data...)
	return nil
}

type chatCompletionToolCallDelta struct {
	Index    int                        `json:"index"`
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
	Function chatCompletionToolFunction `json:"function"`
}

type chatCompletionToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func responseFromChatCompletion(decoded chatCompletionResponse) Response {
	if len(decoded.Choices) == 0 {
		return Response{}
	}
	message := decoded.Choices[0].Message
	text := strings.TrimSpace(message.Content)
	result := Response{FinalText: text, Usage: decoded.Usage.providerUsage()}
	if text != "" {
		result.OutputItems = []ConversationItem{AssistantText(text)}
	}
	result.PendingCalls = pendingCallsFromChatToolCalls(message.ToolCalls)
	if len(result.PendingCalls) > 0 {
		result.OutputItems = append(result.OutputItems, assistantToolCallItems(result.PendingCalls)...)
		result.FinalText = ""
	}
	return result
}

func pendingCallsFromChatToolCalls(toolCalls []chatCompletionToolCall) []ToolInvocation {
	result := make([]ToolInvocation, 0, len(toolCalls))
	for _, call := range toolCalls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		arguments := strings.TrimSpace(call.Function.Arguments)
		invocation := ToolInvocation{
			Kind:   InvokeCustomTool,
			Name:   strings.TrimSpace(call.Function.Name),
			CallID: strings.TrimSpace(call.ID),
			Raw:    append(json.RawMessage(nil), call.Raw...),
		}
		if arguments != "" {
			invocation.Arguments = json.RawMessage(arguments)
			invocation.Input = openAIInvocationInput(invocation.Arguments)
			if invocation.Input == "" {
				invocation.Input = arguments
			}
		}
		result = append(result, invocation)
	}
	return result
}

func toChatMessages(messages []ConversationItem) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		if call, ok := parseAssistantToolCallContent(msg.Content); ok {
			if len(call.Raw) > 0 {
				var rawToolCall map[string]any
				if err := json.Unmarshal(call.Raw, &rawToolCall); err == nil {
					result = append(result, map[string]any{
						"role":       RoleAssistant,
						"content":    "",
						"tool_calls": []map[string]any{rawToolCall},
					})
					continue
				}
			}
			result = append(result, map[string]any{
				"role":    RoleAssistant,
				"content": "",
				"tool_calls": []map[string]any{{
					"id":   call.CallID,
					"type": "function",
					"function": map[string]any{
						"name":      call.Name,
						"arguments": assistantToolCallArguments(call),
					},
				}},
			})
			continue
		}
		if msg.Role == RoleTool {
			call, output := parseToolResultContent(msg.Content)
			item := map[string]any{
				"role":    RoleTool,
				"content": output,
			}
			if call.CallID != "" {
				item["tool_call_id"] = call.CallID
			}
			if call.Name != "" {
				item["name"] = call.Name
			}
			result = append(result, item)
			continue
		}
		result = append(result, map[string]any{
			"role":    normalizeConversationRole(msg.Role),
			"content": msg.Content,
		})
	}
	return result
}

func toResponsesInputItems(messages []ConversationItem) responses.ResponseInputParam {
	items := make(responses.ResponseInputParam, 0, len(messages))
	for _, msg := range messages {
		if call, ok := parseAssistantToolCallContent(msg.Content); ok && call.CallID != "" {
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(assistantToolCallArguments(call), call.CallID, call.Name))
			continue
		}
		if msg.Role == RoleTool {
			call, output := parseToolResultContent(msg.Content)
			if call.CallID != "" {
				items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, output))
				continue
			}
		}
		role := responses.EasyInputMessageRole(normalizeConversationRole(msg.Role))
		items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, role))
	}
	return items
}

func responseTools(tools ToolSet) []responses.ToolUnionParam {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]responses.ToolUnionParam, 0, len(tools.Local))
	for _, tool := range tools.Local {
		parameters := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": toolInputDescription(tool),
				},
			},
			"required":             []string{"input"},
			"additionalProperties": false,
		}
		result = append(result, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
			Name:        tool.Name,
			Description: param.NewOpt(strings.TrimSpace(tool.Description)),
			Parameters:  parameters,
			Strict:      param.NewOpt(true),
		}})
	}
	return result
}

func chatCompletionTools(tools ToolSet) []map[string]any {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(tools.Local))
	for _, tool := range tools.Local {
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": strings.TrimSpace(tool.Description),
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"input": map[string]any{
							"type":        "string",
							"description": toolInputDescription(tool),
						},
					},
					"required":             []string{"input"},
					"additionalProperties": false,
				},
			},
		})
	}
	return result
}

func toolInputDescription(tool LocalToolDefinition) string {
	if hint := strings.TrimSpace(tool.InputHint); hint != "" {
		return hint
	}
	if desc := strings.TrimSpace(tool.Description); desc != "" {
		return desc
	}
	return "Raw text input for the tool."
}

func mergeChatToolCallDeltas(acc map[int]*chatCompletionToolCall, deltas []chatCompletionToolCallDelta) {
	for _, delta := range deltas {
		call := acc[delta.Index]
		if call == nil {
			call = &chatCompletionToolCall{}
			acc[delta.Index] = call
		}
		if delta.ID != "" {
			call.ID = delta.ID
		}
		if delta.Type != "" {
			call.Type = delta.Type
		}
		if delta.Function.Name != "" {
			call.Function.Name += delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			call.Function.Arguments += delta.Function.Arguments
		}
	}
}

func sortedChatToolCalls(acc map[int]*chatCompletionToolCall) []chatCompletionToolCall {
	if len(acc) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(acc))
	for index := range acc {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	result := make([]chatCompletionToolCall, 0, len(indexes))
	for _, index := range indexes {
		if call := acc[index]; call != nil {
			result = append(result, *call)
		}
	}
	return result
}
