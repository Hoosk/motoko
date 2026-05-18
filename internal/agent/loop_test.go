package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

type fakeLoopProvider struct {
	count int
}

func (f *fakeLoopProvider) Configured() bool { return true }
func (f *fakeLoopProvider) Summary() string  { return "fake:loop" }
func (f *fakeLoopProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{"loop"}, nil
}
func (f *fakeLoopProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.Message, tools []provider.ToolDefinition, onDelta func(string) error) (provider.Response, error) {
	return f.Complete(ctx, systemPrompt, messages, tools)
}
func (f *fakeLoopProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools []provider.ToolDefinition) (provider.Response, error) {
	f.count++
	return provider.Response{ToolCall: &provider.ToolCall{Name: "looptool", Input: "same"}}, nil
}

type fakeLoopTool struct{}

func (f *fakeLoopTool) Spec() tools.Spec {
	return tools.Spec{Name: "looptool", Summary: "loop tool", Usage: "looptool <arg>"}
}

func (f *fakeLoopTool) Run(ctx context.Context, args string) (tools.Result, error) {
	return tools.Result{Spec: f.Spec(), Summary: "ok", Output: "ok"}, nil
}

func TestRunDetectsRepeatedToolLoop(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&fakeLoopTool{})
	a := New(&fakeLoopProvider{}, registry)
	_, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo")
	if err == nil {
		t.Fatal("expected loop detection error")
	}
	if !strings.Contains(err.Error(), "ciclo de tool detectado") {
		t.Fatalf("unexpected error: %v", err)
	}
}
