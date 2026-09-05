package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/tracelog"
)

const (
	defaultMaxToolIterations = 250
	inputBloatThresholdPct   = 15.0
	stepErrorKind            = "error"
)

type Result struct {
	Context    ContextSnapshot
	Assistant  string
	AgentLabel string
	Steps      []Step
	Iterations []provider.Usage
	History    []provider.ConversationItem
	Usage      provider.Usage
	Duration   time.Duration
}

type ContextSnapshot struct {
	Signals          string
	Semantic         string
	RelevantFiles    string
	RelevantSnippets string
}

type Step struct {
	Kind    string
	Title   string
	Content string
}

type Agent struct {
	provider    provider.Client
	tools       *tools.Registry
	agentSystem string
	debug       bool
}

type StreamEvent struct {
	Kind             string
	Title            string
	Content          string
	ReasoningContent string
}

func New(p provider.Client, toolsRegistry *tools.Registry) *Agent {
	return &Agent{provider: p, tools: toolsRegistry}
}

func (a *Agent) SetDebug(enabled bool) {
	a.debug = enabled
}

// SetAgentOverride sets the mode-specific system prompt injected before context.
func (a *Agent) SetAgentOverride(system string) {
	a.agentSystem = system
}

func (a *Agent) Configured() bool {
	return a != nil && a.provider != nil && a.provider.Configured() && a.tools != nil
}

func buildToolContext(info system.ContextInfo) tools.ToolContext {
	maxOutputSize := system.MaxToolOutputBytes(info.ContextWindow)
	ctx := tools.ToolContext{
		Workspace:       info.Workspace,
		ActiveMode:      info.ActiveMode,
		AvailableAgents: info.AvailableAgents,
		MaxOutputSize:   maxOutputSize,
	}
	for _, s := range info.AvailableSkills {
		ctx.AvailableSkills = append(ctx.AvailableSkills, s.Name)
	}
	return ctx
}

// SystemPrompt returns the current system prompt that would be sent to the provider.
func (a *Agent) SystemPrompt(info system.ContextInfo) string {
	if a == nil {
		return ""
	}
	staticPrompt := buildSystemPrompt(a.provider.ProviderKind(), info, a.tools.Specs(buildToolContext(info)), a.agentSystem)
	dynamicPrompt := buildDynamicPrompt(a.provider.ProviderKind(), info)
	if dynamicPrompt == "" {
		return staticPrompt
	}
	return staticPrompt + "\n\n" + dynamicPrompt
}

func (a *Agent) Run(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.ConversationItem) (Result, error) {
	return a.run(ctx, info, userInput, priorHistory, nil)
}

func (a *Agent) RunStream(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.ConversationItem, onEvent func(StreamEvent) error) (Result, error) {
	return a.run(ctx, info, userInput, priorHistory, onEvent)
}

// runState carries the mutable per-run bookkeeping across agent iterations.
type runState struct {
	startedAt      time.Time
	seenToolCalls  map[string]struct{}
	context        ContextSnapshot
	history        []provider.ConversationItem
	steps          []Step
	iterations     []provider.Usage
	availableTools []string
	specs          []tools.Spec
	totalUsage     provider.Usage
}

func (a *Agent) newRunState(info system.ContextInfo, userInput string, priorHistory []provider.ConversationItem, maxIterations int) runState {
	history := append([]provider.ConversationItem{}, priorHistory...)
	history = append(history, provider.UserText(userInput))
	tCtx := buildToolContext(info)
	specs := a.tools.Specs(tCtx)
	availableTools := make([]string, 0, len(specs))
	for _, s := range specs {
		availableTools = append(availableTools, s.Name)
	}
	return runState{
		history:        history,
		steps:          []Step{{Kind: "user", Title: "prompt", Content: userInput}},
		iterations:     make([]provider.Usage, 0, max(1, maxIterations)),
		seenToolCalls:  make(map[string]struct{}),
		availableTools: availableTools,
		specs:          specs,
		context: ContextSnapshot{
			Signals:          info.SignalSummary(),
			Semantic:         info.SemanticSummary,
			RelevantFiles:    info.RelevantFilesSummary(),
			RelevantSnippets: info.RelevantSnippetsSummary(),
		},
		startedAt: time.Now(),
	}
}

