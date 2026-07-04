package openai

import (
	"encoding/json"
	"sort"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/Hoosk/motoko/internal/provider"
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

func (u chatCompletionUsage) providerUsage() provider.Usage {
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
	usage := provider.Usage{InputTokens: input, OutputTokens: output, TotalTokens: total}
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

func responseFromChatCompletion(decoded chatCompletionResponse) provider.Response {
	if len(decoded.Choices) == 0 {
		return provider.Response{}
	}
	message := decoded.Choices[0].Message
	text := strings.TrimSpace(message.Content)
	reasoning := strings.TrimSpace(message.ReasoningContent)
	pending := pendingCallsFromChatToolCalls(message.ToolCalls)
	result := provider.Response{FinalText: text, Usage: decoded.Usage.providerUsage(), PendingCalls: pending}
	if text != "" || reasoning != "" || len(pending) > 0 {
		result.OutputItems = []provider.ConversationItem{provider.AssistantTurn(text, reasoning, pending)}
	}
	if len(pending) > 0 {
		result.FinalText = ""
	}
	return result
}

func pendingCallsFromChatToolCalls(toolCalls []chatCompletionToolCall) []provider.ToolInvocation {
	result := make([]provider.ToolInvocation, 0, len(toolCalls))
	for _, call := range toolCalls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		arguments := strings.TrimSpace(call.Function.Arguments)
		invocation := provider.ToolInvocation{
			Kind:   provider.InvokeCustomTool,
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

func toChatMessages(messages []provider.ConversationItem) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		if len(msg.ToolCalls) > 0 {
			item := map[string]any{
				keyRole:    provider.RoleAssistant,
				keyContent: msg.Content,
			}
			if msg.ReasoningContent != "" {
				item["reasoning_content"] = msg.ReasoningContent
			}
			toolCalls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				if len(call.Raw) > 0 {
					var rawToolCall map[string]any
					if err := json.Unmarshal(call.Raw, &rawToolCall); err == nil {
						toolCalls = append(toolCalls, rawToolCall)
						continue
					}
				}
				toolCalls = append(toolCalls, map[string]any{
					"id":    call.CallID,
					keyType: keyFunction,
					keyFunction: map[string]any{
						keyName:     call.Name,
						"arguments": provider.AssistantToolCallArguments(call),
					},
				})
			}
			item["tool_calls"] = toolCalls
			result = append(result, item)
			continue
		}
		if msg.Role == provider.RoleTool {
			item := map[string]any{
				"role":    provider.RoleTool,
				"content": msg.Content,
			}
			if msg.ToolCallID != "" {
				item["tool_call_id"] = msg.ToolCallID
			}
			if msg.ToolName != "" {
				item["name"] = msg.ToolName
			}
			result = append(result, item)
			continue
		}
		item := map[string]any{
			"role":    provider.NormalizeConversationRole(msg.Role),
			"content": msg.Content,
		}
		if msg.ReasoningContent != "" {
			item["reasoning_content"] = msg.ReasoningContent
		}
		result = append(result, item)
	}
	return result
}

func toResponsesInputItems(messages []provider.ConversationItem) responses.ResponseInputParam {
	items := make(responses.ResponseInputParam, 0, len(messages))
	for _, msg := range messages {
		if len(msg.ToolCalls) > 0 {
			for _, call := range msg.ToolCalls {
				if call.CallID != "" {
					items = append(items, responses.ResponseInputItemParamOfFunctionCall(provider.AssistantToolCallArguments(call), call.CallID, call.Name))
				}
			}
			continue
		}
		if msg.Role == provider.RoleTool {
			if msg.ToolCallID != "" {
				items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(msg.ToolCallID, msg.Content))
				continue
			}
		}
		role := responses.EasyInputMessageRole(provider.NormalizeConversationRole(msg.Role))
		items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, role))
	}
	return items
}

