package provider

import (
	"encoding/json"
	"sort"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type completionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type chatCompletionResponse struct {
	Choices []chatCompletionChoice `json:"choices"`
	Usage   chatCompletionUsage    `json:"usage"`
}

type chatCompletionUsage struct {
	PromptTokensDetails     *promptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *completionTokensDetails `json:"completion_tokens_details,omitempty"`
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	InputTokens             int                      `json:"input_tokens"`
	OutputTokens            int                      `json:"output_tokens"`
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
	usage := Usage{InputTokens: input, OutputTokens: output, TotalTokens: total}
	if u.PromptTokensDetails != nil {
		usage.CacheReadInputTokens = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		usage.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return usage
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
	RawMap   map[string]any             `json:"-"`
	Function chatCompletionToolFunction `json:"function"`
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
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
	_ = json.Unmarshal(data, &c.RawMap)
	return nil
}

type chatCompletionToolCallDelta struct {
	Function chatCompletionToolFunction `json:"function"`
	ID       string                     `json:"id"`
	Type     string                     `json:"type"`
	Raw      json.RawMessage            `json:"-"`
	Index    int                        `json:"index"`
}

func (c *chatCompletionToolCallDelta) UnmarshalJSON(data []byte) error {
	type alias chatCompletionToolCallDelta
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = chatCompletionToolCallDelta(decoded)
	c.Raw = append(c.Raw[:0], data...)
	return nil
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
						keyRole:      RoleAssistant,
						keyContent:   "",
						"tool_calls": []map[string]any{rawToolCall},
					})
					continue
				}
			}

			result = append(result, map[string]any{
				keyRole:    RoleAssistant,
				keyContent: "",
				"tool_calls": []map[string]any{{
					"id":        call.CallID,
					schemaType:  keyFunction,
					keyFunction: map[string]any{
						keyName:     call.Name,
						"arguments": assistantToolCallArguments(call),
					},
				}},
			})
			continue
		}
		if msg.Role == RoleTool {
			call, output := parseToolResultContent(msg.Content)
			item := map[string]any{
				keyRole:    RoleTool,
				keyContent: output,
			}
			if call.CallID != "" {
				item["tool_call_id"] = call.CallID
			}
			if call.Name != "" {
				item[keyName] = call.Name
			}
			result = append(result, item)
			continue
		}
		result = append(result, map[string]any{
			keyRole:    normalizeConversationRole(msg.Role),
			keyContent: msg.Content,
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
			schemaType: schemaObject,
			schemaProperties: map[string]any{
				schemaInput: map[string]any{
					"type":            schemaString,
					schemaDescription: toolInputDescription(tool),
				},
			},
			schemaRequired:             []string{schemaInput},
			schemaAdditionalProperties: false,
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
			schemaType: keyFunction,
			keyFunction: map[string]any{
				keyName:           tool.Name,
				schemaDescription: strings.TrimSpace(tool.Description),
				"parameters": map[string]any{
					schemaType: schemaObject,
					schemaProperties: map[string]any{
						schemaInput: map[string]any{
							schemaType:        schemaString,
							schemaDescription: toolInputDescription(tool),
						},
					},
					schemaRequired:             []string{schemaInput},
					schemaAdditionalProperties: false,
				},
			},
		})
	}
	return result
}

func toolInputDescription(tool LocalToolDefinition) string {
	hint := strings.TrimSpace(tool.InputHint)
	if hint != "" {
		prefix := tool.Name + " "
		if strings.HasPrefix(strings.ToLower(hint), strings.ToLower(prefix)) {
			hint = strings.TrimSpace(hint[len(prefix):])
		}
		return hint
	}
	if desc := strings.TrimSpace(tool.Description); desc != "" {
		return desc
	}
	return "Raw text input for the tool."
}

