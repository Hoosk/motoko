package agent

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

const defaultMaxToolIterations = 24

type Result struct {
	Assistant  string
	Steps      []Step
	Usage      provider.Usage
	AgentLabel string
	Duration   time.Duration
	Context    ContextSnapshot
	Messages   []provider.Message
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
	debug       bool
	agentSystem string
}

type StreamEvent struct {
	Kind    string
	Title   string
	Content string
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

func (a *Agent) Run(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.Message) (Result, error) {
	return a.run(ctx, info, userInput, priorHistory, nil)
}

func (a *Agent) RunStream(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.Message, onEvent func(StreamEvent) error) (Result, error) {
	return a.run(ctx, info, userInput, priorHistory, onEvent)
}

func (a *Agent) run(ctx context.Context, info system.ContextInfo, userInput string, priorHistory []provider.Message, onEvent func(StreamEvent) error) (Result, error) {
	if !a.Configured() {
		return Result{}, fmt.Errorf("agente no configurado")
	}
	startedAt := time.Now()

	messages := append([]provider.Message{}, priorHistory...)
	messages = append(messages, provider.Message{Role: "user", Content: userInput})
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
		resp, err := a.complete(ctx, info, messages, onEvent)
		if err != nil {
			return Result{}, err
		}
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens
		if a.debug {
			steps = append(steps, Step{Kind: "debug", Title: "provider", Content: fmt.Sprintf("completion %d tokens in:%d out:%d total:%d", i+1, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)})
		}

		if resp.ToolCall == nil {
			message := strings.TrimSpace(resp.Message)
			if message == "" {
				message = "No tengo una respuesta util todavia."
			}
			messages = append(messages, provider.Message{Role: "assistant", Content: message})
			steps = append(steps, Step{Kind: "assistant", Title: "answer", Content: message})
			return Result{Assistant: message, Steps: steps, Usage: totalUsage, AgentLabel: a.provider.Summary(), Duration: time.Since(startedAt), Context: contextSnapshot, Messages: messages}, nil
		}

		toolName := strings.TrimSpace(resp.ToolCall.Name)
		toolInput := strings.TrimSpace(resp.ToolCall.Input)
		toolKey := toolName + "\x00" + toolInput
		if _, seen := seenToolCalls[toolKey]; seen {
			return Result{}, fmt.Errorf("ciclo de tool detectado: %s %s", toolName, toolInput)
		}
		seenToolCalls[toolKey] = struct{}{}
		steps = append(steps, Step{Kind: "tool", Title: toolName, Content: toolInput})
		if onEvent != nil {
			_ = onEvent(StreamEvent{Kind: "tool", Title: toolName, Content: toolInput})
		}

		result, err := a.tools.Run(ctx, toolName, toolInput)
		if err != nil {
			errText := fmt.Sprintf("tool error: %v", err)
			steps = append(steps, Step{Kind: "error", Title: toolName, Content: errText})
			if onEvent != nil {
				_ = onEvent(StreamEvent{Kind: "error", Title: toolName, Content: errText})
			}
			messages = append(messages,
				provider.Message{Role: "assistant", Content: fmt.Sprintf("Voy a usar la tool %s.", toolName)},
				provider.Message{Role: "user", Content: errText},
			)
			continue
		}

		toolOutput := strings.TrimSpace(strings.Join([]string{result.Summary, result.Output}, "\n\n"))
		steps = append(steps, Step{Kind: "output", Title: toolName, Content: toolOutput})
		if onEvent != nil {
			_ = onEvent(StreamEvent{Kind: "output", Title: toolName, Content: toolOutput})
		}
		messages = append(messages,
			provider.Message{Role: "assistant", Content: fmt.Sprintf("Voy a usar la tool %s con esta entrada:\n%s", toolName, toolInput)},
			provider.Message{Role: "user", Content: fmt.Sprintf("Resultado de %s:\n%s", toolName, toolOutput)},
		)
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

func (a *Agent) complete(ctx context.Context, info system.ContextInfo, messages []provider.Message, onEvent func(StreamEvent) error) (provider.Response, error) {
	if onEvent == nil {
		return a.provider.Complete(ctx, buildSystemPrompt(info, a.tools.Specs(), a.agentSystem), messages, toolDefinitions(a.tools.Specs()))
	}
	extractor := structuredStreamExtractor{}
	return a.provider.StreamComplete(ctx, buildSystemPrompt(info, a.tools.Specs(), a.agentSystem), messages, toolDefinitions(a.tools.Specs()), func(delta string) error {
		visibleDelta := extractor.Feed(delta)
		if visibleDelta == "" {
			return nil
		}
		return onEvent(StreamEvent{Kind: "assistant_delta", Content: visibleDelta})
	})
}

func buildSystemPrompt(info system.ContextInfo, specs []tools.Spec, agentSystem string) string {
	var lines []string
	lines = append(lines,
		"You are Motoko, a senior coding agent working directly in the user's terminal and repository.",
		"Return exactly one JSON object and nothing else.",
		"Do not wrap JSON in markdown fences.",
		"Do not add prose before or after the JSON.",
		"Valid response forms:",
		`1. Final answer: {"message":"plain text for the user"}`,
		`2. Tool call: {"tool_name":"tool_name","tool_input":"arguments"}`,
		"When answering the user, keep message text direct, factual, and ready to display incrementally while you stream.",
		"",
		"--- OPERATING RULES ---",
		"- Use tools to explore the codebase before assuming how it works.",
		"- If you use a tool, request only one tool at a time. The system will return the result to you.",
		"- DO NOT invent file names, functions, or command outputs.",
		"- Prefer finishing the task end-to-end instead of stopping at analysis.",
		"- If the existing context already answers the question, answer directly without unnecessary tool calls.",
		"",
	)
	if agentSystem != "" {
		lines = append(lines, "--- AGENT MODE ---", agentSystem, "")
	}
	lines = append(lines,
		"--- CONTEXT ---",
		"The following context was prepared automatically. Use it before doing blind searches.",
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
		"--- AVAILABLE TOOLS ---",
	)
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("- %s: %s | usage: %s", spec.Name, spec.Summary, spec.Usage))
	}
	return strings.Join(lines, "\n")
}

