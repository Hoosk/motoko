package openai

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
)

func init() {
	provider.Register(config.ProviderKindOpenAICompatible, NewClient)
}

type openAIClient struct {
	provider.BaseClient
	sdkClient          openai.Client
	thinkingBudget     int
	contextWindow      int
	useChatCompletions bool
	useSDK             bool
}

func NewClient(cfg config.ProviderConfig) provider.Client {
	base := provider.NewBaseClient(cfg.Name, cfg.BaseURL, cfg.APIKey, cfg.Model)

	sdkClient := openai.NewClient(
		option.WithAPIKey(base.APIKey()),
		option.WithBaseURL(base.BaseURL()),
		option.WithHTTPClient(base.HTTPClient()),
	)

	return &openAIClient{
		BaseClient:         base,
		thinkingBudget:     cfg.ThinkingBudget,
		contextWindow:      cfg.ContextWindow,
		sdkClient:          sdkClient,
		useChatCompletions: cfg.Preset != config.ProviderPresetOpenAI && cfg.Preset != config.ProviderPresetLMStudio,
		useSDK:             cfg.UseSDK,
	}
}
func (c *openAIClient) Complete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
	if err := c.ConfigurationError(); err != nil {
		return provider.Response{}, err
	}

	if c.useChatCompletions {
		if c.useSDK {
			return c.completeChatSDK(ctx, systemPrompt, messages, tools)
		}
		return c.completeChat(ctx, systemPrompt, messages, tools)
	}

	sessionID, requestID := provider.GetTelemetry(ctx)
	params := buildResponseParams(c.Model(), systemPrompt, messages, tools, c.thinkingBudget, sessionID)
	reqOpts := make([]option.RequestOption, 0)
	telemetryHeaders := map[string]string{}
	provider.ApplyTelemetryHeaders(c.ProviderKind(), telemetryHeaders, sessionID, requestID)
	for k, v := range telemetryHeaders {
		reqOpts = append(reqOpts, option.WithHeader(k, v))
	}
	resp, err := c.sdkClient.Responses.New(ctx, params, reqOpts...)
	if err != nil {
		return provider.Response{}, err
	}
	return responseFromOpenAI(resp), nil
}

func (c *openAIClient) completeChat(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
	var decoded chatCompletionResponse

	payload := map[string]interface{}{
		"model": c.Model(),
		"messages": append([]map[string]any{
			{keyRole: "system", keyContent: systemPrompt},
		}, toChatMessages(messages)...),
		"temperature": 0.2,
	}
	if sessionID, _ := provider.GetTelemetry(ctx); sessionID != "" {
		payload["prompt_cache_key"] = sessionID
	}
	if toolDefs := chatCompletionTools(tools); len(toolDefs) > 0 {
		payload["tools"] = toolDefs
		payload["tool_choice"] = "auto"
		payload["parallel_tool_calls"] = true
	}

	headers := provider.BuildAuthHeaders(c.BaseURL(), c.APIKey())
	sessionID, requestID := provider.GetTelemetry(ctx)
	provider.ApplyTelemetryHeaders(c.ProviderKind(), headers, sessionID, requestID)

	if err := provider.PostJSON(ctx, c.HTTPClient(), c.BaseURL()+"/chat/completions", payload, headers, &decoded); err != nil {
		return provider.Response{}, err
	}

	if len(decoded.Choices) == 0 {
		return provider.Response{}, fmt.Errorf("no response from model")
	}
	return responseFromChatCompletion(decoded), nil
}

