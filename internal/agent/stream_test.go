package agent

import (
	"context"
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
	_ = onDelta("ho")
	_ = onDelta("la")
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
