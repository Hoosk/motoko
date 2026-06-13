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
	sdkClient          *anthropic.Client
	baseClient
	thinkingBudget     int
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

	fallback := true
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
func buildAnthropicSystemBlocks(systemPrompt string) []anthropic.TextBlockParam {
	parts := strings.SplitN(systemPrompt, "--- DYNAMIC ---", 2)
	if len(parts) == 2 {
		return []anthropic.TextBlockParam{
			{
				Text:         strings.TrimSpace(parts[0]),
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
			{
				Text:         strings.TrimSpace(parts[1]),
			},
		}
	}
	return []anthropic.TextBlockParam{
		{
			Text:         systemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		},
	}
}

func buildAnthropicBetaSystemBlocks(systemPrompt string) []anthropic.BetaTextBlockParam {
	parts := strings.SplitN(systemPrompt, "--- DYNAMIC ---", 2)
	if len(parts) == 2 {
		return []anthropic.BetaTextBlockParam{
			{
				Text:         strings.TrimSpace(parts[0]),
				CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
			},
			{
				Text:         strings.TrimSpace(parts[1]),
			},
		}
	}
	return []anthropic.BetaTextBlockParam{
		{
			Text:         systemPrompt,
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		},
	}
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
		System:    buildAnthropicSystemBlocks(systemPrompt),
		Messages:  toSDKMessages(messages),
	}

	if sdkTools := toSDKTools(tools); len(sdkTools) > 0 {
		params.Tools = sdkTools
	}

	if c.thinkingBudget > 0 {
		params.OutputConfig = anthropic.OutputConfigParam{
			Effort: BudgetToAnthropicEffort(c.thinkingBudget),
		}
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
		result = append(result, ModelInfo{
			ID:               id,
			ContextWindow:    int(modelInfo.MaxInputTokens),
			SupportsThinking: modelInfo.Capabilities.Thinking.Supported,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (c *anthropicClient) GetModel(ctx context.Context, model string) (ModelInfo, error) {
	if err := c.ConfigurationError(); err != nil {
		return ModelInfo{}, err
	}
	modelInfo, err := c.sdkClient.Models.Get(ctx, model, anthropic.ModelGetParams{})
	if err != nil {
		return ModelInfo{}, err
	}
	return ModelInfo{
		ID:               modelInfo.ID,
		ContextWindow:    int(modelInfo.MaxInputTokens),
		SupportsThinking: modelInfo.Capabilities.Thinking.Supported,
	}, nil
}

// CreateBatch implements BatchClient
func (c *anthropicClient) CreateBatch(ctx context.Context, requests []BatchRequestItem) (BatchResponse, error) {
	sdkReqs := make([]anthropic.BetaMessageBatchNewParamsRequest, 0, len(requests))
	for _, req := range requests {
		params := anthropic.BetaMessageBatchNewParamsRequestParams{
			Model:     anthropic.Model(c.model),
			MaxTokens: 4096,
			System: buildAnthropicBetaSystemBlocks(req.SystemPrompt),
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

type parsedBlock struct {
	toolUseInput map[string]any
	toolUseID    string
	toolUseName  string
	toolResultID string
	toolOutput   string
	text         string
	isToolUse    bool
	isToolResult bool
	isText       bool
}

func parseConversationItem(msg ConversationItem) (string, []parsedBlock) {
	role := normalizeConversationRole(msg.Role)
	if role == RoleSystem {
		return "", nil
	}

	var blocks []parsedBlock
	if call, ok := parseAssistantToolCallContent(msg.Content); ok {
		var toolInput map[string]any
		if err := json.Unmarshal(call.Arguments, &toolInput); err != nil {
			toolInput = map[string]any{"input": call.Input}
		}
		blocks = append(blocks, parsedBlock{
			isToolUse:    true,
			toolUseID:    call.CallID,
			toolUseName:  call.Name,
			toolUseInput: toolInput,
		})
	} else if msg.Role == RoleTool {
		call, output := parseToolResultContent(msg.Content)
		blocks = append(blocks, parsedBlock{
			isToolResult: true,
			toolResultID: call.CallID,
			toolOutput:   output,
		})
	} else {
		blocks = append(blocks, parsedBlock{
			isText: true,
			text:   msg.Content,
		})
	}
	return role, blocks
}

func buildSDKMessages[MsgT any, RoleT any, BlockT any](
	messages []ConversationItem,
	roleUser RoleT,
	roleAssistant RoleT,
	buildBlock func(b parsedBlock) BlockT,
	setCacheControl func(block *BlockT),
	buildMessage func(role RoleT, blocks []BlockT) MsgT,
) []MsgT {
	result := make([]MsgT, 0, len(messages))
	for i, msg := range messages {
		role, blocks := parseConversationItem(msg)
		if role == "" {
			continue
		}

		var sdkRole RoleT
		if role == RoleUser {
			sdkRole = roleUser
		} else {
			sdkRole = roleAssistant
		}

		sdkBlocks := make([]BlockT, len(blocks))
		for j, b := range blocks {
			sdkBlocks[j] = buildBlock(b)
		}

		if i == len(messages)-1 && role == RoleUser && len(sdkBlocks) > 0 {
			setCacheControl(&sdkBlocks[len(sdkBlocks)-1])
		}

		result = append(result, buildMessage(sdkRole, sdkBlocks))
	}
	return result
}

func buildSDKTools[T any](tools ToolSet, buildFn func(t LocalToolDefinition, isLast bool) T) []T {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]T, 0, len(tools.Local))
	for i, tool := range tools.Local {
		result = append(result, buildFn(tool, i == len(tools.Local)-1))
	}
	return result
}

func toolProperties(t LocalToolDefinition) map[string]any {
	return map[string]any{
		"input": map[string]any{
			"type":        "string",
			"description": toolInputDescription(t),
		},
	}
}

func toSDKTools(tools ToolSet) []anthropic.ToolUnionParam {
	return buildSDKTools(tools, func(t LocalToolDefinition, isLast bool) anthropic.ToolUnionParam {
		tParam := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(strings.TrimSpace(t.Description)),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: toolProperties(t),
				Required:   []string{"input"},
			},
		}
		if isLast {
			tParam.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		return anthropic.ToolUnionParam{OfTool: &tParam}
	})
}

func toSDKMessages(messages []ConversationItem) []anthropic.MessageParam {
	return buildSDKMessages(
		messages,
		anthropic.MessageParamRoleUser,
		anthropic.MessageParamRoleAssistant,
		func(b parsedBlock) anthropic.ContentBlockParamUnion {
			if b.isToolUse {
				return anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    b.toolUseID,
						Name:  b.toolUseName,
						Input: b.toolUseInput,
					},
				}
			}
			if b.isToolResult {
				return anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: b.toolResultID,
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{
								OfText: &anthropic.TextBlockParam{
									Text: b.toolOutput,
								},
							},
						},
					},
				}
			}
			return anthropic.NewTextBlock(b.text)
		},
		func(block *anthropic.ContentBlockParamUnion) {
			if block.OfText != nil {
				block.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
			} else if block.OfToolResult != nil {
				block.OfToolResult.CacheControl = anthropic.NewCacheControlEphemeralParam()
			}
		},
		func(role anthropic.MessageParamRole, blocks []anthropic.ContentBlockParamUnion) anthropic.MessageParam {
			return anthropic.MessageParam{
				Role:    role,
				Content: blocks,
			}
		},
	)
}

