package openai

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

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
	baseURL            string
	apiKey             string
	model              string
	providerName       string
	httpClient         *http.Client
	sdkClient          openai.Client
	thinkingBudget     int
	contextWindow      int
	useChatCompletions bool
	useSDK             bool
}

func NewClient(cfg config.ProviderConfig) provider.Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	apiKey := strings.TrimSpace(cfg.APIKey)
	model := strings.TrimSpace(cfg.Model)
	httpClient := &http.Client{Timeout: 15 * time.Minute}

	sdkClient := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(&http.Client{Timeout: 15 * time.Minute}),
	)

	return &openAIClient{
		baseURL:            baseURL,
		apiKey:             apiKey,
		model:              model,
		providerName:       cfg.Name,
		httpClient:         httpClient,
		thinkingBudget:     cfg.ThinkingBudget,
		contextWindow:      cfg.ContextWindow,
		sdkClient:          sdkClient,
		useChatCompletions: cfg.Preset != config.ProviderPresetOpenAI && cfg.Preset != config.ProviderPresetLMStudio,
		useSDK:             cfg.UseSDK,
	}
}

func (c *openAIClient) Configured() bool {
	return c.baseURL != "" && c.apiKey != "" && c.model != ""
}

func (c *openAIClient) ConfigurationError() error {
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

func (c *openAIClient) listReady() bool {
	return c.baseURL != "" && c.apiKey != ""
}

func (c *openAIClient) ProviderKind() string {
	return c.providerName
}

func (c *openAIClient) Summary() string {
	return fmt.Sprintf("%s:%s", c.providerName, c.model)
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
	params := buildResponseParams(c.model, systemPrompt, messages, tools, c.thinkingBudget, sessionID)
	reqOpts := make([]option.RequestOption, 0)
	telemetryHeaders := map[string]string{}
	provider.ApplyTelemetryHeaders(c.providerName, telemetryHeaders, sessionID, requestID)
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
		"model": c.model,
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
		payload["parallel_tool_calls"] = false
	}

	headers := provider.BuildAuthHeaders(c.baseURL, c.apiKey)
	sessionID, requestID := provider.GetTelemetry(ctx)
	provider.ApplyTelemetryHeaders(c.providerName, headers, sessionID, requestID)

	if err := provider.PostJSON(ctx, c.httpClient, c.baseURL+"/chat/completions", payload, headers, &decoded); err != nil {
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
		Model:       openai.ChatModel(c.model),
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
		params.ParallelToolCalls = param.NewOpt(false)
	}

	headers := provider.BuildAuthHeaders(c.baseURL, c.apiKey)
	provider.ApplyTelemetryHeaders(c.providerName, headers, sessionID, requestID)
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
	if !c.listReady() {
		return nil, fmt.Errorf("provider not configured")
	}
	var decoded struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}

	listHeaders := provider.BuildAuthHeaders(c.baseURL, c.apiKey)

	if err := provider.GetJSON(ctx, c.httpClient, c.baseURL+"/models", listHeaders, &decoded); err != nil {
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
	if info, ok := provider.LookupModel(c.providerName, model); ok {
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
		p.MaxToolCalls = param.NewOpt(int64(1))
		p.ParallelToolCalls = param.NewOpt(false)
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
