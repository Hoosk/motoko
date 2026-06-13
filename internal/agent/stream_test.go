package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

type fakeStreamingProvider struct{}

func (f *fakeStreamingProvider) Configured() bool { return true }
func (f *fakeStreamingProvider) Summary() string  { return "fake:test" }
func (f *fakeStreamingProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "test"}}, nil
}
func (f *fakeStreamingProvider) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	return provider.ModelInfo{ID: model}, nil
}
func (f *fakeStreamingProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools provider.ToolSet) (provider.Response, error) {
	return provider.Response{FinalText: "hola", OutputItems: []provider.ConversationItem{provider.AssistantText("hola")}}, nil
}
func (f *fakeStreamingProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	_ = onDelta(provider.Delta{Content: "ho"})
	_ = onDelta(provider.Delta{Content: "la"})
	return provider.Response{FinalText: "hola", OutputItems: []provider.ConversationItem{provider.AssistantText("hola")}}, nil
}

func TestRunStreamEmitsAssistantDeltas(t *testing.T) {
	a := New(&fakeStreamingProvider{}, tools.NewRegistry())
	var chunks []string
	result, err := a.RunStream(context.Background(), system.ContextInfo{}, "di hola", nil, func(event StreamEvent) error {
		chunks = append(chunks, event.Content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Assistant != "hola" {
		t.Fatalf("expected final assistant message, got %#v", result)
	}
	if len(chunks) != 2 || chunks[0] != "ho" || chunks[1] != "la" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

type fakePlainStreamingProvider struct{}

func (f *fakePlainStreamingProvider) Configured() bool { return true }
func (f *fakePlainStreamingProvider) Summary() string  { return "fake:plain" }
func (f *fakePlainStreamingProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "test"}}, nil
}
func (f *fakePlainStreamingProvider) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	return provider.ModelInfo{ID: model}, nil
}
func (f *fakePlainStreamingProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools provider.ToolSet) (provider.Response, error) {
	return provider.Response{FinalText: "hola mundo", OutputItems: []provider.ConversationItem{provider.AssistantText("hola mundo")}}, nil
}
func (f *fakePlainStreamingProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	_ = onDelta(provider.Delta{Content: "hola"})
	_ = onDelta(provider.Delta{Content: " "})
	_ = onDelta(provider.Delta{Content: "mundo"})
	return provider.Response{FinalText: "hola mundo", OutputItems: []provider.ConversationItem{provider.AssistantText("hola mundo")}}, nil
}

func TestRunStreamFallsBackToPlainDeltas(t *testing.T) {
	a := New(&fakePlainStreamingProvider{}, tools.NewRegistry())
	var chunks []string
	result, err := a.RunStream(context.Background(), system.ContextInfo{}, "di hola", nil, func(event StreamEvent) error {
		chunks = append(chunks, event.Content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Assistant != "hola mundo" {
		t.Fatalf("expected final assistant message, got %#v", result)
	}
	if !reflect.DeepEqual(chunks, []string{"hola", " ", "mundo"}) {
		t.Fatalf("unexpected plain chunks: %#v", chunks)
	}
}

type fakeToolStreamingProvider struct {
	count int
}

func (f *fakeToolStreamingProvider) Configured() bool { return true }
func (f *fakeToolStreamingProvider) Summary() string  { return "fake:tool-stream" }
func (f *fakeToolStreamingProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "test"}}, nil
}
func (f *fakeToolStreamingProvider) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	return provider.ModelInfo{ID: model}, nil
}
func (f *fakeToolStreamingProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	return f.Complete(ctx, systemPrompt, messages, tools)
}
func (f *fakeToolStreamingProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools provider.ToolSet) (provider.Response, error) {
	f.count++
	if f.count == 1 {
		_ = systemPrompt
		_ = messages
		_ = tools
		return provider.Response{PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "fake", Input: "README.md"}}}, nil
	}
	return provider.Response{FinalText: "hecho", OutputItems: []provider.ConversationItem{provider.AssistantText("hecho")}}, nil
}

type fakeStreamTool struct{}

func (f *fakeStreamTool) Spec() tools.Spec {
	return tools.Spec{Name: "fake", Summary: "fake tool", Usage: "fake <arg>"}
}

func (f *fakeStreamTool) Run(ctx context.Context, args string) (tools.Result, error) {
	return tools.Result{Spec: f.Spec(), Summary: "ok", Output: "contenido"}, nil
}

func TestRunStreamSuppressesToolCallJSONAndEmitsToolEvents(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&fakeStreamTool{})
	a := New(&fakeToolStreamingProvider{}, registry)
	var events []StreamEvent
	result, err := a.RunStream(context.Background(), system.ContextInfo{}, "lee el archivo", nil, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Assistant != "hecho" {
		t.Fatalf("expected final assistant message, got %#v", result)
	}
	wantKinds := []string{"tool", "output"}
	gotKinds := make([]string, 0, len(events))
	for _, event := range events {
		gotKinds = append(gotKinds, event.Kind)
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("unexpected stream events: %#v", events)
	}
}
