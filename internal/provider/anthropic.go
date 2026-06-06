package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type anthropicClient struct {
	baseClient
	thinkingBudget int
	sdkClient      *anthropic.Client

	mu                 sync.Mutex
	capabilitiesLoaded bool
	isAdaptive         bool
}

func newAnthropicClient(cfg config.ProviderConfig) Client {
	c := &anthropicClient{
		baseClient:     newBaseClient(cfg.Name, cfg.BaseURL, cfg.APIKey, cfg.Model),
		thinkingBudget: cfg.ThinkingBudget,
	}
	sdkClient := anthropic.NewClient(
		option.WithAPIKey(c.apiKey),
		option.WithBaseURL(c.baseURL),
		option.WithHTTPClient(c.httpClient),
	)
	c.sdkClient = &sdkClient
	return c
}

func (c *anthropicClient) checkAdaptiveThinking(ctx context.Context) bool {
	c.mu.Lock()
	if c.capabilitiesLoaded {
		res := c.isAdaptive
		c.mu.Unlock()
		return res
	}
	c.mu.Unlock()

	fallback := isAnthropicAdaptiveThinkingModel(c.model)
	if !c.listReady() {
		return fallback
	}

	modelInfo, err := c.sdkClient.Models.Get(ctx, c.model, anthropic.ModelGetParams{})
	c.mu.Lock()
	defer c.mu.Unlock()

	if err == nil {
		c.isAdaptive = modelInfo.Capabilities.Thinking.Types.Adaptive.Supported
		c.capabilitiesLoaded = true
	} else {
		return fallback
	}
	return c.isAdaptive
}

func (c *anthropicClient) Complete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	if err := c.ConfigurationError(); err != nil {
		return Response{}, err
	}

	maxTokens := 4096
	if c.thinkingBudget > 0 {
		if c.thinkingBudget >= maxTokens {
			maxTokens = c.thinkingBudget + 1024
		}
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: int64(maxTokens),
		System: []anthropic.TextBlockParam{
			{
				Text:         systemPrompt,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		},
		Messages: toSDKMessages(messages),
	}

	if sdkTools := toSDKTools(tools); len(sdkTools) > 0 {
		params.Tools = sdkTools
	}

	if c.thinkingBudget > 0 {
		if c.checkAdaptiveThinking(ctx) {
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{
					Display: anthropic.ThinkingConfigAdaptiveDisplaySummarized,
				},
			}
		} else {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(c.thinkingBudget))
		}
	}

	resp, err := c.sdkClient.Messages.New(ctx, params, option.WithHeader("anthropic-beta", "prompt-caching-2024-07-31"))
	if err != nil {
		return Response{}, err
	}

	return responseFromSDK(resp), nil
}

