package provider

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/Hoosk/motoko/internal/config"
)

type openAIClient struct {
	baseClient
	sdkClient          openai.Client
	thinkingBudget     int
	useChatCompletions bool
	useSDK             bool
}

func newOpenAIClient(cfg config.ProviderConfig) Client {
	base := newBaseClient(cfg.Name, cfg.BaseURL, cfg.APIKey, cfg.Model)
	sdkClient := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
		option.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}),
	)
	return &openAIClient{
		baseClient:         base,
		thinkingBudget:     cfg.ThinkingBudget,
		sdkClient:          sdkClient,
		useChatCompletions: cfg.Preset != config.ProviderPresetOpenAI,
		useSDK:             cfg.UseSDK,
	}
}

func (c *openAIClient) Complete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	if err := c.ConfigurationError(); err != nil {
		return Response{}, err
	}

	if c.useChatCompletions {
		if c.useSDK {
			return c.completeChatSDK(ctx, systemPrompt, messages, tools)
		}
		return c.completeChat(ctx, systemPrompt, messages, tools)
	}

	params := buildResponseParams(c.model, systemPrompt, messages, tools, c.thinkingBudget)
	sessionID, requestID := GetTelemetry(ctx)
	reqOpts := make([]option.RequestOption, 0)
	if sessionID != "" {
		reqOpts = append(reqOpts, option.WithHeader("X-Session-ID", sessionID))
		if requestID != "" {
			reqOpts = append(reqOpts, option.WithHeader("X-Request-ID", requestID))
		}
	}
	resp, err := c.sdkClient.Responses.New(ctx, params, reqOpts...)
	if err != nil {
		return Response{}, err
	}
	return responseFromOpenAI(resp), nil
}

func (c *openAIClient) completeChat(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	var decoded chatCompletionResponse

	payload := map[string]interface{}{
		keyModel: c.model,
		"messages": append([]map[string]any{
			{keyRole: "system", keyContent: systemPrompt},
		}, toChatMessages(messages)...),
		"temperature": 0.2,
	}
	if toolDefs := chatCompletionTools(tools); len(toolDefs) > 0 {
		payload["tools"] = toolDefs
		payload["tool_choice"] = "auto"
		payload["parallel_tool_calls"] = false
	}

	headers := buildAuthHeaders(c.baseURL, c.apiKey)
	sessionID, requestID := GetTelemetry(ctx)
	if sessionID != "" {
		headers["X-Session-ID"] = sessionID
		if requestID != "" {
			headers["X-Request-ID"] = requestID
		}
	}

	if err := postJSON(ctx, c.httpClient, c.baseURL+"/chat/completions", payload, headers, &decoded); err != nil {
		return Response{}, err
	}

	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("no hay respuesta del modelo")
	}
	return responseFromChatCompletion(decoded), nil
}

func (c *openAIClient) completeChatSDK(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	sdkMessages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
	}
	sdkMessages = append(sdkMessages, toSDKChatMessages(messages)...)

	params := openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(c.model),
		Messages:    sdkMessages,
		Temperature: param.NewOpt(0.2),
	}
	if sdkTools := toSDKChatTools(tools); len(sdkTools) > 0 {
		params.Tools = sdkTools
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.NewOpt("auto"),
		}
		params.ParallelToolCalls = param.NewOpt(false)
	}

	headers := buildAuthHeaders(c.baseURL, c.apiKey)
	sessionID, requestID := GetTelemetry(ctx)
	if sessionID != "" {
		headers["X-Session-ID"] = sessionID
		if requestID != "" {
			headers["X-Request-ID"] = requestID
		}
	}
	reqOpts := make([]option.RequestOption, 0, len(headers))
	for k, v := range headers {
		reqOpts = append(reqOpts, option.WithHeader(k, v))
	}

	comp, err := c.sdkClient.Chat.Completions.New(ctx, params, reqOpts...)
	if err != nil {
		return Response{}, err
	}
	return responseFromSDKChatCompletion(comp), nil
}

func (c *openAIClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if !c.listReady() {
		return nil, fmt.Errorf("provider no configurado")
	}
	var decoded struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}

	listHeaders := buildAuthHeaders(c.baseURL, c.apiKey)

	if err := getJSON(ctx, c.httpClient, c.baseURL+"/models", listHeaders, &decoded); err != nil {
		return nil, err
	}
	result := make([]ModelInfo, 0, len(decoded.Data))
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
		result = append(result, ModelInfo{
			ID:               id,
			ContextWindow:    item.ContextLength,
			SupportsThinking: true,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (c *openAIClient) GetModel(ctx context.Context, model string) (ModelInfo, error) {
	return ModelInfo{
		ID:               model,
		ContextWindow:    131072, // standard fallback
		SupportsThinking: true,
	}, nil
}

// buildResponseParams constructs ResponseNewParams for the OpenAI Responses API.
// The system prompt goes into Instructions; messages become the Input item list.
func buildResponseParams(model, systemPrompt string, messages []ConversationItem, tools ToolSet, thinkingBudget int) responses.ResponseNewParams {
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
	if toolDefs := responseTools(tools); len(toolDefs) > 0 {
		p.Tools = toolDefs
		p.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto)}
		p.MaxToolCalls = param.NewOpt(int64(1))
		p.ParallelToolCalls = param.NewOpt(false)
	}
	if thinkingBudget > 0 {
		p.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(budgetToReasoningEffort(thinkingBudget)),
			Summary: shared.ReasoningSummaryAuto,
		}
	} else {
		p.Temperature = param.NewOpt(0.2)
	}
	return p
}