func (a *Agent) run(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.ConversationItem, onEvent func(StreamEvent) error) (Result, error) {
	if !a.Configured() {
		return Result{}, fmt.Errorf("agent not configured")
	}
	maxIterations := maxToolIterations(ctx)
	state := a.newRunState(info, userInput, priorHistory, maxIterations)

	for i := range maxIterations {
		tracelog.Logf("agent iteration=%d messages=%d provider=%s", i+1, len(state.history), a.provider.Summary())
		result, done, err := a.runIteration(ctx, info, i, maxIterations, &state, onEvent)
		if err != nil {
			return Result{}, err
		}
		if done {
			return result, nil
		}
	}

	return Result{}, fmt.Errorf("maximum tool iterations reached")
}

// runIteration performs a single completion + tool-execution step of the
// agent loop. It returns (result, true, nil) when the agent produced a final
// answer, and (zero, false, nil) when another iteration is required.
func (a *Agent) runIteration(ctx context.Context, info system.ContextInfo, i, maxIterations int, state *runState, onEvent func(StreamEvent) error) (Result, bool, error) {
	currentHistory := make([]provider.ConversationItem, len(state.history))
	copy(currentHistory, state.history)

	if i >= maxIterations-2 {
		if frag := system.LoadFragment("max_steps"); frag != "" {
			currentHistory = append(currentHistory, provider.AssistantText(frag))
		}
	}

	resp, err := a.complete(ctx, info, currentHistory, onEvent, state.specs)
	if err != nil {
		tracelog.Logf("agent completion error=%v", err)
		return Result{}, false, err
	}
	state.iterations = append(state.iterations, resp.Usage)
	logIterationUsage(i, resp, state.iterations)
	accumulateUsage(&state.totalUsage, resp.Usage)
	if a.debug {
		state.steps = append(state.steps, Step{Kind: "debug", Title: "provider", Content: fmt.Sprintf("completion %d tokens in:%d out:%d total:%d", i+1, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)})
	}

	if len(resp.PendingCalls) == 0 {
		message := strings.TrimSpace(resp.FinalText)
		if message == "" {
			message = "No useful response yet."
		}
		logIterationBloatSummary(state.iterations)
		if len(resp.OutputItems) > 0 {
			state.history = append(state.history, resp.OutputItems...)
		} else {
			state.history = append(state.history, provider.AssistantText(message))
		}
		state.steps = append(state.steps, Step{Kind: "assistant", Title: "answer", Content: message})
		return Result{
			Assistant:  message,
			Steps:      state.steps,
			Iterations: state.iterations,
			Usage:      state.totalUsage,
			AgentLabel: a.provider.Summary(),
			Duration:   time.Since(state.startedAt),
			Context:    state.context,
			History:    state.history,
		}, true, nil
	}

	if len(resp.OutputItems) > 0 {
		state.history = append(state.history, resp.OutputItems...)
	}

	toolSteps, toolHistory, rejected, rejection, err := a.executeTools(ctx, resp.PendingCalls, state.availableTools, onEvent, state.seenToolCalls)
	if err != nil {
		return Result{}, false, err
	}
	state.steps = append(state.steps, toolSteps...)
	state.history = append(state.history, toolHistory...)
	if rejected {
		message := "Execution cancelled: a file change was rejected."
		if strings.TrimSpace(rejection) != "" {
			message = "Execution cancelled: " + rejection
		}
		state.steps = append(state.steps, Step{Kind: stepErrorKind, Title: "approval", Content: message})
		state.history = append(state.history, provider.AssistantText(message))
		return Result{
			Assistant:  message,
			Steps:      state.steps,
			Iterations: state.iterations,
			Usage:      state.totalUsage,
			AgentLabel: a.provider.Summary(),
			Duration:   time.Since(state.startedAt),
			Context:    state.context,
			History:    state.history,
		}, true, nil
	}
	return Result{}, false, nil
}

// logIterationUsage writes completion diagnostics for a single iteration,
// including the input growth between consecutive iterations.
func logIterationUsage(i int, resp provider.Response, iterations []provider.Usage) {
	tracelog.Logf(
		"agent completion iteration=%d tool=%t usage_in=%d usage_out=%d usage_total=%d reasoning=%d cache_read=%d cache_write=%d",
		i+1,
		len(resp.PendingCalls) > 0,
		resp.Usage.InputTokens,
		resp.Usage.OutputTokens,
		resp.Usage.TotalTokens,
		resp.Usage.ReasoningTokens,
		resp.Usage.CacheReadInputTokens,
		resp.Usage.CacheWriteInputTokens,
	)
	if len(iterations) > 1 {
		prevInput := iterations[len(iterations)-2].InputTokens
		inputDelta := resp.Usage.InputTokens - prevInput
		pct := percentDelta(inputDelta, prevInput)
		tracelog.Logf(
			"agent iteration=%d input_delta=%+d input_delta_pct=%.1f input_bloat=%t",
			i+1,
			inputDelta,
			pct,
			inputDelta > 0 && pct >= inputBloatThresholdPct,
		)
	}
}

