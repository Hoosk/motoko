package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

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

type Client interface {
	Configured() bool
	Complete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition) (Response, error)
	StreamComplete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition, onDelta func(string) error) (Response, error)
	Summary() string
	ListModels(ctx context.Context) ([]string, error)
}

type clientFactory func(config.ProviderConfig) Client

var clientFactories = map[config.ProviderKind]clientFactory{
	config.ProviderKindOpenAICompatible: newOpenAIClient,
	config.ProviderKindAnthropic:       newAnthropicClient,
	config.ProviderKindGemini:          newGeminiClient,
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
	return &openAIClient{
		baseClient:     newBaseClient(cfg.Name, cfg.BaseURL, cfg.APIKey, cfg.Model),
		thinkingBudget: cfg.ThinkingBudget,
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

	reqBody := map[string]any{
		"model":       c.model,
		"temperature": 0.2,
		"messages":    toOpenAIMessages(systemPrompt, messages),
		"response_format": map[string]any{
			"type": "json_object",
		},
	}

	var decoded openAIResponse
	if err := postJSON(ctx, c.httpClient, c.baseURL+"/chat/completions", reqBody, map[string]string{
		"Authorization": "Bearer " + c.apiKey,
	}, &decoded); err != nil {
		return Response{}, err
	}
	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("respuesta vacia del provider")
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	parsed := parseStructuredResponse(content)
	if parsed.ToolCall != nil || parsed.Message != "" {
		parsed.Usage = Usage{
			InputTokens:  decoded.Usage.PromptTokens,
			OutputTokens: decoded.Usage.CompletionTokens,
			TotalTokens:  decoded.Usage.TotalTokens,
		}
		return parsed, nil
	}
	return Response{Message: content, Usage: Usage{InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens, TotalTokens: decoded.Usage.TotalTokens}}, nil
}

func (c *openAIClient) ListModels(ctx context.Context) ([]string, error) {
	if !c.listReady() {
		return nil, fmt.Errorf("provider no configurado")
	}

	var decoded struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, c.httpClient, c.baseURL+"/models", map[string]string{
		"Authorization": "Bearer " + c.apiKey,
	}, &decoded); err != nil {
		return nil, err
	}
	return collectModels(decoded.Data, func(item struct {
		ID string "json:\"id\""
	}) string {
		return item.ID
	}), nil
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
		reqBody["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": c.thinkingBudget,
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

func (c *anthropicClient) ListModels(ctx context.Context) ([]string, error) {
	if !c.listReady() {
		return nil, fmt.Errorf("provider no configurado")
	}

	var decoded struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, c.httpClient, c.baseURL+"/v1/models", map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
	}, &decoded); err != nil {
		return nil, err
	}
	return collectModels(decoded.Data, func(item struct {
		ID string "json:\"id\""
	}) string {
		return item.ID
	}), nil
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

func (c *geminiClient) ListModels(ctx context.Context) ([]string, error) {
	if !c.listReady() {
		return nil, fmt.Errorf("provider no configurado")
	}

	url := fmt.Sprintf("%s/models?key=%s", c.baseURL, c.apiKey)
	var decoded struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := getJSON(ctx, c.httpClient, url, nil, &decoded); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(decoded.Models))
	for _, model := range decoded.Models {
		name := strings.TrimPrefix(model.Name, "models/")
		if strings.Contains(name, "gemini") {
			models = append(models, name)
		}
	}
	return uniqueSorted(models), nil
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

func collectModels[T any](items []T, getter func(T) string) []string {
	models := make([]string, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(getter(item))
		if name != "" {
			models = append(models, name)
		}
	}
	return uniqueSorted(models)
}

func uniqueSorted(items []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	slices.Sort(result)
	return result
}

func toOpenAIMessages(systemPrompt string, messages []Message) []map[string]string {
	result := []map[string]string{{"role": "system", "content": systemPrompt}}
	for _, message := range messages {
		result = append(result, map[string]string{"role": message.Role, "content": message.Content})
	}
	return result
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

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
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