func (c *anthropicClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if !c.listReady() {
		return nil, fmt.Errorf("provider no configurado")
	}

	iter := c.sdkClient.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	var result []ModelInfo
	for iter.Next() {
		modelInfo := iter.Current()
		id := strings.TrimSpace(modelInfo.ID)
		if id == "" {
			continue
		}
		result = append(result, ModelInfo{ID: id, ContextWindow: int(modelInfo.MaxInputTokens)})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// CreateBatch implements BatchClient
func (c *anthropicClient) CreateBatch(ctx context.Context, requests []BatchRequestItem) (BatchResponse, error) {
	sdkReqs := make([]anthropic.BetaMessageBatchNewParamsRequest, 0, len(requests))
	for _, req := range requests {
		params := anthropic.BetaMessageBatchNewParamsRequestParams{
			Model:     anthropic.Model(c.model),
			MaxTokens: 4096,
			System: []anthropic.BetaTextBlockParam{
				{
					Text:         req.SystemPrompt,
					CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
				},
			},
			Messages: toSDKBetaMessages(req.Messages),
		}

		if betaTools := toSDKBetaTools(req.Tools); len(betaTools) > 0 {
			params.Tools = betaTools
		}

		sdkReqs = append(sdkReqs, anthropic.BetaMessageBatchNewParamsRequest{
			CustomID: req.CustomID,
			Params:   params,
		})
	}

	batch, err := c.sdkClient.Beta.Messages.Batches.New(ctx, anthropic.BetaMessageBatchNewParams{
		Requests: sdkReqs,
	})
	if err != nil {
		return BatchResponse{}, err
	}

	return batchResponseFromSDK(batch), nil
}

// RetrieveBatch implements BatchClient
func (c *anthropicClient) RetrieveBatch(ctx context.Context, batchID string) (BatchResponse, error) {
	batch, err := c.sdkClient.Beta.Messages.Batches.Get(ctx, batchID, anthropic.BetaMessageBatchGetParams{})
	if err != nil {
		return BatchResponse{}, err
	}
	return batchResponseFromSDK(batch), nil
}

// CancelBatch implements BatchClient
func (c *anthropicClient) CancelBatch(ctx context.Context, batchID string) (BatchResponse, error) {
	batch, err := c.sdkClient.Beta.Messages.Batches.Cancel(ctx, batchID, anthropic.BetaMessageBatchCancelParams{})
	if err != nil {
		return BatchResponse{}, err
	}
	return batchResponseFromSDK(batch), nil
}

func batchResponseFromSDK(batch *anthropic.BetaMessageBatch) BatchResponse {
	resultsURL := ""
	if batch.ResultsURL != "" {
		resultsURL = batch.ResultsURL
	}
	return BatchResponse{
		ID:               batch.ID,
		ProcessingStatus: string(batch.ProcessingStatus),
		ProcessingCount:  int(batch.RequestCounts.Processing),
		SucceededCount:   int(batch.RequestCounts.Succeeded),
		ErroredCount:     int(batch.RequestCounts.Errored),
		ResultsURL:       resultsURL,
	}
}

func toSDKTools(tools ToolSet) []anthropic.ToolUnionParam {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]anthropic.ToolUnionParam, 0, len(tools.Local))
	for i, tool := range tools.Local {
		t := tool
		tParam := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(strings.TrimSpace(t.Description)),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": toolInputDescription(t),
					},
				},
				Required: []string{"input"},
			},
		}
		if i == len(tools.Local)-1 {
			tParam.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &tParam})
	}
	return result
}

func toSDKMessages(messages []ConversationItem) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, 0, len(messages))
	for i, msg := range messages {
		role := normalizeConversationRole(msg.Role)
		if role == RoleSystem {
			continue
		}

		var sdkRole anthropic.MessageParamRole
		if role == RoleUser {
			sdkRole = anthropic.MessageParamRoleUser
		} else {
			sdkRole = anthropic.MessageParamRoleAssistant
		}

		var blocks []anthropic.ContentBlockParamUnion

		if call, ok := parseAssistantToolCallContent(msg.Content); ok {
			var toolInput map[string]any
			if err := json.Unmarshal(call.Arguments, &toolInput); err != nil {
				toolInput = map[string]any{"input": call.Input}
			}
			blocks = append(blocks, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    call.CallID,
					Name:  call.Name,
					Input: toolInput,
				},
			})
		} else if msg.Role == RoleTool {
			call, output := parseToolResultContent(msg.Content)
			blocks = append(blocks, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: call.CallID,
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{
							OfText: &anthropic.TextBlockParam{
								Text: output,
							},
						},
					},
				},
			})
		} else {
			blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
		}

		if i == len(messages)-1 && sdkRole == anthropic.MessageParamRoleUser && len(blocks) > 0 {
			lastIdx := len(blocks) - 1
			b := blocks[lastIdx]
			if b.OfText != nil {
				b.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
				blocks[lastIdx] = b
			} else if b.OfToolResult != nil {
				b.OfToolResult.CacheControl = anthropic.NewCacheControlEphemeralParam()
				blocks[lastIdx] = b
			}
		}

		result = append(result, anthropic.MessageParam{
			Role:    sdkRole,
			Content: blocks,
		})
	}
	return result
}

