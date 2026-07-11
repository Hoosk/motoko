package agent

import (
	"context"
	"strings"

	"github.com/Hoosk/motoko/internal/provider"
)

type fakeProviderClient struct {
	configured *bool
	kind       string
	summary    string
	models     []provider.ModelInfo
	completeFn func(context.Context, string, []provider.ConversationItem, provider.ToolSet) (provider.Response, error)
	streamFn   func(context.Context, string, []provider.ConversationItem, provider.ToolSet, func(provider.Delta) error) (provider.Response, error)
}

func (f *fakeProviderClient) Configured() bool {
	if f == nil || f.configured == nil {
		return true
	}
	return *f.configured
}

func (f *fakeProviderClient) ProviderKind() string {
	if f == nil || strings.TrimSpace(f.kind) == "" {
		return "fake"
	}
	return f.kind
}

func (f *fakeProviderClient) Summary() string {
	if f == nil || strings.TrimSpace(f.summary) == "" {
		return f.ProviderKind() + ":test"
	}
	return f.summary
}

func (f *fakeProviderClient) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if len(f.models) == 0 {
		return []provider.ModelInfo{{ID: "test"}}, nil
	}
	return append([]provider.ModelInfo(nil), f.models...), nil
}

func (f *fakeProviderClient) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	for _, item := range f.models {
		if item.ID == model {
			return item, nil
		}
	}
	return provider.ModelInfo{ID: model}, nil
}

func (f *fakeProviderClient) Complete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
	if f.completeFn == nil {
		return provider.Response{}, nil
	}
	return f.completeFn(ctx, systemPrompt, messages, tools)
}

func (f *fakeProviderClient) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	if f.streamFn != nil {
		return f.streamFn(ctx, systemPrompt, messages, tools, onDelta)
	}
	resp, err := f.Complete(ctx, systemPrompt, messages, tools)
	if err != nil {
		return provider.Response{}, err
	}
	if onDelta != nil && strings.TrimSpace(resp.FinalText) != "" {
		if err := onDelta(provider.Delta{Content: resp.FinalText}); err != nil {
			return provider.Response{}, err
		}
	}
	return resp, nil
}
