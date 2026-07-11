package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

func TestRunStreamEmitsAssistantDeltas(t *testing.T) {
	a := New(&fakeProviderClient{streamFn: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
		_ = onDelta(provider.Delta{Content: "ho"})
		_ = onDelta(provider.Delta{Content: "la"})
		return provider.Response{FinalText: "hola", OutputItems: []provider.ConversationItem{provider.AssistantText("hola")}}, nil
	}}, tools.NewRegistry())
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

func TestRunStreamFallsBackToPlainDeltas(t *testing.T) {
	a := New(&fakeProviderClient{streamFn: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
		_ = onDelta(provider.Delta{Content: "hola"})
		_ = onDelta(provider.Delta{Content: " "})
		_ = onDelta(provider.Delta{Content: "mundo"})
		return provider.Response{FinalText: "hola mundo", OutputItems: []provider.ConversationItem{provider.AssistantText("hola mundo")}}, nil
	}}, tools.NewRegistry())
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
	providerClient := &fakeProviderClient{}
	count := 0
	providerClient.completeFn = func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
		count++
		if count == 1 {
			return provider.Response{PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "fake", Input: "README.md"}}}, nil
		}
		return provider.Response{FinalText: "hecho", OutputItems: []provider.ConversationItem{provider.AssistantText("hecho")}}, nil
	}
	providerClient.streamFn = func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
		return providerClient.Complete(ctx, systemPrompt, messages, tools)
	}
	a := New(providerClient, registry)
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
