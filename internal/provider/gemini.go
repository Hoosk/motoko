package provider

import (
	"context"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
)

type geminiClient struct {
	baseClient
	thinkingBudget int
}

func newGeminiClient(cfg config.ProviderConfig) Client {
	return &geminiClient{
		baseClient:     newBaseClient(cfg.Name, normalizeGeminiOpenAIBaseURL(cfg.BaseURL), cfg.APIKey, cfg.Model),
		thinkingBudget: cfg.ThinkingBudget,
	}
}

func (c *geminiClient) Complete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	return c.compatibleClient().Complete(ctx, systemPrompt, messages, tools)
}

func (c *geminiClient) StreamComplete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error) {
	return c.compatibleClient().StreamComplete(ctx, systemPrompt, messages, tools, onDelta)
}

func (c *geminiClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return c.compatibleClient().ListModels(ctx)
}

func (c *geminiClient) compatibleClient() *openAIClient {
	delegate := newOpenAIClient(config.ProviderConfig{
		Name:           c.providerName,
		Kind:           config.ProviderKindOpenAICompatible,
		BaseURL:        c.baseURL,
		APIKey:         c.apiKey,
		Model:          c.model,
		ThinkingBudget: c.thinkingBudget,
	})
	return delegate.(*openAIClient)
}

func normalizeGeminiOpenAIBaseURL(baseURL string) string {
	val := strings.TrimSpace(baseURL)
	if val == "" {
		return "https://generativelanguage.googleapis.com/v1beta/openai"
	}
	val = strings.TrimRight(val, "/")
	if strings.Contains(strings.ToLower(val), "generativelanguage.googleapis.com") && !strings.HasSuffix(val, "/openai") && !strings.HasSuffix(val, "openai") {
		return val + "/openai"
	}
	return val
}
