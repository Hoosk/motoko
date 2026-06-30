package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const keyInput = "input"

func init() {
	provider.Register(config.ProviderKindAnthropic, NewClient)
}

type anthropicClient struct {
	sdkClient          *sdk.Client
	baseURL            string
	apiKey             string
	model              string
	providerName       string
	thinkingBudget     int
	mu                 sync.Mutex
	capabilitiesLoaded bool
	isAdaptive         bool
}

func NewClient(cfg config.ProviderConfig) provider.Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	apiKey := strings.TrimSpace(cfg.APIKey)
	model := strings.TrimSpace(cfg.Model)

	c := &anthropicClient{
		baseURL:        baseURL,
		apiKey:         apiKey,
		model:          model,
		providerName:   cfg.Name,
		thinkingBudget: cfg.ThinkingBudget,
	}
	sdkClient := sdk.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	)
	c.sdkClient = &sdkClient
	return c
}

func (c *anthropicClient) Configured() bool {
	return c.baseURL != "" && c.apiKey != "" && c.model != ""
}

func (c *anthropicClient) ConfigurationError() error {
	if c.baseURL == "" {
		return fmt.Errorf("provider not configured: empty base URL")
	}
	if c.apiKey == "" {
		return fmt.Errorf("provider not configured: empty API Key")
	}
	if c.model == "" {
		return fmt.Errorf("provider not configured: model not specified")
	}
	return nil
}

func (c *anthropicClient) listReady() bool {
	return c.baseURL != "" && c.apiKey != ""
}

func (c *anthropicClient) ProviderKind() string {
	return c.providerName
}

func (c *anthropicClient) Summary() string {
	return fmt.Sprintf("%s:%s", c.providerName, c.model)
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

	modelInfo, err := c.sdkClient.Models.Get(ctx, c.model, sdk.ModelGetParams{})
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

func buildAnthropicSystemBlocks(systemPrompt string) []sdk.TextBlockParam {
	parts := strings.SplitN(systemPrompt, "--- DYNAMIC ---", 2)
	if len(parts) == 2 {
		return []sdk.TextBlockParam{
			{
				Text:         strings.TrimSpace(parts[0]),
				CacheControl: sdk.NewCacheControlEphemeralParam(),
			},
			{
				Text: strings.TrimSpace(parts[1]),
			},
		}
	}
	return []sdk.TextBlockParam{
		{
			Text:         systemPrompt,
			CacheControl: sdk.NewCacheControlEphemeralParam(),
		},
	}
}

func buildAnthropicBetaSystemBlocks(systemPrompt string) []sdk.BetaTextBlockParam {
	parts := strings.SplitN(systemPrompt, "--- DYNAMIC ---", 2)
	if len(parts) == 2 {
		return []sdk.BetaTextBlockParam{
			{
				Text:         strings.TrimSpace(parts[0]),
				CacheControl: sdk.NewBetaCacheControlEphemeralParam(),
			},
			{
				Text: strings.TrimSpace(parts[1]),
			},
		}
	}
	return []sdk.BetaTextBlockParam{
		{
			Text:         systemPrompt,
			CacheControl: sdk.NewBetaCacheControlEphemeralParam(),
		},
	}
}

func (c *anthropicClient) Complete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
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
	if sessionID, requestID := provider.GetTelemetry(ctx); sessionID != "" {
		reqOpts = append(reqOpts, option.WithHeader("X-Session-ID", sessionID))
		if requestID != "" {
			reqOpts = append(reqOpts, option.WithHeader("X-Request-ID", requestID))
		}
	}

	resp, err := c.sdkClient.Messages.New(ctx, params, reqOpts...)
	if err != nil {
		return provider.Response{}, err
	}

	return responseFromSDK(resp), nil
}

