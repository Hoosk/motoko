package agent

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
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
	tool := &countingTool{name: "looptool"}
	registry.Register(tool)
	completions := 0
	a := New(&fakeProviderClient{summary: "fake:loop", models: []provider.ModelInfo{{ID: "loop"}}, completeFn: func(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
		completions++
		return provider.Response{PendingCalls: []provider.ToolInvocation{{Kind: provider.InvokeCustomTool, Name: "looptool", Input: "same"}}}, nil
	}}, registry)
	_, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err == nil {
		t.Fatal("expected loop detection error")
	}
	if !strings.Contains(err.Error(), "tool cycle detected") {
		t.Fatalf("unexpected error: %v", err)
	}
	if completions != toolRepeatAbortThreshold {
		t.Fatalf("expected abort on completion %d, got %d", toolRepeatAbortThreshold, completions)
	}
	if got := tool.runs.Load(); got != 1 {
		t.Fatalf("expected the repeated tool to run once, got %d", got)
	}
}

// countingTool records how many times it was executed.
type countingTool struct {
	name string
	runs atomic.Int32
}

func (c *countingTool) Spec() tools.Spec {
	return tools.Spec{Name: c.name, Summary: "counting tool", Usage: c.name + " <arg>"}
}

func (c *countingTool) Run(context.Context, string) (tools.Result, error) {
	c.runs.Add(1)
	return tools.Result{Spec: c.Spec(), Summary: "ok", Output: "ok"}, nil
}

// scriptedProvider returns the scripted responses in order and a final
// answer once the script is exhausted.
func scriptedProvider(responses ...[]provider.ToolInvocation) *fakeProviderClient {
	step := 0
	return &fakeProviderClient{summary: "fake:script", completeFn: func(context.Context, string, []provider.ConversationItem, provider.ToolSet) (provider.Response, error) {
		if step < len(responses) {
			calls := responses[step]
			step++
			return provider.Response{PendingCalls: calls}, nil
		}
		return provider.Response{FinalText: "done", OutputItems: []provider.ConversationItem{provider.AssistantText("done")}}, nil
	}}
}

func toolCall(name, callID, input string) provider.ToolInvocation {
	return provider.ToolInvocation{Kind: provider.InvokeCustomTool, Name: name, CallID: callID, Input: input}
}

func toolResultFor(history []provider.ConversationItem, callID string) (provider.ConversationItem, bool) {
	for _, item := range history {
		if item.Role == provider.RoleTool && item.ToolCallID == callID {
			return item, true
		}
	}
	return provider.ConversationItem{}, false
}

func TestRunNudgesFirstRepeatInsteadOfAborting(t *testing.T) {
	registry := tools.NewRegistry()
	tool := &countingTool{name: "looptool"}
	registry.Register(tool)
	// Distinct provider call IDs and differently formatted but equivalent
	// JSON arguments must still be detected as the same call.
	a := New(scriptedProvider(
		[]provider.ToolInvocation{toolCall("looptool", "call-1", `{"path": "a.go", "limit": 10}`)},
		[]provider.ToolInvocation{toolCall("looptool", "call-2", `{"limit":10,"path":"a.go"}`)},
	), registry)

	var events []StreamEvent
	result, err := a.RunStream(context.Background(), system.ContextInfo{}, "haz algo", nil, func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("expected repeat to be nudged, not aborted: %v", err)
	}
	if result.Assistant != "done" {
		t.Fatalf("expected the run to finish normally, got %q", result.Assistant)
	}
	if got := tool.runs.Load(); got != 1 {
		t.Fatalf("expected the repeated call not to be executed, got %d runs", got)
	}
	nudge, ok := toolResultFor(result.History, "call-2")
	if !ok {
		t.Fatalf("expected a tool result for the repeated call, history %#v", result.History)
	}
	if !strings.Contains(nudge.Content, "already ran looptool with these exact arguments") {
		t.Fatalf("unexpected nudge content %q", nudge.Content)
	}
	found := false
	for _, ev := range events {
		if ev.Kind == stepErrorKind && ev.Title == "looptool" && strings.Contains(ev.Content, "Not executed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a stream event for the skipped call, got %#v", events)
	}
}

func TestRunAbortsAtRepeatThreshold(t *testing.T) {
	registry := tools.NewRegistry()
	tool := &countingTool{name: "looptool"}
	registry.Register(tool)
	a := New(scriptedProvider(
		[]provider.ToolInvocation{toolCall("looptool", "call-1", `{"q":"x"}`)},
		[]provider.ToolInvocation{toolCall("looptool", "call-2", `{"q":"x"}`)},
		[]provider.ToolInvocation{toolCall("looptool", "call-3", `{ "q" : "x" }`)},
	), registry)

	_, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err == nil || !strings.Contains(err.Error(), "tool cycle detected") {
		t.Fatalf("expected tool cycle error at threshold, got %v", err)
	}
	if got := tool.runs.Load(); got != 1 {
		t.Fatalf("expected a single execution before the abort, got %d", got)
	}
}

