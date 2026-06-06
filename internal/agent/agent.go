package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// SystemPrompt returns the current system prompt that would be sent to the provider.
func (a *Agent) SystemPrompt(info system.ContextInfo) string {
	if a == nil {
		return ""
	}
	return buildSystemPrompt(info, a.tools.Specs(), a.agentSystem)
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

	for i := 0; i < maxToolIterations(); i++ {
		tracelog.Logf("agent iteration=%d messages=%d provider=%s", i+1, len(history), a.provider.Summary())
		resp, err := a.complete(ctx, info, history, onEvent)
		if err != nil {
			tracelog.Logf("agent completion error=%v", err)
			return Result{}, err
		}
		tracelog.Logf("agent completion tool=%t usage_in=%d usage_out=%d usage_total=%d", len(resp.PendingCalls) > 0, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens
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
	toolSet := toolSet(a.tools.Specs())
	systemPrompt := buildSystemPrompt(info, a.tools.Specs(), a.agentSystem)
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

func buildSystemPrompt(info system.ContextInfo, specs []tools.Spec, agentSystem string) string {
	var lines []string
	lines = append(lines,
		"You are Motoko, a senior coding agent working directly in the user's terminal and repository.",
		"When answering the user, write plain text directly so it can stream cleanly to the terminal.",
		"When you need a tool, use the provider's native tool/function call mechanism instead of printing JSON that describes a tool call.",
		"",
		"--- OPERATING RULES ---",
		"- TACHIKOMA FIRST: Always check '[Background Signals]' and '[Context]' sections before using any tool.",
		"- AGENTS & DESIGN RULES: If 'AGENTS.md' guidelines are present, strictly follow them for code conventions, build and test setups. If 'DESIGN.md' specifications are present, strictly adhere to them for any UI, TUI, or visual styling.",
		"- WEB SEARCH & FETCH PROTOCOL: When you use 'web_search', do not just rely on the search snippets. Always identify the most relevant URL(s) and use the 'web_fetch' tool to visit and read their full contents to obtain accurate details. Limit search and fetch activities to a maximum of 2-3 queries per task to avoid excessive network load.",
		"- If a signal mentions 'available on-demand', use the 'inspect' tool for that worker before using 'read', 'grep', or 'bash'.",
		"- Use tools only to explore parts of the codebase NOT already covered by the provided context.",
		"- If you use a tool, request only one tool at a time. The system will return the result to you.",
		"- DO NOT invent file names, functions, or command outputs.",
		"- Prefer finishing the task end-to-end instead of stopping at analysis.",
		"- If the existing context already answers the question, answer directly without unnecessary tool calls.",
		"",
		"--- BRAIN PROTOCOL ---",
		"You have a persistent session brain — a set of markdown files that survive across turns.",
		"Use brain_write, brain_read, and brain_list to manage your brain files.",
		"",
		"MANDATORY BEHAVIORS:",
		"- When starting a multi-step task, ALWAYS create plan.md with your approach BEFORE writing code.",
		"- When a plan exists in your brain, ALWAYS read it at the start of your turn and follow it.",
		"- Track progress in tasks.md — mark items as completed with [x] as you finish them.",
		"- When all work is done, write summary.md with what was accomplished.",
		"- If the user asks you to plan, save the plan to plan.md, not just in your response.",
		"",
		"BRAIN FILES CONVENTION:",
		"- plan.md    — Current implementation plan (goals, approach, file changes)",
		"- tasks.md   — Checklist of tasks with completion status",
		"- summary.md — Post-completion summary of what was done",
		"- notes.md   — Free-form notes, discoveries, or context for future turns",
		"",
	)
	if agentSystem != "" {
		lines = append(lines, "--- AGENT MODE ---", agentSystem, "")
	}
	if len(info.AvailableSkills) > 0 {
		lines = append(lines,
			"--- AVAILABLE SKILLS ---",
			"The following skills provide specialized instructions for specific tasks.",
			"When a task matches a skill's description, you MUST call the 'activate_skill' tool with the skill's name to load its full instructions BEFORE proceeding.",
			"",
			"<available_skills>",
		)
		for _, s := range info.AvailableSkills {
			lines = append(lines,
				"  <skill>",
				fmt.Sprintf("    <name>%s</name>", s.Name),
				fmt.Sprintf("    <description>%s</description>", s.Description),
				"  </skill>",
			)
		}
		lines = append(lines,
			"</available_skills>",
			"",
		)
	}
	lines = append(lines,
		"--- CONTEXT ---",
		"The following context was prepared automatically. Use it before doing blind searches.",
		"Some background information might be summarized as 'available on-demand' to save space.",
		"If you see an on-demand signal, use your tools (read, grep, etc.) to fetch the specific details you need.",
		"",
		fmt.Sprintf("[Workspace]: %s (%s)", info.Workspace, info.Path),
		fmt.Sprintf("[Git Status]: %s", info.GitSummary()),
		fmt.Sprintf("[Background Signals]: %s", info.SignalSummary()),
		"",
		"[Project Semantic Summary]:",
		info.SemanticSummary,
		"",
		"[Relevant Files for your current request]:",
		info.RelevantFilesSummary(),
		"",
		"[Pre-extracted Relevant Snippets]:",
		info.RelevantSnippetsSummary(),
		"",
	)
	if info.BrainSummary != "" {
		lines = append(lines,
			"--- SESSION BRAIN STATE ---",
			info.BrainSummary,
			"",
		)
	}

	// Load AGENTS.md if it exists in the workspace root path
	if info.Path != "" {
		agentsPath := filepath.Join(info.Path, "AGENTS.md")
		if data, err := os.ReadFile(agentsPath); err == nil && len(data) > 0 {
			lines = append(lines,
				"--- AGENTS GUIDELINES (AGENTS.md) ---",
				string(data),
				"",
			)
		}

		designPath := filepath.Join(info.Path, "DESIGN.md")
		if data, err := os.ReadFile(designPath); err == nil && len(data) > 0 {
			lines = append(lines,
				"--- DESIGN SPECIFICATION (DESIGN.md) ---",
				string(data),
				"",
			)
		}
	}

	lines = append(lines,
		"--- AVAILABLE TOOLS ---",
	)
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("- %s: %s | usage: %s", spec.Name, spec.Summary, spec.Usage))
	}
	lines = append(lines,
		"",
		"- task: asynchronous execution for long-running commands (installs, tests, builds). It returns immediately with a task ID; DO NOT use task for quick commands (like git status, git tag, cat) where you need to read the output immediately to make your next step. For those, use the 'bash' tool instead. Usage: 'task <comando>' to start a task, 'task terminate <id>' to kill a running task.",
	)
	return strings.Join(lines, "\n")
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