func (c *anthropicClient) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if !c.listReady() {
		return nil, fmt.Errorf("provider no configurado")
	}

	iter := c.sdkClient.Models.ListAutoPaging(ctx, sdk.ModelListParams{})
	var result []provider.ModelInfo
	for iter.Next() {
		modelInfo := iter.Current()
		id := strings.TrimSpace(modelInfo.ID)
		if id == "" {
			continue
		}
		result = append(result, provider.ModelInfo{
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

func (c *anthropicClient) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	if err := c.ConfigurationError(); err != nil {
		return provider.ModelInfo{}, err
	}
	modelInfo, err := c.sdkClient.Models.Get(ctx, model, sdk.ModelGetParams{})
	if err != nil {
		return provider.ModelInfo{}, err
	}
	return provider.ModelInfo{
		ID:               modelInfo.ID,
		ContextWindow:    int(modelInfo.MaxInputTokens),
		SupportsThinking: modelInfo.Capabilities.Thinking.Supported,
	}, nil
}

// CreateBatch implements BatchClient
func (c *anthropicClient) CreateBatch(ctx context.Context, requests []provider.BatchRequestItem) (provider.BatchResponse, error) {
	sdkReqs := make([]sdk.BetaMessageBatchNewParamsRequest, 0, len(requests))
	for _, req := range requests {
		params := sdk.BetaMessageBatchNewParamsRequestParams{
			Model:     sdk.Model(c.model),
			MaxTokens: 4096,
			System:    buildAnthropicBetaSystemBlocks(req.SystemPrompt),
			Messages:  toSDKBetaMessages(req.Messages),
		}

		if betaTools := toSDKBetaTools(req.Tools); len(betaTools) > 0 {
			params.Tools = betaTools
		}

		sdkReqs = append(sdkReqs, sdk.BetaMessageBatchNewParamsRequest{
			CustomID: req.CustomID,
			Params:   params,
		})
	}

	batch, err := c.sdkClient.Beta.Messages.Batches.New(ctx, sdk.BetaMessageBatchNewParams{
		Requests: sdkReqs,
	})
	if err != nil {
		return provider.BatchResponse{}, err
	}

	return batchResponseFromSDK(batch), nil
}

// RetrieveBatch implements BatchClient
func (c *anthropicClient) RetrieveBatch(ctx context.Context, batchID string) (provider.BatchResponse, error) {
	batch, err := c.sdkClient.Beta.Messages.Batches.Get(ctx, batchID, sdk.BetaMessageBatchGetParams{})
	if err != nil {
		return provider.BatchResponse{}, err
	}
	return batchResponseFromSDK(batch), nil
}

// CancelBatch implements BatchClient
func (c *anthropicClient) CancelBatch(ctx context.Context, batchID string) (provider.BatchResponse, error) {
	batch, err := c.sdkClient.Beta.Messages.Batches.Cancel(ctx, batchID, sdk.BetaMessageBatchCancelParams{})
	if err != nil {
		return provider.BatchResponse{}, err
	}
	return batchResponseFromSDK(batch), nil
}

func batchResponseFromSDK(batch *sdk.BetaMessageBatch) provider.BatchResponse {
	resultsURL := ""
	if batch.ResultsURL != "" {
		resultsURL = batch.ResultsURL
	}
	return provider.BatchResponse{
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

func parseConversationItem(msg provider.ConversationItem) (string, []parsedBlock) {
	role := provider.NormalizeConversationRole(msg.Role)
	if role == provider.RoleSystem {
		return "", nil
	}

	var blocks []parsedBlock
	if call, ok := provider.ParseAssistantToolCallContent(msg.Content); ok {
		var toolInput map[string]any
		if err := json.Unmarshal(call.Arguments, &toolInput); err != nil {
			toolInput = map[string]any{keyInput: call.Input}
		}
		blocks = append(blocks, parsedBlock{
			isToolUse:    true,
			toolUseID:    call.CallID,
			toolUseName:  call.Name,
			toolUseInput: toolInput,
		})
	} else if msg.Role == provider.RoleTool {
		call, output := provider.ParseToolResultContent(msg.Content)
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
	messages []provider.ConversationItem,
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
		if role == provider.RoleUser {
			sdkRole = roleUser
		} else {
			sdkRole = roleAssistant
		}

		sdkBlocks := make([]BlockT, len(blocks))
		for j, b := range blocks {
			sdkBlocks[j] = buildBlock(b)
		}

		if i == len(messages)-1 && role == provider.RoleUser && len(sdkBlocks) > 0 {
			setCacheControl(&sdkBlocks[len(sdkBlocks)-1])
		}

		result = append(result, buildMessage(sdkRole, sdkBlocks))
	}
	return result
}

func buildSDKTools[T any](tools provider.ToolSet, buildFn func(t provider.LocalToolDefinition, isLast bool) T) []T {
	if len(tools.Local) == 0 {
		return nil
	}
	result := make([]T, 0, len(tools.Local))
	for i, tool := range tools.Local {
		result = append(result, buildFn(tool, i == len(tools.Local)-1))
	}
	return result
}

func toolProperties(t provider.LocalToolDefinition) map[string]any {
	return map[string]any{
		keyInput: map[string]any{
			"type":        "string",
			"description": provider.ToolInputDescription(t),
		},
	}
}

func toSDKTools(tools provider.ToolSet) []sdk.ToolUnionParam {
	return buildSDKTools(tools, func(t provider.LocalToolDefinition, isLast bool) sdk.ToolUnionParam {
		tParam := sdk.ToolParam{
			Name:        t.Name,
			Description: sdk.String(strings.TrimSpace(t.Description)),
			InputSchema: sdk.ToolInputSchemaParam{
				Properties: toolProperties(t),
				Required:   []string{keyInput},
			},
		}
		if isLast {
			tParam.CacheControl = sdk.NewCacheControlEphemeralParam()
		}
		return sdk.ToolUnionParam{OfTool: &tParam}
	})
}

func toSDKMessages(messages []provider.ConversationItem) []sdk.MessageParam {
	return buildSDKMessages(
		messages,
		sdk.MessageParamRoleUser,
		sdk.MessageParamRoleAssistant,
		func(b parsedBlock) sdk.ContentBlockParamUnion {
			if b.isToolUse {
				return sdk.ContentBlockParamUnion{
					OfToolUse: &sdk.ToolUseBlockParam{
						ID:    b.toolUseID,
						Name:  b.toolUseName,
						Input: b.toolUseInput,
					},
				}
			}
			if b.isToolResult {
				return sdk.ContentBlockParamUnion{
					OfToolResult: &sdk.ToolResultBlockParam{
						ToolUseID: b.toolResultID,
						Content: []sdk.ToolResultBlockParamContentUnion{
							{
								OfText: &sdk.TextBlockParam{
									Text: b.toolOutput,
								},
							},
						},
					},
				}
			}
			return sdk.NewTextBlock(b.text)
		},
		func(block *sdk.ContentBlockParamUnion) {
			if block.OfText != nil {
				block.OfText.CacheControl = sdk.NewCacheControlEphemeralParam()
			} else if block.OfToolResult != nil {
				block.OfToolResult.CacheControl = sdk.NewCacheControlEphemeralParam()
			}
		},
		func(role sdk.MessageParamRole, blocks []sdk.ContentBlockParamUnion) sdk.MessageParam {
			return sdk.MessageParam{
				Role:    role,
				Content: blocks,
			}
		},
	)
}

func toSDKBetaMessages(messages []provider.ConversationItem) []sdk.BetaMessageParam {
	return buildSDKMessages(
		messages,
		sdk.BetaMessageParamRoleUser,
		sdk.BetaMessageParamRoleAssistant,
		func(b parsedBlock) sdk.BetaContentBlockParamUnion {
			switch {
			case b.isToolUse:
				return sdk.BetaContentBlockParamUnion{
					OfToolUse: &sdk.BetaToolUseBlockParam{
						ID:    b.toolUseID,
						Name:  b.toolUseName,
						Input: b.toolUseInput,
					},
				}
			case b.isToolResult:
				return sdk.BetaContentBlockParamUnion{
					OfToolResult: &sdk.BetaToolResultBlockParam{
						ToolUseID: b.toolResultID,
						Content: []sdk.BetaToolResultBlockParamContentUnion{
							{
								OfText: &sdk.BetaTextBlockParam{
									Text: b.toolOutput,
								},
							},
						},
					},
				}
			default:
				return sdk.NewBetaTextBlock(b.text)
			}
		},
		func(block *sdk.BetaContentBlockParamUnion) {
			if block.OfText != nil {
				block.OfText.CacheControl = sdk.NewBetaCacheControlEphemeralParam()
			} else if block.OfToolResult != nil {
				block.OfToolResult.CacheControl = sdk.NewBetaCacheControlEphemeralParam()
			}
		},
		func(role sdk.BetaMessageParamRole, blocks []sdk.BetaContentBlockParamUnion) sdk.BetaMessageParam {
			return sdk.BetaMessageParam{
				Role:    role,
				Content: blocks,
			}
		},
	)
}

func toSDKBetaTools(tools provider.ToolSet) []sdk.BetaToolUnionParam {
	return buildSDKTools(tools, func(t provider.LocalToolDefinition, isLast bool) sdk.BetaToolUnionParam {
		tParam := sdk.BetaToolParam{
			Name:        t.Name,
			Description: sdk.String(strings.TrimSpace(t.Description)),
			InputSchema: sdk.BetaToolInputSchemaParam{
				Properties: toolProperties(t),
				Required:   []string{keyInput},
			},
		}
		if isLast {
			tParam.CacheControl = sdk.NewBetaCacheControlEphemeralParam()
		}
		return sdk.BetaToolUnionParam{OfTool: &tParam}
	})
}

func responseFromSDK(decoded *sdk.Message) provider.Response {
	var textParts []string
	var pendingCalls []provider.ToolInvocation

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
			pendingCalls = append(pendingCalls, provider.ToolInvocation{
				Kind:      provider.InvokeCustomTool,
				Name:      strings.TrimSpace(toolUse.Name),
				Input:     strings.TrimSpace(inputStr),
				Arguments: rawInput,
				CallID:    strings.TrimSpace(toolUse.ID),
			})
		}
	}

	finalText := strings.TrimSpace(strings.Join(textParts, "\n"))
	usage := provider.Usage{
		InputTokens:           int(decoded.Usage.InputTokens),
		OutputTokens:          int(decoded.Usage.OutputTokens),
		TotalTokens:           int(decoded.Usage.InputTokens + decoded.Usage.OutputTokens),
		ReasoningTokens:       int(decoded.Usage.OutputTokensDetails.ThinkingTokens),
		CacheReadInputTokens:  int(decoded.Usage.CacheReadInputTokens),
		CacheWriteInputTokens: int(decoded.Usage.CacheCreationInputTokens),
	}

	result := provider.Response{FinalText: finalText, Usage: usage}
	if finalText != "" {
		result.OutputItems = []provider.ConversationItem{provider.AssistantText(finalText)}
	}
	result.PendingCalls = pendingCalls
	if len(result.PendingCalls) > 0 {
		result.OutputItems = append(result.OutputItems, provider.AssistantToolCallItems(result.PendingCalls)...)
		result.FinalText = ""
	}
	return result
}