func (c *openAIClient) completeChatSDK(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
	sdkMessages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
	}
	sdkMessages = append(sdkMessages, toSDKChatMessages(messages)...)
	sessionID, requestID := provider.GetTelemetry(ctx)

	params := openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(c.Model()),
		Messages:    sdkMessages,
		Temperature: param.NewOpt(0.2),
	}
	if sessionID != "" {
		params.PromptCacheKey = param.NewOpt(sessionID)
	}
	if sdkTools := toSDKChatTools(tools); len(sdkTools) > 0 {
		params.Tools = sdkTools
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.NewOpt("auto"),
		}
		params.ParallelToolCalls = param.NewOpt(true)
	}

	headers := provider.BuildAuthHeaders(c.BaseURL(), c.APIKey())
	provider.ApplyTelemetryHeaders(c.ProviderKind(), headers, sessionID, requestID)
	reqOpts := make([]option.RequestOption, 0, len(headers))
	for k, v := range headers {
		reqOpts = append(reqOpts, option.WithHeader(k, v))
	}

	comp, err := c.sdkClient.Chat.Completions.New(ctx, params, reqOpts...)
	if err != nil {
		return provider.Response{}, err
	}
	return responseFromSDKChatCompletion(comp), nil
}

func (c *openAIClient) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if !c.ListReady() {
		return nil, fmt.Errorf("provider not configured")
	}
	var decoded struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}

	listHeaders := provider.BuildAuthHeaders(c.BaseURL(), c.APIKey())

	if err := provider.GetJSON(ctx, c.HTTPClient(), c.BaseURL()+"/models", listHeaders, &decoded); err != nil {
		return nil, err
	}
	result := make([]provider.ModelInfo, 0, len(decoded.Data))
	seen := make(map[string]struct{})
	for _, item := range decoded.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, provider.ModelInfo{
			ID:               id,
			ContextWindow:    item.ContextLength,
			SupportsThinking: true,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (c *openAIClient) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	// Attempt 1: Query the individual model via the SDK.
	m, err := c.sdkClient.Models.Get(ctx, model)
	if err == nil && m != nil {
		if field, ok := m.JSON.ExtraFields["context_length"]; ok {
			if raw := field.Raw(); raw != "" {
				if contextLength, atoiErr := strconv.Atoi(raw); atoiErr == nil && contextLength > 0 {
					return provider.ModelInfo{
						ID:               model,
						ContextWindow:    contextLength,
						SupportsThinking: true,
					}, nil
				}
			}
		}
	}

	// Attempt 2: Query the full model list if the API does not support individual lookups or omitted the field.
	list, err := c.ListModels(ctx)
	if err == nil {
		for _, item := range list {
			if item.ID == model {
				return item, nil
			}
		}
	}

	// Attempt 3: Fall back to the local catalog cache.
	if info, ok := provider.LookupModel(c.ProviderKind(), model); ok {
		return info, nil
	}

	// Fallback to configured or standard default context window
	fallback := c.contextWindow
	if fallback <= 0 {
		fallback = 131072 // standard fallback
	}
	return provider.ModelInfo{
		ID:               model,
		ContextWindow:    fallback,
		SupportsThinking: true,
	}, nil
}

// buildResponseParams constructs ResponseNewParams for the OpenAI Responses API.
// The system prompt goes into Instructions; messages become the Input item list.
func buildResponseParams(model, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, thinkingBudget int, promptCacheKey string) responses.ResponseNewParams {
	p := responses.ResponseNewParams{
		Model:        model,
		Instructions: param.NewOpt(systemPrompt),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: toResponsesInputItems(messages),
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			},
		},
	}
	if promptCacheKey != "" {
		p.PromptCacheKey = param.NewOpt(promptCacheKey)
	}
	if toolDefs := responseTools(tools); len(toolDefs) > 0 {
		p.Tools = toolDefs
		p.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto)}
		p.MaxToolCalls = param.NewOpt(int64(8))
		p.ParallelToolCalls = param.NewOpt(true)
	}
	if thinkingBudget > 0 {
		p.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(provider.BudgetToReasoningEffort(thinkingBudget)),
			Summary: shared.ReasoningSummaryAuto,
		}
	} else {
		p.Temperature = param.NewOpt(0.2)
	}
	return p
}
