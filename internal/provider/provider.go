package provider

import (
	"bytes"
	"context"
	"encoding/json"
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

type ToolDefinition struct {
	Name        string
	Description string
	InputHint   string
}

type ToolCall struct {
	Name  string
	Input string
}

type Response struct {
	Message  string
	ToolCall *ToolCall
	Usage    Usage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type Message struct {
	Role    string
	Content string
}

type ModelInfo struct {
	ID            string
	ContextWindow int
}

type Client interface {
	Configured() bool
	Complete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition) (Response, error)
	StreamComplete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition, onDelta func(string) error) (Response, error)
	Summary() string
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

type clientFactory func(config.ProviderConfig) Client

var clientFactories = map[config.ProviderKind]clientFactory{
	config.ProviderKindOpenAICompatible: newOpenAIClient,
	config.ProviderKindAnthropic:        newAnthropicClient,
	config.ProviderKindGemini:           newGeminiClient,
}

func NewClient(cfg config.ProviderConfig) (Client, error) {
	cfg = config.NormalizeProvider(cfg)
	factory, ok := clientFactories[cfg.Kind]
	if !ok {
		return nil, fmt.Errorf("provider no soportado: %s", cfg.Kind)
	}
	return factory(cfg), nil
}

type baseClient struct {
	providerName string
	baseURL      string
	apiKey       string
	model        string
	httpClient   *http.Client
}

func newBaseClient(providerName, baseURL, apiKey, model string) baseClient {
	return baseClient{
		providerName: providerName,
		baseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:       strings.TrimSpace(apiKey),
		model:        strings.TrimSpace(model),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c baseClient) Configured() bool {
	return c.baseURL != "" && c.apiKey != "" && c.model != ""
}

func (c baseClient) listReady() bool {
	return c.baseURL != "" && c.apiKey != ""
}

func (c baseClient) Summary() string {
	return fmt.Sprintf("%s:%s", c.providerName, c.model)
}

type openAIClient struct {
	baseClient
	thinkingBudget int
	sdkClient      openai.Client
}
type anthropicClient struct {
	baseClient
	thinkingBudget int
}
type geminiClient struct {
	baseClient
	thinkingBudget int
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
	}
}

func newAnthropicClient(cfg config.ProviderConfig) Client {
	return &anthropicClient{
		baseClient:     newBaseClient(cfg.Name, cfg.BaseURL, cfg.APIKey, cfg.Model),
		thinkingBudget: cfg.ThinkingBudget,
	}
}

func newGeminiClient(cfg config.ProviderConfig) Client {
	return &geminiClient{
		baseClient:     newBaseClient(cfg.Name, cfg.BaseURL, cfg.APIKey, cfg.Model),
		thinkingBudget: cfg.ThinkingBudget,
	}
}

func (c *openAIClient) Complete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}

	params := buildResponseParams(c.model, systemPrompt, messages, c.thinkingBudget)
	resp, err := c.sdkClient.Responses.New(ctx, params)
	if err != nil {
		return Response{}, err
	}

	content := strings.TrimSpace(resp.OutputText())
	usage := Usage{
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
		TotalTokens:  int(resp.Usage.TotalTokens),
	}
	parsed := parseStructuredResponse(content)
	if parsed.ToolCall != nil || parsed.Message != "" {
		parsed.Usage = usage
		return parsed, nil
	}
	return Response{Message: content, Usage: usage}, nil
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
	if err := getJSON(ctx, c.httpClient, c.baseURL+"/models", map[string]string{
		"Authorization": "Bearer " + c.apiKey,
	}, &decoded); err != nil {
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
func buildResponseParams(model, systemPrompt string, messages []Message, thinkingBudget int) responses.ResponseNewParams {
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

// toResponsesInputItems converts []Message to a ResponseInputParam (slice of
// ResponseInputItemUnionParam) for use in the Responses API.
func toResponsesInputItems(messages []Message) responses.ResponseInputParam {
	items := make(responses.ResponseInputParam, 0, len(messages))
	for _, msg := range messages {
		role := responses.EasyInputMessageRole(msg.Role)
		items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, role))
	}
	return items
}

func (c *anthropicClient) Complete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}

	maxTokens := 4096
	reqBody := map[string]any{
		"model":      c.model,
		"max_tokens": maxTokens,
		"system":     systemPrompt,
		"messages":   toAnthropicMessages(messages),
	}
	if c.thinkingBudget > 0 {
		if c.thinkingBudget >= maxTokens {
			reqBody["max_tokens"] = c.thinkingBudget + 1024
		}
		if isAnthropicAdaptiveThinkingModel(c.model) {
			reqBody["thinking"] = map[string]any{
				"type":          "adaptive",
				"budget_tokens": c.thinkingBudget,
			}
		} else {
			reqBody["thinking"] = map[string]any{
				"type":          "enabled",
				"budget_tokens": c.thinkingBudget,
			}
		}
	}

	var decoded anthropicResponse
	if err := postJSON(ctx, c.httpClient, c.baseURL+"/v1/messages", reqBody, map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
	}, &decoded); err != nil {
		return Response{}, err
	}

	content := decoded.Text()
	parsed := parseStructuredResponse(content)
	if parsed.ToolCall != nil || parsed.Message != "" {
		parsed.Usage = Usage{
			InputTokens:  decoded.Usage.InputTokens,
			OutputTokens: decoded.Usage.OutputTokens,
			TotalTokens:  decoded.Usage.InputTokens + decoded.Usage.OutputTokens,
		}
		return parsed, nil
	}
	return Response{Message: strings.TrimSpace(content), Usage: Usage{InputTokens: decoded.Usage.InputTokens, OutputTokens: decoded.Usage.OutputTokens, TotalTokens: decoded.Usage.InputTokens + decoded.Usage.OutputTokens}}, nil
}

