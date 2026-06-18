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
func (f *fakeLoopProvider) ProviderKind() string { return "fake" }
func (f *fakeLoopProvider) Summary() string  { return "fake:loop" }
func (f *fakeLoopProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "loop"}}, nil
}
func (f *fakeLoopProvider) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	return provider.ModelInfo{ID: model}, nil
}
func (f *fakeLoopProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	return f.Complete(ctx, systemPrompt, messages, tools)
}
func (f *fakeLoopProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools provider.ToolSet) (provider.Response, error) {
	f.count++
	return provider.Response{PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "looptool", Input: "same"}}}, nil
}

type fakeLoopTool struct{}

func (f *fakeLoopTool) Spec() tools.Spec {
	return tools.Spec{Name: "looptool", Summary: "loop tool", Usage: "looptool <arg>"}
}

func (f *fakeLoopTool) Run(ctx context.Context, args string) (tools.Result, error) {
	return tools.Result{Spec: f.Spec(), Summary: "ok", Output: "ok"}, nil
}

type fakeMultiProvider struct {
	count int
}

func (f *fakeMultiProvider) Configured() bool { return true }
func (f *fakeMultiProvider) ProviderKind() string { return "fake" }
func (f *fakeMultiProvider) Summary() string  { return "fake:multi" }
func (f *fakeMultiProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{ID: "multi"}}, nil
}
func (f *fakeMultiProvider) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	return provider.ModelInfo{ID: model}, nil
}
func (f *fakeMultiProvider) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	return f.Complete(ctx, systemPrompt, messages, tools)
}
func (f *fakeMultiProvider) Complete(ctx context.Context, systemPrompt string, messages []provider.Message, tools provider.ToolSet) (provider.Response, error) {
	f.count++
	if f.count == 1 {
		return provider.Response{PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "looptool", CallID: "1", Input: "a"}, {Kind: provider.InvokeCustomTool, Name: "looptool", CallID: "2", Input: "b"}}}, nil
	}
	return provider.Response{FinalText: "ok", OutputItems: []provider.ConversationItem{provider.AssistantText("ok")}}, nil
}

func TestRunDetectsRepeatedToolLoop(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&fakeLoopTool{})
	a := New(&fakeLoopProvider{}, registry)
	_, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err == nil {
		t.Fatal("expected loop detection error")
	}
	if !strings.Contains(err.Error(), "ciclo de tool detectado") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMaxToolIterationsDefaultsToTwentyFour(t *testing.T) {
	t.Setenv("MOTOKO_MAX_ITERATIONS", "")
	if got := maxToolIterations(); got != defaultMaxToolIterations {
		t.Fatalf("expected default max iterations %d, got %d", defaultMaxToolIterations, got)
	}
}

func TestMaxToolIterationsAcceptsEnvOverride(t *testing.T) {
	t.Setenv("MOTOKO_MAX_ITERATIONS", "31")
	if got := maxToolIterations(); got != 31 {
		t.Fatalf("expected env override, got %d", got)
	}
}

func TestMaxToolIterationsFallsBackOnInvalidEnv(t *testing.T) {
	t.Setenv("MOTOKO_MAX_ITERATIONS", "invalid")
	if got := maxToolIterations(); got != defaultMaxToolIterations {
		t.Fatalf("expected invalid env to fall back, got %d", got)
	}
	t.Setenv("MOTOKO_MAX_ITERATIONS", "0")
	if got := maxToolIterations(); got != defaultMaxToolIterations {
		t.Fatalf("expected non-positive env to fall back, got %d", got)
	}
}

func TestRunHonorsMaxToolIterationsOverride(t *testing.T) {
	t.Setenv("MOTOKO_MAX_ITERATIONS", "1")

	registry := tools.NewRegistry()
	registry.Register(&fakeLoopTool{})
	provider := &fakeLoopProvider{}
	a := New(provider, registry)

	_, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err == nil {
		t.Fatal("expected max-iterations error")
	}
	if provider.count != 1 {
		t.Fatalf("expected a single completion attempt, got %d", provider.count)
	}
}

func TestRunExecutesMultipleToolCallsInSingleIteration(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&fakeLoopTool{})
	a := New(&fakeMultiProvider{}, registry)
	result, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Assistant != "ok" {
		t.Fatalf("expected final assistant answer, got %#v", result)
	}
	if len(result.History) < 4 {
		t.Fatalf("expected tool history entries persisted, got %#v", result.History)
	}
}