// accumulateUsage folds a single iteration's usage into the running total.
func accumulateUsage(total *provider.Usage, u provider.Usage) {
	total.InputTokens += u.InputTokens
	total.OutputTokens += u.OutputTokens
	total.TotalTokens += u.TotalTokens
	total.ReasoningTokens += u.ReasoningTokens
	total.CacheReadInputTokens += u.CacheReadInputTokens
	total.CacheWriteInputTokens += u.CacheWriteInputTokens
	total.SystemStaticChars += u.SystemStaticChars
	total.SystemDynamicChars += u.SystemDynamicChars
	total.ToolsChars += u.ToolsChars
	total.HistoryChars += u.HistoryChars
}

type toolResult struct {
	rejection   string
	historyItem provider.ConversationItem
	steps       []Step
	idx         int
	rejected    bool
}

// executeTools runs a batch of tool invocations in parallel and returns the
// accumulated steps and history items in the original call order.
func (a *Agent) executeTools(ctx context.Context, pending []provider.ToolInvocation, availableTools []string, onEvent func(StreamEvent) error, seenToolCalls map[string]struct{}) ([]Step, []provider.ConversationItem, bool, string, error) {
	ch := make(chan toolResult, len(pending))
	var wg sync.WaitGroup
	var mu sync.Mutex
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for idx, call := range pending {
		toolName := strings.TrimSpace(call.Name)
		if repairedName := tools.RepairToolName(toolName, availableTools); repairedName != "" {
			if repairedName != toolName {
				tracelog.Logf("agent tool repair from=%s to=%s", toolName, repairedName)
				toolName = repairedName
			}
		}
		toolInput := strings.TrimSpace(call.Input)
		if toolInput == "" && len(call.Arguments) > 0 {
			toolInput = strings.TrimSpace(string(call.Arguments))
		}
		toolKey := toolName + "\x00" + toolInput + "\x00" + strings.TrimSpace(call.CallID)

		mu.Lock()
		if _, seen := seenToolCalls[toolKey]; seen {
			mu.Unlock()
			return nil, nil, false, "", fmt.Errorf("tool cycle detected: %s %s", toolName, toolInput)
		}
		seenToolCalls[toolKey] = struct{}{}
		mu.Unlock()

		wg.Add(1)
		go func(idx int, call provider.ToolInvocation, toolName, toolInput string) {
			defer wg.Done()
			res := a.runTool(execCtx, idx, call, toolName, toolInput, onEvent, &mu)
			if res.rejected {
				cancel()
			}
			ch <- res
		}(idx, call, toolName, toolInput)
	}

	wg.Wait()
	close(ch)

	orderedResults := make([]toolResult, len(pending))
	for res := range ch {
		orderedResults[res.idx] = res
	}

	var toolSteps []Step
	var toolHistory []provider.ConversationItem
	rejected := false
	rejection := ""
	for _, res := range orderedResults {
		toolSteps = append(toolSteps, res.steps...)
		toolHistory = append(toolHistory, res.historyItem)
		if res.rejected && !rejected {
			rejected = true
			rejection = res.rejection
		}
	}
	return toolSteps, toolHistory, rejected, rejection, nil
}

// runTool executes a single tool invocation in a worker goroutine and
// produces the ordered result and history item for it.
func (a *Agent) runTool(ctx context.Context, idx int, call provider.ToolInvocation, toolName, toolInput string, onEvent func(StreamEvent) error, mu *sync.Mutex) toolResult {
	var subSteps []Step
	subSteps = append(subSteps, Step{Kind: "tool", Title: toolName, Content: toolInput})

	mu.Lock()
	if onEvent != nil {
		_ = onEvent(StreamEvent{Kind: "tool", Title: toolName, Content: toolInput})
	}
	mu.Unlock()

	result, err := a.tools.Run(ctx, toolName, toolInput)
	if err != nil {
		tracelog.Logf("agent tool error name=%s err=%v", toolName, err)
		errText := fmt.Sprintf("tool error: %v", err)
		subSteps = append(subSteps, Step{Kind: stepErrorKind, Title: toolName, Content: errText})

		mu.Lock()
		if onEvent != nil {
			_ = onEvent(StreamEvent{Kind: stepErrorKind, Title: toolName, Content: errText})
		}
		mu.Unlock()

		return toolResult{
			idx:         idx,
			steps:       subSteps,
			rejection:   err.Error(),
			rejected:    errors.Is(err, tools.ErrChangeRejected),
			historyItem: provider.ToolResultForInvocation(call, errText),
		}
	}

	toolOutput := strings.TrimSpace(strings.Join([]string{result.Summary, result.Output}, "\n\n"))
	tracelog.Logf("agent tool result name=%s summary=%q output_bytes=%d", toolName, result.Summary, len(result.Output))
	subSteps = append(subSteps, Step{Kind: "output", Title: toolName, Content: toolOutput})

	mu.Lock()
	if onEvent != nil {
		_ = onEvent(StreamEvent{Kind: "output", Title: toolName, Content: toolOutput})
	}
	mu.Unlock()

	return toolResult{
		idx:         idx,
		steps:       subSteps,
		historyItem: provider.ToolResultForInvocation(call, toolOutput),
	}
}

