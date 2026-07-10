package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

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
	a := New(&fakeProviderClient{summary: "fake:loop", models: []provider.ModelInfo{{ID: "loop"}}, completeFn: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
		return provider.Response{PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "looptool", Input: "same"}}}, nil
	}}, registry)
	_, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err == nil {
		t.Fatal("expected loop detection error")
	}
	if !strings.Contains(err.Error(), "tool cycle detected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMaxToolIterationsDefaultsToConfiguredLimit(t *testing.T) {
	t.Setenv("MOTOKO_MAX_ITERATIONS", "")
	if got := maxToolIterations(context.Background()); got != defaultMaxToolIterations {
		t.Fatalf("expected default max iterations %d, got %d", defaultMaxToolIterations, got)
	}
}

func TestMaxToolIterationsAcceptsEnvOverride(t *testing.T) {
	t.Setenv("MOTOKO_MAX_ITERATIONS", "31")
	if got := maxToolIterations(context.Background()); got != 31 {
		t.Fatalf("expected env override, got %d", got)
	}
}

func TestMaxToolIterationsFallsBackOnInvalidEnv(t *testing.T) {
	t.Setenv("MOTOKO_MAX_ITERATIONS", "invalid")
	if got := maxToolIterations(context.Background()); got != defaultMaxToolIterations {
		t.Fatalf("expected invalid env to fall back, got %d", got)
	}
	t.Setenv("MOTOKO_MAX_ITERATIONS", "0")
	if got := maxToolIterations(context.Background()); got != defaultMaxToolIterations {
		t.Fatalf("expected non-positive env to fall back, got %d", got)
	}
}

func TestRunHonorsMaxToolIterationsOverride(t *testing.T) {
	t.Setenv("MOTOKO_MAX_ITERATIONS", "1")

	registry := tools.NewRegistry()
	registry.Register(&fakeLoopTool{})
	count := 0
	providerClient := &fakeProviderClient{summary: "fake:loop", models: []provider.ModelInfo{{ID: "loop"}}, completeFn: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
		count++
		return provider.Response{PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "looptool", Input: "same"}}}, nil
	}}
	a := New(providerClient, registry)

	_, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err == nil {
		t.Fatal("expected max-iterations error")
	}
	if count != 1 {
		t.Fatalf("expected a single completion attempt, got %d", count)
	}
}

func TestRunExecutesMultipleToolCallsInSingleIteration(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&fakeLoopTool{})
	count := 0
	a := New(&fakeProviderClient{summary: "fake:multi", models: []provider.ModelInfo{{ID: "multi"}}, completeFn: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
		count++
		if count == 1 {
			return provider.Response{
				PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "looptool", CallID: "1", Input: "a"}, {Kind: provider.InvokeCustomTool, Name: "looptool", CallID: "2", Input: "b"}},
				Usage:        provider.Usage{InputTokens: 100, OutputTokens: 25, TotalTokens: 125, ReasoningTokens: 10},
			}, nil
		}
		return provider.Response{
			FinalText:   "ok",
			OutputItems: []provider.ConversationItem{provider.AssistantText("ok")},
			Usage:       provider.Usage{InputTokens: 140, OutputTokens: 30, TotalTokens: 170, ReasoningTokens: 12},
		}, nil
	}}, registry)
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
	if len(result.Iterations) != 2 {
		t.Fatalf("expected 2 iterations, got %#v", result.Iterations)
	}
	if result.Iterations[0].InputTokens != 100 || result.Iterations[1].InputTokens != 140 {
		t.Fatalf("unexpected iteration usage %#v", result.Iterations)
	}
	if result.Usage.ReasoningTokens != 22 {
		t.Fatalf("expected cumulative reasoning tokens, got %#v", result.Usage)
	}
}
