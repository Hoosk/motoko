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
	thinkingBudget int
	sdkClient      openai.Client
	forceChat      bool
}

func newOpenAIClient(cfg config.ProviderConfig) Client {
	base := newBaseClient(cfg.Name, cfg.BaseURL, cfg.APIKey, cfg.Model)
	sdkClient := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
		option.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}),
	)
	return &openAIClient{
		baseClient:     base,
		thinkingBudget: cfg.ThinkingBudget,
		sdkClient:      sdkClient,
		forceChat:      cfg.Preset != config.ProviderPresetOpenAI,
	}
}

func (c *openAIClient) Complete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	if err := c.ConfigurationError(); err != nil {
		return Response{}, err
	}

	// Gemini and some other OpenAI-compatible providers don't support the Responses API yet.
	// We fall back to Chat Completions if we detect Gemini in the URL or if forceChat is set.
	if c.forceChat || strings.Contains(c.baseURL, "generativelanguage.googleapis.com") {
		return c.completeChat(ctx, systemPrompt, messages, tools)
	}

	params := buildResponseParams(c.model, systemPrompt, messages, tools, c.thinkingBudget)
	resp, err := c.sdkClient.Responses.New(ctx, params)
	if err != nil {
		return Response{}, err
	}
	return responseFromOpenAI(resp), nil
}

func (c *openAIClient) completeChat(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	var decoded chatCompletionResponse

	payload := map[string]interface{}{
		"model": c.model,
		"messages": append([]map[string]any{
			{"role": "system", "content": systemPrompt},
		}, toChatMessages(messages, isGoogleEndpoint(c.baseURL))...),
		"temperature": 0.2,
	}
	if toolDefs := chatCompletionTools(tools); len(toolDefs) > 0 {
		payload["tools"] = toolDefs
		payload["tool_choice"] = "auto"
		payload["parallel_tool_calls"] = false
	}

	headers := geminiAuthHeaders(c.baseURL, c.apiKey)

	if err := postJSON(ctx, c.httpClient, c.baseURL+"/chat/completions", payload, headers, &decoded); err != nil {
		return Response{}, err
	}

	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("no hay respuesta del modelo")
	}
	return responseFromChatCompletion(decoded), nil
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

	listHeaders := geminiAuthHeaders(c.baseURL, c.apiKey)

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
		result = append(result, ModelInfo{ID: id, ContextWindow: item.ContextLength})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
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
	if isOpenAIReasoningModel(model) {
		// Reasoning models (o-series, gpt-5.x) don't support temperature.
		if thinkingBudget > 0 {
			p.Reasoning = shared.ReasoningParam{
				Effort: shared.ReasoningEffort(budgetToReasoningEffort(thinkingBudget)),
			}
		}
	} else {
		p.Temperature = param.NewOpt(0.2)
	}
	return p
}