func TestRunResetsRepeatDetectionAfterWriteTool(t *testing.T) {
	registry := tools.NewRegistry()
	tool := &countingTool{name: "looptool"}
	patch := &countingTool{name: "patch"}
	registry.Register(tool)
	registry.Register(patch)
	a := New(scriptedProvider(
		[]provider.ToolInvocation{toolCall("looptool", "1", `{"cmd":"go test"}`)},
		[]provider.ToolInvocation{toolCall("patch", "2", `{"diff":"..."}`)},
		[]provider.ToolInvocation{toolCall("looptool", "3", `{"cmd":"go test"}`)},
		[]provider.ToolInvocation{toolCall("looptool", "4", `{"cmd":"go test"}`)},
		[]provider.ToolInvocation{toolCall("patch", "5", `{"diff":"other"}`)},
		[]provider.ToolInvocation{toolCall("looptool", "6", `{"cmd":"go test"}`)},
	), registry)

	result, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err != nil {
		t.Fatalf("expected repeats separated by writes to be allowed: %v", err)
	}
	if got := tool.runs.Load(); got != 3 {
		t.Fatalf("expected looptool to run after each write (3 runs), got %d", got)
	}
	if got := patch.runs.Load(); got != 2 {
		t.Fatalf("expected both patches to run, got %d", got)
	}
	nudge, ok := toolResultFor(result.History, "4")
	if !ok || !strings.Contains(nudge.Content, "Not executed") {
		t.Fatalf("expected back-to-back repeat without a write to be nudged, got %#v", nudge)
	}
}

func TestRunWriteToolRepeatedBackToBackIsDetected(t *testing.T) {
	registry := tools.NewRegistry()
	patch := &countingTool{name: "patch"}
	registry.Register(patch)
	a := New(scriptedProvider(
		[]provider.ToolInvocation{toolCall("patch", "1", `{"diff":"same"}`)},
		[]provider.ToolInvocation{toolCall("patch", "2", `{"diff":"same"}`)},
	), registry)

	result, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := patch.runs.Load(); got != 1 {
		t.Fatalf("expected identical write not to run twice, got %d", got)
	}
	if nudge, ok := toolResultFor(result.History, "2"); !ok || !strings.Contains(nudge.Content, "Not executed") {
		t.Fatalf("expected repeated write to be nudged, got %#v", nudge)
	}
}

func TestRunBatchDuplicatesAreNudgedNotAborted(t *testing.T) {
	registry := tools.NewRegistry()
	tool := &countingTool{name: "looptool"}
	registry.Register(tool)
	a := New(scriptedProvider(
		[]provider.ToolInvocation{
			toolCall("looptool", "a", `{"q":"x"}`),
			toolCall("looptool", "b", `{"q":"x"}`),
			toolCall("looptool", "c", `{"q":"x"}`),
			toolCall("looptool", "d", `{"q":"y"}`),
		},
	), registry)

	result, err := a.Run(context.Background(), system.ContextInfo{}, "haz algo", nil)
	if err != nil {
		t.Fatalf("expected in-batch duplicates not to abort: %v", err)
	}
	if got := tool.runs.Load(); got != 2 {
		t.Fatalf("expected one run per distinct call (2), got %d", got)
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		if _, ok := toolResultFor(result.History, id); !ok {
			t.Fatalf("expected a tool result for call %s", id)
		}
	}
	for _, id := range []string{"b", "c"} {
		item, _ := toolResultFor(result.History, id)
		if !strings.Contains(item.Content, "same batch") {
			t.Fatalf("expected batch duplicate nudge for %s, got %q", id, item.Content)
		}
	}
}

func TestToolCallKeyCanonicalizesJSON(t *testing.T) {
	cases := []struct {
		a, b string
		same bool
	}{
		{`{"a":1,"b":[1,2]}`, "{ \"b\": [1, 2],\n \"a\": 1 }", true},
		{`{"a":1}`, `{"a":2}`, false},
		{"not json", "  not json  ", true},
		{"not json", "not  json", false},
		{`{"a":1}`, `{"a":1} trailing`, false},
	}
	for _, tc := range cases {
		got := toolCallKey("Tool", tc.a) == toolCallKey("tool", tc.b)
		if got != tc.same {
			t.Errorf("toolCallKey(%q) == toolCallKey(%q) = %t, want %t", tc.a, tc.b, got, tc.same)
		}
	}
	if toolCallKey("read", `{"a":1}`) == toolCallKey("grep", `{"a":1}`) {
		t.Error("different tools must not share a key")
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
