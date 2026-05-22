package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
)

type anthropicClient struct {
	baseClient
	thinkingBudget int
}

func newAnthropicClient(cfg config.ProviderConfig) Client {
	return &anthropicClient{
		baseClient:     newBaseClient(cfg.Name, cfg.BaseURL, cfg.APIKey, cfg.Model),
		thinkingBudget: cfg.ThinkingBudget,
	}
}

func (c *anthropicClient) Complete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}

	_ = tools
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
	return responseFromText(content, Usage{
		InputTokens:  decoded.Usage.InputTokens,
		OutputTokens: decoded.Usage.OutputTokens,
		TotalTokens:  decoded.Usage.InputTokens + decoded.Usage.OutputTokens,
	}), nil
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

func toAnthropicMessages(messages []ConversationItem) []map[string]string {
	result := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		role := normalizeConversationRole(message.Role)
		if role == RoleSystem {
			continue
		}
		result = append(result, map[string]string{"role": role, "content": message.Content})
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
