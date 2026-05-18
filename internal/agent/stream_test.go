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
func (f *fakeStreamingProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{"test"}, nil
}
func (f *fakeStreamingProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools []provider.ToolDefinition) (provider.Response, error) {
	return provider.Response{Message: "hola"}, nil
}
func (f *fakeStreamingProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.Message, tools []provider.ToolDefinition, onDelta func(string) error) (provider.Response, error) {
	_ = onDelta("{\"message\":\"ho")
	_ = onDelta("la\"}")
	return provider.Response{Message: "hola"}, nil
}

func TestRunStreamEmitsAssistantDeltas(t *testing.T) {
	a := New(&fakeStreamingProvider{}, tools.NewRegistry())
	var chunks []string
	result, err := a.RunStream(context.Background(), system.ContextInfo{}, "di hola", func(event StreamEvent) error {
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
func (f *fakePlainStreamingProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{"test"}, nil
}
func (f *fakePlainStreamingProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools []provider.ToolDefinition) (provider.Response, error) {
	return provider.Response{Message: "hola mundo"}, nil
}
func (f *fakePlainStreamingProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.Message, tools []provider.ToolDefinition, onDelta func(string) error) (provider.Response, error) {
	_ = onDelta("hola")
	_ = onDelta(" ")
	_ = onDelta("mundo")
	return provider.Response{Message: "hola mundo"}, nil
}

func TestRunStreamFallsBackToPlainDeltas(t *testing.T) {
	a := New(&fakePlainStreamingProvider{}, tools.NewRegistry())
	var chunks []string
	result, err := a.RunStream(context.Background(), system.ContextInfo{}, "di hola", func(event StreamEvent) error {
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
func (f *fakeToolStreamingProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{"test"}, nil
}
func (f *fakeToolStreamingProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.Message, tools []provider.ToolDefinition, onDelta func(string) error) (provider.Response, error) {
	return f.Complete(ctx, systemPrompt, messages, tools)
}
func (f *fakeToolStreamingProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools []provider.ToolDefinition) (provider.Response, error) {
	f.count++
	if f.count == 1 {
		_ = systemPrompt
		_ = messages
		_ = tools
		return provider.Response{ToolCall: &provider.ToolCall{Name: "fake", Input: "README.md"}}, nil
	}
	return provider.Response{Message: "hecho"}, nil
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
	result, err := a.RunStream(context.Background(), system.ContextInfo{}, "lee el archivo", func(event StreamEvent) error {
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

func TestStructuredStreamExtractorPreservesWhitespace(t *testing.T) {
	extractor := structuredStreamExtractor{}
	var got string
	for _, chunk := range []string{"{\"message\":\"hola", " ", "mundo\"}"} {
		got += extractor.Feed(chunk)
	}
	if got != "hola mundo" {
		t.Fatalf("expected whitespace preserved, got %q", got)
	}
}

func TestStructuredStreamExtractorSuppressesToolJSON(t *testing.T) {
	extractor := structuredStreamExtractor{}
	var got string
	for _, chunk := range []string{"{\"tool_name\":\"read\"", ",\"tool_input\":\"README.md\"}"} {
		got += extractor.Feed(chunk)
	}
	if got != "" {
		t.Fatalf("expected no text emitted for tool json, got %q", got)
	}
}