func toSDKBetaMessages(messages []ConversationItem) []anthropic.BetaMessageParam {
	result := make([]anthropic.BetaMessageParam, 0, len(messages))
	for i, msg := range messages {
		role := normalizeConversationRole(msg.Role)
		if role == RoleSystem {
			continue
		}

		var sdkRole anthropic.BetaMessageParamRole
		if role == RoleUser {
			sdkRole = anthropic.BetaMessageParamRoleUser
		} else {
			sdkRole = anthropic.BetaMessageParamRoleAssistant
		}

		var blocks []anthropic.BetaContentBlockParamUnion

		if call, ok := parseAssistantToolCallContent(msg.Content); ok {
			var toolInput map[string]any
			if err := json.Unmarshal(call.Arguments, &toolInput); err != nil {
				toolInput = map[string]any{"input": call.Input}
			}
			blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
				OfToolUse: &anthropic.BetaToolUseBlockParam{
					ID:    call.CallID,
					Name:  call.Name,
					Input: toolInput,
				},
			})
		} else if msg.Role == RoleTool {
			call, output := parseToolResultContent(msg.Content)
			blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
				OfToolResult: &anthropic.BetaToolResultBlockParam{
					ToolUseID: call.CallID,
					Content: []anthropic.BetaToolResultBlockParamContentUnion{
						{
							OfText: &anthropic.BetaTextBlockParam{
								Text: output,
							},
						},
					},
				},
			})
		} else {
			blocks = append(blocks, anthropic.NewBetaTextBlock(msg.Content))
		}

		if i == len(messages)-1 && sdkRole == anthropic.BetaMessageParamRoleUser && len(blocks) > 0 {
			lastIdx := len(blocks) - 1
			b := blocks[lastIdx]
			if b.OfText != nil {
				b.OfText.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
				blocks[lastIdx] = b
			} else if b.OfToolResult != nil {
				b.OfToolResult.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
				blocks[lastIdx] = b
			}
		}

		result = append(result, anthropic.BetaMessageParam{
			Role:    sdkRole,
			Content: blocks,
		})
	}
	return result
}

func toSDKBetaTools(tools ToolSet) []anthropic.BetaToolUnionParam {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]anthropic.BetaToolUnionParam, 0, len(tools.Local))
	for i, tool := range tools.Local {
		t := tool
		tParam := anthropic.BetaToolParam{
			Name:        t.Name,
			Description: anthropic.String(strings.TrimSpace(t.Description)),
			InputSchema: anthropic.BetaToolInputSchemaParam{
				Properties: map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": toolInputDescription(t),
					},
				},
				Required: []string{"input"},
			},
		}
		if i == len(tools.Local)-1 {
			tParam.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
		}
		result = append(result, anthropic.BetaToolUnionParam{OfTool: &tParam})
	}
	return result
}

func responseFromSDK(decoded *anthropic.Message) Response {
	var textParts []string
	var pendingCalls []ToolInvocation

	for _, block := range decoded.Content {
		switch block.Type {
		case "text":
			text := strings.TrimSpace(block.Text)
			if text != "" {
				textParts = append(textParts, text)
			}
		case "tool_use":
			toolUse := block.AsToolUse()
			var parsed struct {
				Input string `json:"input"`
			}
			var inputStr string
			rawInput, _ := json.Marshal(toolUse.Input)
			if err := json.Unmarshal(rawInput, &parsed); err == nil {
				inputStr = parsed.Input
			} else {
				inputStr = string(rawInput)
			}
			pendingCalls = append(pendingCalls, ToolInvocation{
				Kind:      InvokeCustomTool,
				Name:      strings.TrimSpace(toolUse.Name),
				Input:     strings.TrimSpace(inputStr),
				Arguments: rawInput,
				CallID:    strings.TrimSpace(toolUse.ID),
			})
		}
	}

	finalText := strings.TrimSpace(strings.Join(textParts, "\n"))
	usage := Usage{
		InputTokens:  int(decoded.Usage.InputTokens),
		OutputTokens: int(decoded.Usage.OutputTokens),
		TotalTokens:  int(decoded.Usage.InputTokens + decoded.Usage.OutputTokens),
	}

	result := Response{FinalText: finalText, Usage: usage}
	if finalText != "" {
		result.OutputItems = []ConversationItem{AssistantText(finalText)}
	}
	result.PendingCalls = pendingCalls
	if len(result.PendingCalls) > 0 {
		result.OutputItems = append(result.OutputItems, assistantToolCallItems(result.PendingCalls)...)
		result.FinalText = ""
	}
	return result
}
