package agent

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

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

type rejectedLoopTool struct{}

func (r *rejectedLoopTool) Spec() tools.Spec {
	return tools.Spec{Name: "rejecttool", Summary: "reject tool", Usage: "rejecttool"}
}

func (r *rejectedLoopTool) Run(context.Context, string) (tools.Result, error) {
	return tools.Result{}, tools.ErrChangeRejected
}

func TestRunStopsAfterRejectedChange(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&rejectedLoopTool{})
	a := New(&fakeProviderClient{summary: "fake:reject", models: []provider.ModelInfo{{ID: "reject"}}, completeFn: func(context.Context, string, []provider.ConversationItem, provider.ToolSet) (provider.Response, error) {
		return provider.Response{PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "rejecttool", CallID: "1", Input: ""}}}, nil
	}}, registry)

	result, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Assistant, "Execution cancelled") {
		t.Fatalf("expected cancellation assistant message, got %q", result.Assistant)
	}
	if len(result.History) < 3 {
		t.Fatalf("expected tool call, tool result, and cancellation in history, got %#v", result.History)
	}
}

// TestRunKeepsDynamicContextInStablePrefix records the messages sent on every
// iteration of one turn and checks that the request is append-only: everything
// up to and including the turn's user message (which carries the dynamic
// block) is byte-identical across iterations, and tool traffic only follows it.
func TestRunKeepsDynamicContextInStablePrefix(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&fakeLoopTool{})
	var sent [][]provider.ConversationItem
	a := New(&fakeProviderClient{summary: "fake:prefix", models: []provider.ModelInfo{{ID: "prefix"}}, completeFn: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
		sent = append(sent, append([]provider.ConversationItem(nil), messages...))
		switch len(sent) {
		case 1, 2:
			call := provider.ToolInvocation{Kind: provider.InvokeCustomTool, Name: "looptool", CallID: strings.Repeat("c", len(sent)), Input: strings.Repeat("x", len(sent))}
			return provider.Response{
				PendingCalls: []provider.ToolInvocation{call},
				OutputItems:  []provider.ConversationItem{provider.AssistantTurn("", "", []provider.ToolInvocation{call})},
			}, nil
		default:
			return provider.Response{FinalText: "done", OutputItems: []provider.ConversationItem{provider.AssistantText("done")}}, nil
		}
	}}, registry)

	info := system.ContextInfo{
		Workspace:       "motoko",
		Path:            "/tmp/motoko",
		Signals:         map[string]string{"GitTachikoma": "main", "CodeTachikoma": "ok", "DiffTachikoma": "2", "SearchTachikoma": "3"},
		OnDemandSignals: map[string]string{"DependencyTachikoma": "deps", "ZetaTachikoma": "z"},
	}
	prior := []provider.ConversationItem{provider.UserText("earlier question"), provider.AssistantText("earlier answer")}

	result, err := a.Run(context.Background(), info, "current question", prior)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 3 {
		t.Fatalf("expected 3 iterations, got %d", len(sent))
	}

	const turnIdx = 2
	first := sent[0]
	if len(first) != turnIdx+1 {
		t.Fatalf("expected prior history + turn message on first iteration, got %#v", first)
	}
	turnMsg := first[turnIdx]
	if turnMsg.Role != provider.RoleUser || !strings.HasPrefix(turnMsg.Content, "<environment>") || !strings.HasSuffix(turnMsg.Content, "\n\ncurrent question") {
		t.Fatalf("expected dynamic block merged ahead of the user input, got %#v", turnMsg)
	}

	for i := 1; i < len(sent); i++ {
		prev, cur := sent[i-1], sent[i]
		if len(cur) <= len(prev) {
			t.Fatalf("iteration %d: expected request to grow, got %d <= %d", i+1, len(cur), len(prev))
		}
		for j := range prev {
			if !reflect.DeepEqual(prev[j], cur[j]) {
				t.Fatalf("iteration %d: message %d changed between iterations\nprev: %#v\ncur:  %#v", i+1, j, prev[j], cur[j])
			}
		}
		for j := turnIdx + 1; j < len(cur); j++ {
			if strings.Contains(cur[j].Content, "<environment>") {
				t.Fatalf("iteration %d: dynamic block repeated after the turn message at %d", i+1, j)
			}
		}
		if last := cur[len(cur)-1]; last.Role != provider.RoleTool {
			t.Fatalf("iteration %d: expected tool result to be the tail, got %#v", i+1, last)
		}
	}

	for idx, item := range result.History {
		if strings.Contains(item.Content, "<environment>") || strings.Contains(item.Content, "<context>") {
			t.Fatalf("dynamic block leaked into returned history at %d: %q", idx, item.Content)
		}
	}
	if got := result.History[turnIdx].Content; got != "current question" {
		t.Fatalf("expected raw user input in history, got %q", got)
	}
}

func TestNewRunStateFreezesDynamicPromptTime(t *testing.T) {
	a := New(&fakeProviderClient{}, tools.NewRegistry())
	state := a.newRunState(system.ContextInfo{Workspace: "motoko"}, "hi", nil, 3)
	want := "Current Time: " + state.startedAt.Format(time.RFC3339)
	if !strings.Contains(state.dynamicPrompt, want) {
		t.Fatalf("expected dynamic prompt to use the run start time %q, got %q", want, state.dynamicPrompt)
	}
	if state.turnIndex != 0 || state.history[state.turnIndex].Content != "hi" {
		t.Fatalf("expected turn index to point at the user message, got %d %#v", state.turnIndex, state.history)
	}
}