func maxToolIterations(ctx context.Context) int {
	if cfg := tools.GetConfig(ctx); cfg != nil && cfg.MaxIterations > 0 {
		return cfg.MaxIterations
	}
	value := strings.TrimSpace(os.Getenv("MOTOKO_MAX_ITERATIONS"))
	if value == "" {
		return defaultMaxToolIterations
	}
	iterations, err := strconv.Atoi(value)
	if err != nil || iterations < 1 {
		return defaultMaxToolIterations
	}
	return iterations
}

func (a *Agent) complete(ctx context.Context, info system.ContextInfo, messages []provider.ConversationItem, onEvent func(StreamEvent) error, specs []tools.Spec) (provider.Response, error) {
	toolSet := toolSet(specs)
	systemPrompt := buildSystemPrompt(a.provider.ProviderKind(), info, specs, a.agentSystem)
	dynamicPrompt := buildDynamicPrompt(a.provider.ProviderKind(), info)
	providerMessages := append([]provider.ConversationItem(nil), messages...)
	if dynamicPrompt != "" {
		providerMessages = append(providerMessages, provider.UserText(dynamicPrompt))
	}

	var resp provider.Response
	var err error
	if onEvent == nil {
		resp, err = a.provider.Complete(ctx, systemPrompt, providerMessages, toolSet)
	} else {
		resp, err = a.provider.StreamComplete(ctx, systemPrompt, providerMessages, toolSet, func(delta provider.Delta) error {
			if delta.ReasoningContent != "" {
				if evErr := onEvent(StreamEvent{Kind: "thinking_delta", ReasoningContent: delta.ReasoningContent}); evErr != nil {
					return evErr
				}
			}
			if delta.Content != "" {
				if evErr := onEvent(StreamEvent{Kind: "assistant_delta", Content: delta.Content}); evErr != nil {
					return evErr
				}
			}
			return nil
		})
	}

	if err != nil {
		return resp, err
	}

	// Calculate character metrics
	resp.Usage.SystemStaticChars = len(systemPrompt)
	resp.Usage.SystemDynamicChars = len(dynamicPrompt)

	toolsSize := 0
	for _, spec := range specs {
		toolsSize += len(spec.Name) + len(spec.Summary) + len(spec.Usage)
	}
	resp.Usage.ToolsChars = toolsSize

	historySize := 0
	for _, msg := range providerMessages {
		historySize += len(msg.Role) + len(msg.Content)
	}
	resp.Usage.HistoryChars = historySize

	return resp, nil
}

func toolSet(specs []tools.Spec) provider.ToolSet {
	result := make([]provider.LocalToolDefinition, 0, len(specs))
	for _, spec := range specs {
		result = append(result, provider.LocalToolDefinition{
			Name:        spec.Name,
			Description: spec.Summary,
			InputType:   provider.ToolInputText,
			InputHint:   spec.Usage,
			Schema:      spec.InputSchema,
		})
	}
	return provider.ToolSet{Local: result}
}

func percentDelta(delta, base int) float64 {
	if base <= 0 {
		return 0
	}
	return float64(delta) / float64(base) * 100
}

func logIterationBloatSummary(iterations []provider.Usage) {
	if len(iterations) == 0 {
		return
	}
	firstInput := iterations[0].InputTokens
	lastInput := iterations[len(iterations)-1].InputTokens
	growth := lastInput - firstInput
	pct := percentDelta(growth, firstInput)
	tracelog.Logf(
		"agent turn done iterations=%d input_growth=%+d input_growth_pct=%.1f input_bloat=%t",
		len(iterations),
		growth,
		pct,
		growth > 0 && pct >= inputBloatThresholdPct,
	)
}