func toSDKBetaMessages(messages []ConversationItem) []anthropic.BetaMessageParam {
	return buildSDKMessages(
		messages,
		anthropic.BetaMessageParamRoleUser,
		anthropic.BetaMessageParamRoleAssistant,
		func(b parsedBlock) anthropic.BetaContentBlockParamUnion {
			switch {
			case b.isToolUse:
				return anthropic.BetaContentBlockParamUnion{
					OfToolUse: &anthropic.BetaToolUseBlockParam{
						ID:    b.toolUseID,
						Name:  b.toolUseName,
						Input: b.toolUseInput,
					},
				}
			case b.isToolResult:
				return anthropic.BetaContentBlockParamUnion{
					OfToolResult: &anthropic.BetaToolResultBlockParam{
						ToolUseID: b.toolResultID,
						Content: []anthropic.BetaToolResultBlockParamContentUnion{
							{
								OfText: &anthropic.BetaTextBlockParam{
									Text: b.toolOutput,
								},
							},
						},
					},
				}
			default:
				return anthropic.NewBetaTextBlock(b.text)
			}
		},
		func(block *anthropic.BetaContentBlockParamUnion) {
			if block.OfText != nil {
				block.OfText.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
			} else if block.OfToolResult != nil {
				block.OfToolResult.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
			}
		},
		func(role anthropic.BetaMessageParamRole, blocks []anthropic.BetaContentBlockParamUnion) anthropic.BetaMessageParam {
			return anthropic.BetaMessageParam{
				Role:    role,
				Content: blocks,
			}
		},
	)
}

func toSDKBetaTools(tools ToolSet) []anthropic.BetaToolUnionParam {
	return buildSDKTools(tools, func(t LocalToolDefinition, isLast bool) anthropic.BetaToolUnionParam {
		tParam := anthropic.BetaToolParam{
			Name:        t.Name,
			Description: anthropic.String(strings.TrimSpace(t.Description)),
			InputSchema: anthropic.BetaToolInputSchemaParam{
				Properties: toolProperties(t),
				Required:   []string{"input"},
			},
		}
		if isLast {
			tParam.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
		}
		return anthropic.BetaToolUnionParam{OfTool: &tParam}
	})
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