func (c *anthropicClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if !c.listReady() {
		return nil, fmt.Errorf("provider no configurado")
	}

	var decoded struct {
		Data []struct {
			ID             string `json:"id"`
			MaxInputTokens int    `json:"max_input_tokens"`
		} `json:"data"`
	}
	if err := getJSON(ctx, c.httpClient, c.baseURL+"/v1/models", map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
	}, &decoded); err != nil {
		return nil, err
	}
	result := make([]ModelInfo, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		result = append(result, ModelInfo{ID: id, ContextWindow: item.MaxInputTokens})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (c *geminiClient) Complete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}

	body := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{{"text": systemPrompt}},
		},
		"contents": toGeminiMessages(messages),
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"temperature":      0.2,
		},
	}
	if c.thinkingBudget > 0 {
		genConfig := body["generationConfig"].(map[string]any)
		if isGemini3Model(c.model) {
			genConfig["thinkingConfig"] = map[string]any{
				"thinkingLevel": budgetToGeminiThinkingLevel(c.thinkingBudget),
			}
		} else {
			genConfig["thinkingConfig"] = map[string]any{
				"thinkingBudget": c.thinkingBudget,
			}
		}
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	var decoded geminiResponse
	if err := postJSON(ctx, c.httpClient, url, body, nil, &decoded); err != nil {
		return Response{}, err
	}

	content := decoded.Text()
	parsed := parseStructuredResponse(content)
	if parsed.ToolCall != nil || parsed.Message != "" {
		parsed.Usage = Usage{
			InputTokens:  decoded.UsageMetadata.PromptTokenCount,
			OutputTokens: decoded.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  decoded.UsageMetadata.TotalTokenCount,
		}
		return parsed, nil
	}
	return Response{Message: strings.TrimSpace(content), Usage: Usage{InputTokens: decoded.UsageMetadata.PromptTokenCount, OutputTokens: decoded.UsageMetadata.CandidatesTokenCount, TotalTokens: decoded.UsageMetadata.TotalTokenCount}}, nil
}

func (c *geminiClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if !c.listReady() {
		return nil, fmt.Errorf("provider no configurado")
	}

	url := fmt.Sprintf("%s/models?key=%s", c.baseURL, c.apiKey)
	var decoded struct {
		Models []struct {
			Name            string `json:"name"`
			InputTokenLimit int    `json:"inputTokenLimit"`
		} `json:"models"`
	}
	if err := getJSON(ctx, c.httpClient, url, nil, &decoded); err != nil {
		return nil, err
	}

	models := make([]ModelInfo, 0, len(decoded.Models))
	for _, model := range decoded.Models {
		name := strings.TrimPrefix(model.Name, "models/")
		if strings.Contains(name, "gemini") {
			models = append(models, ModelInfo{ID: name, ContextWindow: model.InputTokenLimit})
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func postJSON(ctx context.Context, client *http.Client, url string, body any, headers map[string]string, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("provider error %d", resp.StatusCode)
	}
	return nil
}

func getJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("provider error %d", resp.StatusCode)
	}
	return nil
}

func toAnthropicMessages(messages []Message) []map[string]string {
	result := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		role := message.Role
		if role == "system" {
			continue
		}
		result = append(result, map[string]string{"role": role, "content": message.Content})
	}
	return result
}

func toGeminiMessages(messages []Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		role := "user"
		if message.Role == "assistant" {
			role = "model"
		}
		result = append(result, map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": message.Content}},
		})
	}
	return result
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (r anthropicResponse) Text() string {
	var parts []string
	for _, part := range r.Content {
		if part.Type == "text" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (r geminiResponse) Text() string {
	if len(r.Candidates) == 0 {
		return ""
	}
	var parts []string
	for _, part := range r.Candidates[0].Content.Parts {
		parts = append(parts, part.Text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

type structuredResponse struct {
	Message   string `json:"message"`
	ToolName  string `json:"tool_name"`
	ToolInput string `json:"tool_input"`
}

func parseStructuredResponse(raw string) Response {
	raw = normalizeStructuredPayload(raw)
	if raw == "" {
		return Response{}
	}

	var parsed structuredResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return Response{}
	}

	response := Response{Message: strings.TrimSpace(parsed.Message)}
	if strings.TrimSpace(parsed.ToolName) != "" {
		response.ToolCall = &ToolCall{Name: strings.TrimSpace(parsed.ToolName), Input: parsed.ToolInput}
	}
	return response
}

func normalizeStructuredPayload(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
		if strings.HasPrefix(trimmed, "json") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "json"))
		}
		if end := strings.LastIndex(trimmed, "```"); end >= 0 {
			trimmed = strings.TrimSpace(trimmed[:end])
		}
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		return strings.TrimSpace(trimmed[start : end+1])
	}
	return trimmed
}