func responseTools(tools provider.ToolSet) []responses.ToolUnionParam {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]responses.ToolUnionParam, 0, len(tools.Local))
	for _, tool := range tools.Local {
		parameters := map[string]any{
			keyType: keyObject,
			keyProperties: map[string]any{
				keyInput: map[string]any{
					keyType:        keyString,
					keyDescription: provider.ToolInputDescription(tool),
				},
			},
			keyRequired:             []string{keyInput},
			keyAdditionalProperties: false,
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

func chatCompletionTools(tools provider.ToolSet) []map[string]any {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(tools.Local))
	for _, tool := range tools.Local {
		result = append(result, map[string]any{
			keyType: keyFunction,
			keyFunction: map[string]any{
				keyName:        tool.Name,
				keyDescription: strings.TrimSpace(tool.Description),
				"parameters": map[string]any{
					keyType: keyObject,
					keyProperties: map[string]any{
						keyInput: map[string]any{
							keyType:        keyString,
							keyDescription: provider.ToolInputDescription(tool),
						},
					},
					keyRequired:             []string{keyInput},
					keyAdditionalProperties: false,
				},
			},
		})
	}
	return result
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

func toSDKChatMessages(messages []provider.ConversationItem) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		if len(msg.ToolCalls) > 0 || msg.ReasoningContent != "" {
			msgMap := map[string]any{
				"role":    provider.RoleAssistant,
				"content": msg.Content,
			}
			if msg.ReasoningContent != "" {
				msgMap["reasoning_content"] = msg.ReasoningContent
			}
			var sdkToolCalls []openai.ChatCompletionMessageToolCallUnionParam
			for _, call := range msg.ToolCalls {
				if len(call.Raw) > 0 {
					var rawToolCall map[string]any
					if err := json.Unmarshal(call.Raw, &rawToolCall); err == nil {
						var sdkCall openai.ChatCompletionMessageToolCallUnionParam
						if rawBytes, err := json.Marshal(rawToolCall); err == nil {
							if err := json.Unmarshal(rawBytes, &sdkCall); err == nil {
								sdkToolCalls = append(sdkToolCalls, sdkCall)
								continue
							}
						}
					}
				}
				sdkToolCalls = append(sdkToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID:   call.CallID,
						Type: "function",
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      call.Name,
							Arguments: provider.AssistantToolCallArguments(call),
						},
					},
				})
			}
			if len(sdkToolCalls) > 0 {
				msgMap["tool_calls"] = sdkToolCalls
			}
			var sdkMsg openai.ChatCompletionMessageParamUnion
			if rawBytes, err := json.Marshal(msgMap); err == nil {
				if err := json.Unmarshal(rawBytes, &sdkMsg); err == nil {
					result = append(result, sdkMsg)
					continue
				}
			}
			result = append(result, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{Role: "assistant", ToolCalls: sdkToolCalls},
			})
			continue
		}
		if msg.Role == provider.RoleTool {
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
			continue
		}

		role := provider.NormalizeConversationRole(msg.Role)
		switch role {
		case provider.RoleUser:
			result = append(result, openai.UserMessage(msg.Content))
		case provider.RoleAssistant:
			if msg.ReasoningContent != "" {
				msgMap := map[string]any{"role": provider.RoleAssistant, "content": msg.Content, "reasoning_content": msg.ReasoningContent}
				var sdkMsg openai.ChatCompletionMessageParamUnion
				if rawBytes, err := json.Marshal(msgMap); err == nil {
					if err := json.Unmarshal(rawBytes, &sdkMsg); err == nil {
						result = append(result, sdkMsg)
						continue
					}
				}
			}
			result = append(result, openai.AssistantMessage(msg.Content))
		default:
			result = append(result, openai.UserMessage(msg.Content))
		}
	}
	return result
}

func toSDKChatTools(tools provider.ToolSet) []openai.ChatCompletionToolUnionParam {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools.Local))
	for _, tool := range tools.Local {
		parameters := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": provider.ToolInputDescription(tool),
				},
			},
			"required":             []string{"input"},
			"additionalProperties": false,
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

func responseFromSDKChatCompletion(comp *openai.ChatCompletion) provider.Response {
	if len(comp.Choices) == 0 {
		return provider.Response{}
	}
	message := comp.Choices[0].Message
	text := strings.TrimSpace(message.Content)
	reasoningContent := strings.TrimSpace(rawReasoningContent(message.RawJSON()))

	input := int(comp.Usage.PromptTokens)
	output := int(comp.Usage.CompletionTokens)
	total := int(comp.Usage.TotalTokens)
	cacheRead := int(comp.Usage.PromptTokensDetails.CachedTokens)
	reasoningTokens := int(comp.Usage.CompletionTokensDetails.ReasoningTokens)

	pending := pendingCallsFromSDKToolCalls(message.ToolCalls)
	result := provider.Response{
		FinalText:    text,
		PendingCalls: pending,
		Usage: provider.Usage{
			InputTokens:          input,
			OutputTokens:         output,
			TotalTokens:          total,
			CacheReadInputTokens: cacheRead,
			ReasoningTokens:      reasoningTokens,
		},
	}
	if text != "" || reasoningContent != "" || len(pending) > 0 {
		result.OutputItems = []provider.ConversationItem{provider.AssistantTurn(text, reasoningContent, pending)}
	}
	if len(result.PendingCalls) > 0 {
		result.FinalText = ""
	}
	return result
}

func pendingCallsFromSDKToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) []provider.ToolInvocation {
	result := make([]provider.ToolInvocation, 0, len(toolCalls))
	for _, call := range toolCalls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		var raw []byte
		if rawJSON := call.RawJSON(); rawJSON != "" {
			raw = []byte(rawJSON)
		}
		arguments := strings.TrimSpace(call.Function.Arguments)
		invocation := provider.ToolInvocation{
			Kind:   provider.InvokeCustomTool,
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

func openAIInvocationInput(arguments json.RawMessage) string {
	var parsed struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal(arguments, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Input)
}

func rawReasoningContent(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var parsed string
	var payload struct {
		ReasoningContent string `json:"reasoning_content"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return strings.TrimSpace(payload.ReasoningContent)
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return strings.TrimSpace(parsed)
	}
	return ""
}