func mergeChatToolCallDeltas(acc map[int]*chatCompletionToolCall, deltas []chatCompletionToolCallDelta, mappedIndexes map[int]int) {
	if mappedIndexes == nil {
		mappedIndexes = make(map[int]int)
	}
	for _, delta := range deltas {
		targetIndex := delta.Index
		if delta.ID != "" {
			existing, exists := acc[delta.Index]
			if exists && existing.ID != "" && existing.ID != delta.ID {
				newIndex := len(acc)
				mappedIndexes[delta.Index] = newIndex
				targetIndex = newIndex
			} else {
				mappedIndexes[delta.Index] = delta.Index
				targetIndex = delta.Index
			}
		} else {
			if mapped, ok := mappedIndexes[delta.Index]; ok {
				targetIndex = mapped
			}
		}

		call := acc[targetIndex]
		if call == nil {
			call = &chatCompletionToolCall{
				RawMap: make(map[string]any),
			}
			acc[targetIndex] = call
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

		if len(delta.Raw) > 0 {
			var deltaMap map[string]any
			if err := json.Unmarshal(delta.Raw, &deltaMap); err == nil {
				mergeMaps(call.RawMap, deltaMap)
			}
		}
	}
}

func mergeMaps(dest, src map[string]any) {
	for k, v := range src {
		if k == "index" {
			continue
		}
		if srcMap, ok := v.(map[string]any); ok {
			destMap, destOk := dest[k].(map[string]any)
			if !destOk {
				destMap = make(map[string]any)
				dest[k] = destMap
			}
			mergeMaps(destMap, srcMap)
		} else if srcStr, ok := v.(string); ok {
			if k == "name" || k == "arguments" {
				existing, _ := dest[k].(string)
				dest[k] = existing + srcStr
			} else {
				dest[k] = v
			}
		} else {
			dest[k] = v
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
			if len(call.RawMap) > 0 {
				if bytes, err := json.Marshal(call.RawMap); err == nil {
					call.Raw = bytes
				}
			}
			result = append(result, *call)
		}
	}
	return result
}

func toSDKChatMessages(messages []ConversationItem) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		if call, ok := parseAssistantToolCallContent(msg.Content); ok {
			var sdkToolCalls []openai.ChatCompletionMessageToolCallUnionParam
			if len(call.Raw) > 0 {
				var rawToolCall map[string]any
				if err := json.Unmarshal(call.Raw, &rawToolCall); err == nil {
					var sdkCall openai.ChatCompletionMessageToolCallUnionParam
					if rawBytes, err := json.Marshal(rawToolCall); err == nil {
						if err := json.Unmarshal(rawBytes, &sdkCall); err == nil {
							sdkToolCalls = append(sdkToolCalls, sdkCall)
						}
					}
				}
			}

			if len(sdkToolCalls) == 0 {
				sdkToolCalls = append(sdkToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID:   call.CallID,
						Type: "function",
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      call.Name,
							Arguments: assistantToolCallArguments(call),
						},
					},
				})
			}

			result = append(result, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Role:      "assistant",
					ToolCalls: sdkToolCalls,
				},
			})
			continue
		}
		if msg.Role == RoleTool {
			call, output := parseToolResultContent(msg.Content)
			result = append(result, openai.ToolMessage(output, call.CallID))
			continue
		}

		role := normalizeConversationRole(msg.Role)
		switch role {
		case RoleUser:
			result = append(result, openai.UserMessage(msg.Content))
		case RoleAssistant:
			result = append(result, openai.AssistantMessage(msg.Content))
		default:
			result = append(result, openai.UserMessage(msg.Content))
		}
	}
	return result
}

func toSDKChatTools(tools ToolSet) []openai.ChatCompletionToolUnionParam {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools.Local))
	for _, tool := range tools.Local {
		parameters := map[string]any{
			schemaType: schemaObject,
			schemaProperties: map[string]any{
				schemaInput: map[string]any{
					schemaType:        schemaString,
					schemaDescription: toolInputDescription(tool),
				},
			},
			schemaRequired:             []string{schemaInput},
			schemaAdditionalProperties: false,
		}

		result = append(result, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Type: "function",
				Function: shared.FunctionDefinitionParam{
					Name:        tool.Name,
					Description: param.NewOpt(strings.TrimSpace(tool.Description)),
					Parameters:  shared.FunctionParameters(parameters),
					Strict:      param.NewOpt(true),
				},
			},
		})
	}
	return result
}

func responseFromSDKChatCompletion(comp *openai.ChatCompletion) Response {
	if len(comp.Choices) == 0 {
		return Response{}
	}
	message := comp.Choices[0].Message
	text := strings.TrimSpace(message.Content)

	input := int(comp.Usage.PromptTokens)
	output := int(comp.Usage.CompletionTokens)
	total := int(comp.Usage.TotalTokens)
	cacheRead := int(comp.Usage.PromptTokensDetails.CachedTokens)
	reasoning := int(comp.Usage.CompletionTokensDetails.ReasoningTokens)

	result := Response{
		FinalText: text,
		Usage: Usage{
			InputTokens:           input,
			OutputTokens:          output,
			TotalTokens:           total,
			CacheReadInputTokens:  cacheRead,
			ReasoningTokens:       reasoning,
		},
	}
	if text != "" {
		result.OutputItems = []ConversationItem{AssistantText(text)}
	}
	result.PendingCalls = pendingCallsFromSDKToolCalls(message.ToolCalls)
	if len(result.PendingCalls) > 0 {
		result.OutputItems = append(result.OutputItems, assistantToolCallItems(result.PendingCalls)...)
		result.FinalText = ""
	}
	return result
}

func pendingCallsFromSDKToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) []ToolInvocation {
	result := make([]ToolInvocation, 0, len(toolCalls))
	for _, call := range toolCalls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		var raw []byte
		if rawJSON := call.RawJSON(); rawJSON != "" {
			raw = []byte(rawJSON)
		}
		arguments := strings.TrimSpace(call.Function.Arguments)
		invocation := ToolInvocation{
			Kind:   InvokeCustomTool,
			Name:   strings.TrimSpace(call.Function.Name),
			CallID: strings.TrimSpace(call.ID),
			Raw:    raw,
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

func toInternalToolCallDeltas(sdkDeltas []openai.ChatCompletionChunkChoiceDeltaToolCall) []chatCompletionToolCallDelta {
	result := make([]chatCompletionToolCallDelta, 0, len(sdkDeltas))
	for _, sdk := range sdkDeltas {
		var raw []byte
		if rawJSON := sdk.RawJSON(); rawJSON != "" {
			raw = []byte(rawJSON)
		}
		result = append(result, chatCompletionToolCallDelta{
			ID:    sdk.ID,
			Type:  sdk.Type,
			Index: int(sdk.Index),
			Raw:   raw,
			Function: chatCompletionToolFunction{
				Name:      sdk.Function.Name,
				Arguments: sdk.Function.Arguments,
			},
		})
	}
	return result
}
