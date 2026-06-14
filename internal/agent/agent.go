package agent

import (
	"context"
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

const defaultMaxToolIterations = 24

type Result struct {
	Context    ContextSnapshot
	Assistant  string
	AgentLabel string
	Steps      []Step
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
	ctx := tools.ToolContext{
		Workspace:       info.Workspace,
		ActiveMode:      info.ActiveMode,
		AvailableAgents: info.AvailableAgents,
		MaxOutputSize:   12000,
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
	return buildSystemPrompt(a.provider.ProviderKind(), info, a.tools.Specs(buildToolContext(info)), a.agentSystem)
}

func (a *Agent) Run(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.ConversationItem) (Result, error) {
	return a.run(ctx, info, userInput, priorHistory, nil)
}

func (a *Agent) RunStream(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.ConversationItem, onEvent func(StreamEvent) error) (Result, error) {
	return a.run(ctx, info, userInput, priorHistory, onEvent)
}

func (a *Agent) run(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.ConversationItem, onEvent func(StreamEvent) error) (Result, error) {
	if !a.Configured() {
		return Result{}, fmt.Errorf("agente no configurado")
	}
	startedAt := time.Now()

	history := append([]provider.ConversationItem{}, priorHistory...)
	history = append(history, provider.UserText(userInput))
	steps := []Step{{Kind: "user", Title: "prompt", Content: userInput}}
	totalUsage := provider.Usage{}
	seenToolCalls := make(map[string]struct{})
	contextSnapshot := ContextSnapshot{
		Signals:          info.SignalSummary(),
		Semantic:         info.SemanticSummary,
		RelevantFiles:    info.RelevantFilesSummary(),
		RelevantSnippets: info.RelevantSnippetsSummary(),
	}

	for i := 0; i < defaultMaxToolIterations; i++ {
		tracelog.Logf("agent iteration=%d messages=%d provider=%s", i+1, len(history), a.provider.Summary())

		currentHistory := make([]provider.ConversationItem, len(history))
		copy(currentHistory, history)

		userMsgIdx := len(priorHistory)
		if userMsgIdx < len(currentHistory) {
			if strings.EqualFold(info.ActiveMode, "plan") {
				frag := system.LoadFragment("plan_active")
				if frag != "" {
					currentHistory[userMsgIdx].Content += "\n\n" + frag
				}
			} else if strings.EqualFold(info.ActiveMode, "build") {
				frag := system.LoadFragment("build_switch")
				if frag != "" {
					currentHistory[userMsgIdx].Content += "\n\n" + frag
				}
			}
		}

		if i >= defaultMaxToolIterations-2 {
			frag := system.LoadFragment("max_steps")
			if frag != "" {
				currentHistory = append(currentHistory, provider.AssistantText(frag))
			}
		}

		resp, err := a.complete(ctx, info, currentHistory, onEvent)
		if err != nil {
			tracelog.Logf("agent completion error=%v", err)
			return Result{}, err
		}
		tracelog.Logf("agent completion tool=%t usage_in=%d usage_out=%d usage_total=%d", len(resp.PendingCalls) > 0, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens
		totalUsage.ReasoningTokens += resp.Usage.ReasoningTokens
		totalUsage.CacheReadInputTokens += resp.Usage.CacheReadInputTokens
		totalUsage.CacheWriteInputTokens += resp.Usage.CacheWriteInputTokens
		if a.debug {
			steps = append(steps, Step{Kind: "debug", Title: "provider", Content: fmt.Sprintf("completion %d tokens in:%d out:%d total:%d", i+1, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)})
		}

		pending := resp.PendingCalls
		if len(pending) == 0 {
			message := strings.TrimSpace(resp.FinalText)
			if message == "" {
				message = "No tengo una respuesta util todavia."
			}
			if len(resp.OutputItems) > 0 {
				history = append(history, resp.OutputItems...)
			} else {
				history = append(history, provider.AssistantText(message))
			}
			steps = append(steps, Step{Kind: "assistant", Title: "answer", Content: message})
			return Result{Assistant: message, Steps: steps, Usage: totalUsage, AgentLabel: a.provider.Summary(), Duration: time.Since(startedAt), Context: contextSnapshot, History: history}, nil
		}

		if len(resp.OutputItems) > 0 {
			history = append(history, resp.OutputItems...)
		}

		type toolResult struct {
			historyItem provider.ConversationItem
			steps       []Step
			idx         int
		}
		ch := make(chan toolResult, len(pending))
		var wg sync.WaitGroup
		var mu sync.Mutex

		for idx, call := range pending {
			toolName := strings.TrimSpace(call.Name)

			var availableTools []string
			for _, s := range a.tools.Specs(buildToolContext(info)) {
				availableTools = append(availableTools, s.Name)
			}
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
				return Result{}, fmt.Errorf("ciclo de tool detectado: %s %s", toolName, toolInput)
			}
			seenToolCalls[toolKey] = struct{}{}
			mu.Unlock()

			wg.Add(1)
			go func(idx int, call provider.ToolInvocation, toolName, toolInput string) {
				defer wg.Done()

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
					subSteps = append(subSteps, Step{Kind: "error", Title: toolName, Content: errText})

					mu.Lock()
					if onEvent != nil {
						_ = onEvent(StreamEvent{Kind: "error", Title: toolName, Content: errText})
					}
					mu.Unlock()

					ch <- toolResult{
						idx:         idx,
						steps:       subSteps,
						historyItem: provider.ToolResultForInvocation(call, errText),
					}
					return
				}

				toolOutput := strings.TrimSpace(strings.Join([]string{result.Summary, result.Output}, "\n\n"))
				tracelog.Logf("agent tool result name=%s summary=%q output_bytes=%d", toolName, result.Summary, len(result.Output))
				subSteps = append(subSteps, Step{Kind: "output", Title: toolName, Content: toolOutput})

				mu.Lock()
				if onEvent != nil {
					_ = onEvent(StreamEvent{Kind: "output", Title: toolName, Content: toolOutput})
				}
				mu.Unlock()

				ch <- toolResult{
					idx:         idx,
					steps:       subSteps,
					historyItem: provider.ToolResultForInvocation(call, toolOutput),
				}
			}(idx, call, toolName, toolInput)
		}

		wg.Wait()
		close(ch)

		orderedResults := make([]toolResult, len(pending))
		for res := range ch {
			orderedResults[res.idx] = res
		}

		for _, res := range orderedResults {
			steps = append(steps, res.steps...)
			history = append(history, res.historyItem)
		}
	}

	return Result{}, fmt.Errorf("se alcanzo el maximo de iteraciones de tools")
}

func maxToolIterations() int {
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

func (a *Agent) complete(ctx context.Context, info system.ContextInfo, messages []provider.ConversationItem, onEvent func(StreamEvent) error) (provider.Response, error) {
	tCtx := buildToolContext(info)
	toolSet := toolSet(a.tools.Specs(tCtx))
	systemPrompt := buildSystemPrompt(a.provider.ProviderKind(), info, a.tools.Specs(tCtx), a.agentSystem)
	if onEvent == nil {
		return a.provider.Complete(ctx, systemPrompt, messages, toolSet)
	}
	return a.provider.StreamComplete(ctx, systemPrompt, messages, toolSet, func(delta provider.Delta) error {
		if delta.ReasoningContent != "" {
			if err := onEvent(StreamEvent{Kind: "thinking_delta", ReasoningContent: delta.ReasoningContent}); err != nil {
				return err
			}
		}
		if delta.Content != "" {
			if err := onEvent(StreamEvent{Kind: "assistant_delta", Content: delta.Content}); err != nil {
				return err
			}
		}
		return nil
	})
}

func toolSet(specs []tools.Spec) provider.ToolSet {
	result := make([]provider.LocalToolDefinition, 0, len(specs))
	for _, spec := range specs {
		result = append(result, provider.LocalToolDefinition{
			Name:        spec.Name,
			Description: spec.Summary,
			InputType:   provider.ToolInputText,
			InputHint:   spec.Usage,
		})
	}
	return provider.ToolSet{Local: result}
}