func toolDefinitions(specs []tools.Spec) []provider.ToolDefinition {
	result := make([]provider.ToolDefinition, 0, len(specs))
	for _, spec := range specs {
		result = append(result, provider.ToolDefinition{
			Name:        spec.Name,
			Description: spec.Summary,
			InputHint:   spec.Usage,
		})
	}
	return result
}

type structuredStreamExtractor struct {
	raw     strings.Builder
	emitted string
	plain   bool
}

func (e *structuredStreamExtractor) Feed(chunk string) string {
	if chunk == "" {
		return ""
	}
	if e.plain {
		return chunk
	}
	e.raw.WriteString(chunk)
	raw := providerNormalizeStructuredPayload(e.raw.String())
	decoded := extractStructuredMessagePrefix(raw)
	if decoded == "" {
		trimmed := strings.TrimLeft(raw, " \n\r\t")
		if trimmed != "" && trimmed[0] != '{' {
			e.plain = true
			return raw
		}
		return ""
	}
	if !strings.HasPrefix(decoded, e.emitted) {
		e.emitted = decoded
		return ""
	}
	delta := decoded[len(e.emitted):]
	e.emitted = decoded
	return delta
}

func extractStructuredMessagePrefix(raw string) string {
	keyIndex := strings.Index(raw, `"message"`)
	if keyIndex == -1 {
		return ""
	}
	i := keyIndex + len(`"message"`)
	for i < len(raw) && isJSONWhitespace(raw[i]) {
		i++
	}
	if i >= len(raw) || raw[i] != ':' {
		return ""
	}
	i++
	for i < len(raw) && isJSONWhitespace(raw[i]) {
		i++
	}
	if i >= len(raw) || raw[i] != '"' {
		return ""
	}
	i++
	var out strings.Builder
	for i < len(raw) {
		switch raw[i] {
		case '"':
			return out.String()
		case '\\':
			if i+1 >= len(raw) {
				return out.String()
			}
			switch raw[i+1] {
			case '"', '\\', '/':
				out.WriteByte(raw[i+1])
				i += 2
			case 'b':
				out.WriteByte('\b')
				i += 2
			case 'f':
				out.WriteByte('\f')
				i += 2
			case 'n':
				out.WriteByte('\n')
				i += 2
			case 'r':
				out.WriteByte('\r')
				i += 2
			case 't':
				out.WriteByte('\t')
				i += 2
			case 'u':
				if i+6 > len(raw) {
					return out.String()
				}
				value, err := strconv.ParseInt(raw[i+2:i+6], 16, 32)
				if err != nil {
					return out.String()
				}
				out.WriteRune(rune(value))
				i += 6
			default:
				return out.String()
			}
		default:
			out.WriteByte(raw[i])
			i++
		}
	}
	return out.String()
}

func isJSONWhitespace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}

func providerNormalizeStructuredPayload(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
		if strings.HasPrefix(trimmed, "json") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "json"))
		}
	}
	return trimmed
}
