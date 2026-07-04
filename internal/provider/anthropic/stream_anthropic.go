package anthropic

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/Hoosk/motoko/internal/provider"
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func (c *anthropicClient) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	if err := c.ConfigurationError(); err != nil {
		return provider.Response{}, err
	}
	maxTokens := 4096
	if c.thinkingBudget > 0 {
		if c.thinkingBudget >= maxTokens {
			maxTokens = c.thinkingBudget + 1024
		}
	}

	params := sdk.MessageNewParams{
		Model:     sdk.Model(c.model),
		MaxTokens: int64(maxTokens),
		System:    buildAnthropicSystemBlocks(systemPrompt),
		Messages:  toSDKMessages(messages),
	}

	if sdkTools := toSDKTools(tools); len(sdkTools) > 0 {
		params.Tools = sdkTools
	}

	if c.thinkingBudget > 0 {
		params.OutputConfig = sdk.OutputConfigParam{
			Effort: provider.BudgetToAnthropicEffort(c.thinkingBudget),
		}
		if c.checkAdaptiveThinking(ctx) {
			params.Thinking = sdk.ThinkingConfigParamUnion{
				OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{
					Display: sdk.ThinkingConfigAdaptiveDisplaySummarized,
				},
			}
		} else {
			params.Thinking = sdk.ThinkingConfigParamOfEnabled(int64(c.thinkingBudget))
		}
	}

	reqOpts := []option.RequestOption{
		option.WithHeader("anthropic-beta", "prompt-caching-2024-07-31"),
	}
	telemetryHeaders := map[string]string{}
	if sessionID, requestID := provider.GetTelemetry(ctx); sessionID != "" {
		provider.ApplyTelemetryHeaders(c.providerName, telemetryHeaders, sessionID, requestID)
	}
	for k, v := range telemetryHeaders {
		reqOpts = append(reqOpts, option.WithHeader(k, v))
	}

	stream := c.sdkClient.Messages.NewStreaming(ctx, params, reqOpts...)
	defer func() { _ = stream.Close() }()

	var raw strings.Builder
	usage := provider.Usage{}

	type streamedToolCall struct {
		id           string
		name         string
		partialInput strings.Builder
	}
	toolCalls := make(map[int]*streamedToolCall)

	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "message_start":
			msgEvent := event.AsMessageStart()
			usage.InputTokens = int(msgEvent.Message.Usage.InputTokens)
			usage.CacheReadInputTokens = int(msgEvent.Message.Usage.CacheReadInputTokens)
			usage.CacheWriteInputTokens = int(msgEvent.Message.Usage.CacheCreationInputTokens)

		case "content_block_start":
			blockEvent := event.AsContentBlockStart()
			if blockEvent.ContentBlock.Type == "tool_use" {
				toolCalls[int(blockEvent.Index)] = &streamedToolCall{
					id:   blockEvent.ContentBlock.ID,
					name: blockEvent.ContentBlock.Name,
				}
			}

		case "content_block_delta":
			deltaEvent := event.AsContentBlockDelta()
			switch d := deltaEvent.Delta.AsAny().(type) {
			case sdk.TextDelta:
				if d.Text != "" {
					raw.WriteString(d.Text)
					if onDelta != nil {
						if err := onDelta(provider.Delta{Content: d.Text}); err != nil {
							return provider.Response{}, err
						}
					}
				}
			case sdk.ThinkingDelta:
				if d.Thinking != "" {
					if onDelta != nil {
						if err := onDelta(provider.Delta{ReasoningContent: d.Thinking}); err != nil {
							return provider.Response{}, err
						}
					}
				}
			case sdk.InputJSONDelta:
				if tc, ok := toolCalls[int(deltaEvent.Index)]; ok {
					tc.partialInput.WriteString(d.PartialJSON)
				}
			}

		case "message_delta":
			msgDelta := event.AsMessageDelta()
			if msgDelta.Usage.OutputTokens > 0 {
				usage.OutputTokens = int(msgDelta.Usage.OutputTokens)
				usage.ReasoningTokens = int(msgDelta.Usage.OutputTokensDetails.ThinkingTokens)
				usage.CacheReadInputTokens = int(msgDelta.Usage.CacheReadInputTokens)
				usage.CacheWriteInputTokens = int(msgDelta.Usage.CacheCreationInputTokens)
				usage.TotalTokens = usage.InputTokens + usage.OutputTokens
			}
		}
	}

	if err := stream.Err(); err != nil {
		return provider.Response{}, err
	}

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}

	keys := make([]int, 0, len(toolCalls))
	for k := range toolCalls {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	var pendingCalls []provider.ToolInvocation
	for _, k := range keys {
		tc := toolCalls[k]
		rawInput := tc.partialInput.String()
		var parsed struct {
			Input string `json:"input"`
		}
		var inputStr string
		if err := json.Unmarshal([]byte(rawInput), &parsed); err == nil {
			inputStr = parsed.Input
		} else {
			inputStr = rawInput
		}
		pendingCalls = append(pendingCalls, provider.ToolInvocation{
			Kind:      provider.InvokeCustomTool,
			Name:      strings.TrimSpace(tc.name),
			Input:     strings.TrimSpace(inputStr),
			Arguments: json.RawMessage(rawInput),
			CallID:    strings.TrimSpace(tc.id),
		})
	}

	finalText := strings.TrimSpace(raw.String())
	result := provider.Response{FinalText: finalText, Usage: usage, PendingCalls: pendingCalls}
	if finalText != "" || len(pendingCalls) > 0 {
		result.OutputItems = []provider.ConversationItem{provider.AssistantTurn(finalText, "", pendingCalls)}
	}
	if len(result.PendingCalls) > 0 {
		result.FinalText = ""
	}
	return result, nil
}
