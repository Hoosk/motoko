package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/mcp"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/tracelog"

	"github.com/Hoosk/motoko/internal/app/providerman"
	"github.com/Hoosk/motoko/internal/app/scheduleman"
	"github.com/Hoosk/motoko/internal/app/shell"
	"github.com/Hoosk/motoko/internal/app/taskman"
	"github.com/Hoosk/motoko/internal/app/types"
)

const (
	CmdClear       = "clear"
	CmdStatus      = "status"
	CmdList        = "list"
	CmdTool        = "tool"
	CmdTools       = "tools"
	ValNone        = "none"
	ThemeCyberpunk = "cyberpunk"
	DefaultTheme   = ThemeCyberpunk

	CmdQuit    = "quit"
	CmdThemes  = "themes"
	CmdAgent   = "agent"
	CmdShell   = "shell"
	CmdApprove = "approve"
)

type Deps struct {
	ConfigFn     func() *config.AppConfig
	SaveConfigFn func() error
	ThemeFn      func() string
	SetThemeFn   func(name string) error

	InputModeFn    func() types.InputMode
	SetInputModeFn func(types.InputMode)

	ModeFn            func() types.Mode
	SetAgentModeFn    func(string)
	AgentNameFn       func() string
	AgentNamesFn      func() []string
	AgentConfiguredFn func() bool
	DebugFn           func() bool
	SetDebugFn        func(bool)
	AgentFn           func() *agent.Agent
	SystemPromptFn    func(system.ContextInfo) string

	SessionFn      func() *session.Session
	SaveSessionFn  func() error
	BrainFn        func() *brain.Brain
	BrainInitErrFn func() error

	ListTasksFn      func() []*taskman.TaskState
	TerminateTaskFn  func(id string) error
	ListSchedulesFn  func() []scheduleman.Definition
	AddScheduleFn    func(instruction string, interval time.Duration, oneShot bool) (scheduleman.Definition, error)
	RemoveScheduleFn func(id string) error

	ToolSpecsFn        func() []tools.Spec
	RunToolFn          func(ctx context.Context, name, args string) (tools.Result, error)
	MCPServersFn       func() []mcp.ServerStatus
	AddMCPServerFn     func(srv config.MCPServerConfig)
	RemoveMCPServerFn  func(name string) bool

	ProvMgr *providerman.Manager

	PendingFn      func() string
	SetPendingFn   func(cmd string)
	ClearPendingFn func() string

	ContextWindowFn func() int
}

type Dispatcher struct {
	deps     Deps
	registry *Registry
}

func New(deps Deps) *Dispatcher {
	d := &Dispatcher{deps: deps}
	d.registry = d.buildRegistry()
	return d
}

func (d *Dispatcher) Handle(input string, info system.ContextInfo) types.Response {
	parts := strings.Fields(strings.TrimPrefix(input, "/"))
	if len(parts) == 0 {
		return types.Response{}
	}

	command := strings.ToLower(parts[0])
	cmd, ok := d.registry.Lookup(command)
	if !ok {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown command: /%s", command)}}}
	}
	return cmd.Handler(Invocation{RawInput: input, Args: parts[1:], Info: info})
}

func (d *Dispatcher) Definitions() []Definition {
	if d.registry == nil {
		return nil
	}
	return d.registry.Definitions()
}

func (d *Dispatcher) buildRegistry() *Registry {
	r := NewRegistry()
	for _, def := range commandDefinitions {
		r.Add(Command{
			Definition: def,
			Handler:    d.handlerFor(def.Name),
		})
	}
	return r
}

func (d *Dispatcher) handlerFor(command string) Handler {
	switch command {
	case "help":
		return func(inv Invocation) types.Response { return d.helpResponse() }
	case "exit", CmdQuit:
		return func(inv Invocation) types.Response { return types.Response{Signal: CmdQuit} }
	case CmdThemes:
		return d.handleThemesCommand
	case CmdClear:
		return func(inv Invocation) types.Response { return d.handleClearCommand() }
	case "compact":
		return func(inv Invocation) types.Response {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Compacting session..."}}, Action: &types.Action{Type: types.ActionCompact}}
		}
	case string(types.ModePlan), string(types.ModeBuild):
		return d.handleModePresetCommand(command)
	case "learn":
		return func(inv Invocation) types.Response {
			d.deps.SetAgentModeFn("learn")
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Agent switched to: learn"}}, Action: &types.Action{Type: types.ActionAgent, AgentPrompt: learnPrompt()}}
		}
	case "teamwork-preview":
		return func(inv Invocation) types.Response {
			goal := strings.TrimSpace(strings.Join(inv.Args, " "))
			d.deps.SetAgentModeFn("teamwork")
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Agent switched to: teamwork"}}, Action: &types.Action{Type: types.ActionAgent, AgentPrompt: teamworkPreviewPrompt(goal)}}
		}
	case "grill-me":
		return func(inv Invocation) types.Response {
			d.deps.SetAgentModeFn("grill")
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Agent switched to: grill"}}, Action: &types.Action{Type: types.ActionAgent, AgentPrompt: grillMePrompt()}}
		}
	case CmdAgent:
		return d.handleAgentCommand
	case "mode":
		return func(inv Invocation) types.Response { return types.Response{Signal: "open-mode-popup"} }
	case CmdShell, "chat":
		return d.handleInputModeCommand(command)
	case CmdStatus:
		return func(inv Invocation) types.Response {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: d.statusText(inv.Info)}}}
		}
	case "debug":
		return func(inv Invocation) types.Response { return d.handleDebugCommand() }
	case "context":
		return func(inv Invocation) types.Response { return d.handleContextCommand(inv.Info) }
	case "provider":
		return func(inv Invocation) types.Response { return d.deps.ProvMgr.HandleProviderCommand(inv.Args) }
	case "models":
		return func(inv Invocation) types.Response { return d.deps.ProvMgr.HandleModelsCommand(inv.Args) }
	case "sessions":
		return func(inv Invocation) types.Response { return types.Response{Signal: "open-sessions-popup"} }
	case "settings":
		return func(inv Invocation) types.Response { return types.Response{Signal: "open-settings-popup"} }
	case CmdTools:
		return func(inv Invocation) types.Response {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: formatToolList(d.deps.ToolSpecsFn())}}}
		}
	case CmdTool:
		return d.handleToolCommand
	case "mcp":
		return func(inv Invocation) types.Response { return d.handleMCPCommand(inv.Args) }
	case CmdApprove, "deny":
		return d.handleApprovalCommand(command)
	case "trace":
		return func(inv Invocation) types.Response { return d.handleTraceCommand() }
	default:
		return func(inv Invocation) types.Response { return d.dispatchCommand(command, inv) }
	}
}

func (d *Dispatcher) dispatchCommand(command string, inv Invocation) types.Response {
	switch command {
	case "goal":
		return d.handleGoalCommand(inv.Args)
	case "schedule":
		return d.handleScheduleCommand(inv.Args)
	case "task":
		return d.handleTaskCommand(append([]string{command}, inv.Args...))
	case "brain":
		return d.handleBrainCommand(inv.Args)
	case "metrics":
		return d.handleMetricsCommand()
	}

	return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown command: /%s", command)}}}
}

func (d *Dispatcher) handleThemesCommand(inv Invocation) types.Response {
	parts := append([]string{"themes"}, inv.Args...)
	if len(parts) < 2 {
		current := d.deps.ThemeFn()
		if current == "" {
			current = DefaultTheme
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf(
			"Current theme: %s\n"+
				"Available themes:\n"+
				"  cyberpunk    Default dark neon green (default)\n"+
				"  ghost-cyber  Restrained dark cyberpunk with precise accents\n"+
				"  neon-shadow  Dramatic high-contrast magenta and cyan\n"+
				"  black-ice    Cold technical with ice-blue accents\n"+
				"  nord         Arctic blue palette\n"+
				"  dracula      Classic purple and green\n"+
				"  monochrome   Pure green-on-black terminal\n"+
				"Usage: /themes <name>",
			current)}}}
	}
	themeName := strings.ToLower(parts[1])
	switch themeName {
	case ThemeCyberpunk, "ghost-cyber", "neon-shadow", "black-ice", "nord", "dracula", "monochrome":
		if err := d.deps.SetThemeFn(themeName); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		styles.SetTheme(themeName)
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Theme changed to: " + themeName}}}
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown theme: %s. Available: cyberpunk, ghost-cyber, neon-shadow, black-ice, nord, dracula, monochrome", themeName)}}}
	}
}

func (d *Dispatcher) handleClearCommand() types.Response {
	if sess := d.deps.SessionFn(); sess != nil {
		sess.History = nil
		sess.LastInputTokens = 0
		_ = d.deps.SaveSessionFn()
	}
	return types.Response{Clear: true, Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Timeline reset."}}}
}

func (d *Dispatcher) handleModePresetCommand(command string) Handler {
	return func(inv Invocation) types.Response {
		d.deps.SetAgentModeFn(command)
		if command == string(types.ModePlan) {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Mode set to: plan. Shell commands require explicit approval."}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Mode set to: build. Safe commands run directly; sensitive ones require approval."}}}
	}
}

func (d *Dispatcher) handleAgentCommand(inv Invocation) types.Response {
	parts := append([]string{"agent"}, inv.Args...)
	if len(parts) < 2 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Active agent: %s. Available agents: %s", d.deps.AgentNameFn(), strings.Join(d.deps.AgentNamesFn(), ", "))}}}
	}
	agentName := parts[1]
	for _, name := range d.deps.AgentNamesFn() {
		if strings.EqualFold(name, agentName) {
			d.deps.SetAgentModeFn(name)
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Agent switched to: %s", d.deps.AgentNameFn())}}}
		}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown agent: %s", agentName)}}}
}

func (d *Dispatcher) handleInputModeCommand(command string) Handler {
	return func(inv Invocation) types.Response {
		if command == "shell" {
			d.deps.SetInputModeFn(types.InputModeShell)
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Input mode: shell. Any line not starting with / will be executed as a command."}}}
		}
		d.deps.SetInputModeFn(types.InputModeChat)
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Input mode: chat. Normal input will be treated as a prompt."}}}
	}
}

func (d *Dispatcher) handleDebugCommand() types.Response {
	newDebug := !d.deps.DebugFn()
	d.deps.SetDebugFn(newDebug)
	if ag := d.deps.AgentFn(); ag != nil {
		ag.SetDebug(newDebug)
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Agent debug: %t", newDebug)}}}
}

func (d *Dispatcher) handleContextCommand(info system.ContextInfo) types.Response {
	rawPrompt := d.deps.SystemPromptFn(info)
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "--- RAW AGENT SYSTEM PROMPT ---\n\n" + rawPrompt}}}
}

func (d *Dispatcher) handleToolCommand(inv Invocation) types.Response {
	parts := append([]string{"tool"}, inv.Args...)
	if len(parts) < 2 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /tool <name> <args>. Use /tools to list available ones."}}}
	}
	toolName := parts[1]
	toolArgs := ""
	rawTrimmed := strings.TrimPrefix(inv.RawInput, "/")
	idx := 0
	for idx < len(rawTrimmed) && (rawTrimmed[idx] == ' ' || rawTrimmed[idx] == '\t' || rawTrimmed[idx] == '\n' || rawTrimmed[idx] == '\r') {
		idx++
	}
	for idx < len(rawTrimmed) && rawTrimmed[idx] != ' ' && rawTrimmed[idx] != '\t' && rawTrimmed[idx] != '\n' && rawTrimmed[idx] != '\r' {
		idx++
	}
	for idx < len(rawTrimmed) && (rawTrimmed[idx] == ' ' || rawTrimmed[idx] == '\t' || rawTrimmed[idx] == '\n' || rawTrimmed[idx] == '\r') {
		idx++
	}
	for idx < len(rawTrimmed) && rawTrimmed[idx] != ' ' && rawTrimmed[idx] != '\t' && rawTrimmed[idx] != '\n' && rawTrimmed[idx] != '\r' {
		idx++
	}
	for idx < len(rawTrimmed) && (rawTrimmed[idx] == ' ' || rawTrimmed[idx] == '\t' || rawTrimmed[idx] == '\n' || rawTrimmed[idx] == '\r') {
		idx++
	}
	if idx < len(rawTrimmed) {
		toolArgs = rawTrimmed[idx:]
	}
	if strings.EqualFold(toolName, "bash") {
		return d.handleShell(toolArgs)
	}
	runCtx := tools.WithBrain(context.Background(), d.deps.BrainFn())
	runCtx = tools.WithConfig(runCtx, d.deps.ConfigFn())
	runCtx = tools.WithMaxOutputSize(runCtx, system.MaxToolOutputBytes(d.deps.ContextWindowFn()))
	result, err := d.deps.RunToolFn(runCtx, toolName, toolArgs)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
	}
	entries := []types.Entry{{Kind: types.EntryCommand, Text: fmt.Sprintf("tool %s %s", toolName, strings.TrimSpace(toolArgs))}, {Kind: types.EntrySystem, Text: result.Summary}}
	if strings.TrimSpace(result.Output) != "" {
		entries = append(entries, types.Entry{Kind: types.EntryOutput, Text: result.Output})
	}
	return types.Response{Entries: entries}
}

func (d *Dispatcher) handleApprovalCommand(command string) Handler {
	return func(inv Invocation) types.Response {
		pending := d.deps.PendingFn()
		if pending == "" {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No pending action."}}}
		}
		cleared := d.deps.ClearPendingFn()
		if command == "approve" {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryCommand, Text: "$ " + cleared}, {Kind: types.EntrySystem, Text: "Approval received. Executing command..."}}, Action: &types.Action{Type: types.ActionShell, ShellCommand: cleared}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Action cancelled: %s", cleared)}}}
	}
}

func (d *Dispatcher) handleTraceCommand() types.Response {
	if !tracelog.Available() {
		return types.Response{}
	}
	enabled := tracelog.SetEnabled(!tracelog.Enabled())
	if enabled {
		tracelog.Logf("=== TRACE ENABLED ===")
	}
	return types.Response{}
}

func (d *Dispatcher) helpResponse() types.Response {
	defs := d.Definitions()
	maxWidth := 0
	for _, def := range defs {
		if len(def.Usage) > maxWidth {
			maxWidth = len(def.Usage)
		}
	}

	lines := []string{"Available commands:"}
	for _, def := range defs {
		lines = append(lines, fmt.Sprintf("%-*s %s", maxWidth, def.Usage, def.Summary))
	}
	lines = append(lines,
		fmt.Sprintf("%-*s %s", maxWidth, "!<cmd>", "Execute an explicit shell command"),
		fmt.Sprintf("%-*s %s", maxWidth, "@<file|agent>", "Mention a file or agent in the prompt"),
	)

	return types.Response{Entries: []types.Entry{{Kind: types.EntryHelp, Text: strings.Join(lines, "\n")}}}
}

func percentage(value, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

func growthPercentage(turn *session.TurnUsage) float64 {
	if turn == nil || len(turn.Iterations) == 0 {
		return 0
	}
	return percentage(turn.InputGrowth, turn.Iterations[0].InputTokens)
}

func (d *Dispatcher) handleTaskCommand(parts []string) types.Response {
	if len(parts) < 2 || strings.EqualFold(parts[1], CmdList) {
		return d.formatTaskList()
	}
	subcmd := strings.ToLower(parts[1])
	switch subcmd {
	case "terminate":
		if len(parts) < 3 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /task terminate <idTask>"}}}
		}
		id := parts[2]
		if err := d.deps.TerminateTaskFn(id); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Task %s terminated.", id)}}}
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown subcommand: %s. Usage: /task or /task terminate <idTask>", subcmd)}}}
	}
}

func (d *Dispatcher) formatTaskList() types.Response {
	tasks := d.deps.ListTasksFn()
	if len(tasks) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No active background tasks."}}}
	}
	var sb strings.Builder
	sb.WriteString("Active tasks:\n")
	for _, t := range tasks {
		fmt.Fprintf(&sb, "- %s: %q (started %s ago)\n", t.ID, t.Command, time.Since(t.Started).Round(time.Second))
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.TrimSpace(sb.String())}}}
}

func writeTokenBreakdown(sb *strings.Builder, label string, input, static, dynamic, toolDefs, history, cacheRead, output, reasoning, cacheWrite int) {
	fmt.Fprintf(sb, "\n%s:\n", label)
	fmt.Fprintf(sb, "- Input Tokens: %d\n", input)
	if input > 0 {
		fmt.Fprintf(sb, "  * System Prompt (Static):  %d (%.1f%% of input)\n", static, percentage(static, input))
		fmt.Fprintf(sb, "  * System Prompt (Dynamic): %d (%.1f%% of input)\n", dynamic, percentage(dynamic, input))
		fmt.Fprintf(sb, "  * Tool Definitions:       %d (%.1f%% of input)\n", toolDefs, percentage(toolDefs, input))
		fmt.Fprintf(sb, "  * History & Query:        %d (%.1f%% of input)\n", history, percentage(history, input))
	}
	if input > 0 && cacheRead > 0 {
		fmt.Fprintf(sb, "  * Cache Read:  %d (%.1f%% of input)\n", cacheRead, percentage(cacheRead, input))
	}
	if cacheWrite > 0 {
		fmt.Fprintf(sb, "  * Cache Write: %d\n", cacheWrite)
	}
	fmt.Fprintf(sb, "- Output Tokens: %d\n", output)
	if output > 0 && reasoning > 0 {
		fmt.Fprintf(sb, "  * Reasoning (Thinking) Tokens: %d (%.1f%% of output)\n", reasoning, percentage(reasoning, output))
	}
}

func (d *Dispatcher) handleMetricsCommand() types.Response {
	sess := d.deps.SessionFn()
	if sess == nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No active session."}}}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Current Session Metrics (%s):\n", sess.ID)
	fmt.Fprintf(&sb, "- Created at: %s\n", sess.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "- History Messages: %d\n", len(sess.History))

	writeTokenBreakdown(&sb, "Last Turn Token Usage", sess.LastInputTokens, sess.LastSystemStaticTokens, sess.LastSystemDynamicTokens, sess.LastToolsTokens, sess.LastHistoryTokens, sess.LastCacheReadTokens, sess.LastOutputTokens, sess.LastReasoningTokens, sess.LastCacheWriteTokens)
	writeTokenBreakdown(&sb, "Cumulative Token Usage", sess.TotalInputTokens, sess.TotalSystemStaticTokens, sess.TotalSystemDynamicTokens, sess.TotalToolsTokens, sess.TotalHistoryTokens, sess.TotalCacheReadTokens, sess.TotalOutputTokens, sess.TotalReasoningTokens, sess.TotalCacheWriteTokens)
	fmt.Fprintf(&sb, "- Total Tokens:  %d\n", sess.TotalTokens)

	if len(sess.Turns) > 0 {
		sb.WriteString("\nRecent Turn Trend:\n")
		for _, turn := range sess.Turns {
			fmt.Fprintf(&sb, "- Turn %d", turn.Turn)
			if turn.AgentLabel != "" {
				fmt.Fprintf(&sb, " [%s]", turn.AgentLabel)
			}
			fmt.Fprintf(&sb, ": in=%d out=%d reasoning=%d total=%d", turn.InputTokens, turn.OutputTokens, turn.ReasoningTokens, turn.TotalTokens)
			if turn.InputGrowth != 0 {
				growthPct := growthPercentage(&turn)
				fmt.Fprintf(&sb, " input_growth=%+d (%.1f%%)", turn.InputGrowth, growthPct)
				if turn.InputGrowth > 0 && growthPct >= 15.0 {
					sb.WriteString(" BLOAT")
				}
			}
			if turn.CacheReadTokens > 0 || turn.CacheWriteTokens > 0 {
				fmt.Fprintf(&sb, " cache=%d/%d", turn.CacheReadTokens, turn.CacheWriteTokens)
			}
			sb.WriteByte('\n')
			for idx, iter := range turn.Iterations {
				fmt.Fprintf(&sb, "  iter %d: in=%d out=%d reasoning=%d total=%d", idx+1, iter.InputTokens, iter.OutputTokens, iter.ReasoningTokens, iter.TotalTokens)
				if idx > 0 {
					delta := iter.InputTokens - turn.Iterations[idx-1].InputTokens
					fmt.Fprintf(&sb, " input_delta=%+d", delta)
				}
				if iter.CacheReadInputTokens > 0 || iter.CacheWriteInputTokens > 0 {
					fmt.Fprintf(&sb, " cache=%d/%d", iter.CacheReadInputTokens, iter.CacheWriteInputTokens)
				}
				sb.WriteByte('\n')
			}
		}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: sb.String()}}}
}

func (d *Dispatcher) statusText(info system.ContextInfo) string {
	pending := ValNone
	if p := d.deps.PendingFn(); p != "" {
		pending = p
	}

	agentsStatus := "not found"
	if info.Path != "" {
		if _, err := os.Stat(filepath.Join(info.Path, "AGENTS.md")); err == nil {
			agentsStatus = "loaded"
		}
	}

	designStatus := "not found"
	if info.Path != "" {
		if _, err := os.Stat(filepath.Join(info.Path, "DESIGN.md")); err == nil {
			designStatus = "loaded"
		}
	}

	return strings.Join([]string{
		fmt.Sprintf("mode: %s", d.deps.ModeFn()),
		fmt.Sprintf("input: %s", d.deps.InputModeFn()),
		fmt.Sprintf("agent configured: %t", d.deps.AgentConfiguredFn()),
		fmt.Sprintf("active provider: %s", d.deps.ProvMgr.ProviderSummary()),
		fmt.Sprintf("workspace: %s", info.Workspace),
		fmt.Sprintf("git: %s", info.GitSummary()),
		fmt.Sprintf("agents.md guidelines: %s", agentsStatus),
		fmt.Sprintf("design.md specification: %s", designStatus),
		fmt.Sprintf("pending approval: %s", pending),
		"policy: plan asks for shell approval; build runs safe commands and asks for sensitive ones.",
	}, "\n")
}

func (d *Dispatcher) handleShell(command string) types.Response {
	if command == "" {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Missing command after !"}}}
	}

	decision := shell.Classify(d.deps.ModeFn(), command)
	if decision.Deny {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: decision.Reason}}}
	}

	if decision.RequiresApproval {
		d.deps.SetPendingFn(command)
		return types.Response{Entries: []types.Entry{
			{Kind: types.EntryCommand, Text: "$ " + command},
			{Kind: types.EntrySystem, Text: fmt.Sprintf("Pending action: %s Use /approve or /deny.", decision.Reason)},
		}}
	}

	return types.Response{
		Entries: []types.Entry{
			{Kind: types.EntryCommand, Text: "$ " + command},
			{Kind: types.EntrySystem, Text: "Executing command..."},
		},
		Action: &types.Action{Type: types.ActionShell, ShellCommand: command},
	}
}

func (d *Dispatcher) handleBrainCommand(parts []string) types.Response {
	br := d.deps.BrainFn()
	if br == nil {
		if err := d.deps.BrainInitErrFn(); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Session brain not initialized: %v", err)}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Session brain not initialized."}}}
	}

	if len(parts) == 0 {
		return d.listBrainFiles()
	}

	subcmd := strings.ToLower(parts[0])
	switch subcmd {
	case CmdList:
		return d.listBrainFiles()
	case "read":
		if len(parts) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /brain read <filename>"}}}
		}
		filename := parts[1]
		content, err := br.Read(filename)
		if err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Failed to read brain file: %v", err)}}}
		}
		return types.Response{Entries: []types.Entry{
			{Kind: types.EntrySystem, Text: fmt.Sprintf("--- Brain File: %s ---", filename)},
			{Kind: types.EntrySystem, Text: content},
		}}
	case "plan":
		content, err := br.Read("plan.md")
		if err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "No plan.md found in session brain."}}}
		}
		return types.Response{Entries: []types.Entry{
			{Kind: types.EntrySystem, Text: "--- Session Plan (plan.md) ---"},
			{Kind: types.EntrySystem, Text: content},
		}}
	case "tasks":
		content, err := br.Read("tasks.md")
		if err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "No tasks.md found in session brain."}}}
		}
		return types.Response{Entries: []types.Entry{
			{Kind: types.EntrySystem, Text: "--- Session Tasks (tasks.md) ---"},
			{Kind: types.EntrySystem, Text: content},
		}}
	case "summary":
		content, err := br.Read("summary.md")
		if err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "No summary.md found in session brain."}}}
		}
		return types.Response{Entries: []types.Entry{
			{Kind: types.EntrySystem, Text: "--- Session Summary (summary.md) ---"},
			{Kind: types.EntrySystem, Text: content},
		}}
	case CmdClear:
		files, err := br.List()
		if err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Failed to list brain files: %v", err)}}}
		}
		var deleteErrors []string
		for _, f := range files {
			if err := br.Delete(f.Name); err != nil {
				deleteErrors = append(deleteErrors, fmt.Sprintf("%s: %v", f.Name, err))
			}
		}
		if len(deleteErrors) > 0 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Failed to delete some brain files: %s", strings.Join(deleteErrors, "; "))}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "All session brain files deleted."}}}
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown subcommand: %s. Available subcommands: list, read, plan, tasks, summary, clear.", subcmd)}}}
	}
}

func (d *Dispatcher) listBrainFiles() types.Response {
	files, err := d.deps.BrainFn().List()
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Failed to list brain files: %v", err)}}}
	}
	if len(files) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No brain files in the current session."}}}
	}
	var sb strings.Builder
	sb.WriteString("Session brain files:\n")
	for _, f := range files {
		ago := time.Since(f.ModTime).Truncate(time.Second)
		fmt.Fprintf(&sb, "- %s (%d bytes, updated %s ago)\n", f.Name, f.SizeBytes, ago)
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.TrimSpace(sb.String())}}}
}

func formatToolList(specs []tools.Spec) string {
	lines := []string{"Registered tools:"}
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("- %s: %s", spec.Usage, spec.Summary))
	}
	return strings.Join(lines, "\n")
}

func (d *Dispatcher) handleGoalCommand(args []string) types.Response {
	br := d.deps.BrainFn()
	if br == nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Session brain not initialized."}}}
	}
	if len(args) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /goal [plan|clear|status|<description>]"}}}
	}
	joined := strings.TrimSpace(strings.Join(args, " "))
	switch strings.ToLower(joined) {
	case "clear":
		_ = br.Delete("goal")
		_ = br.Delete("goal_state")
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Goal cleared."}}}
	case "status":
		content, err := br.Read("goal")
		if err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No active goal."}}}
		}
		pending, completed := countTaskCheckboxes(br)
		status := fmt.Sprintf("Active goal:\n%s\n\nTasks: %d pending, %d completed", content, pending, completed)
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: status}}}
	case "plan":
		if _, err := br.Read("tasks"); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "No tasks.md found in the session brain. Run /plan first."}}}
		}
		if err := br.Write("goal", "# Goal\nFinish every unchecked task in tasks.md until the plan is complete."); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Goal activated from tasks.md. Motoko will auto-continue until all tasks are done or /goal clear is used."}}}
	default:
		content := "# Goal\n" + joined + "\n\nBreak this into tasks.md if needed and keep going until it is complete."
		if err := br.Write("goal", content); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		return types.Response{
			Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Goal stored. Motoko will keep auto-continuing until completion or /goal clear."}},
			Action:  &types.Action{Type: types.ActionAgent, AgentPrompt: goalKickoffPrompt(joined)},
		}
	}
}

func (d *Dispatcher) handleScheduleCommand(args []string) types.Response {
	if len(args) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /schedule [list|add <instruction> every|once <duration>|remove <id>]"}}}
	}
	subcmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch subcmd {
	case CmdList:
		schedules := d.deps.ListSchedulesFn()
		if len(schedules) == 0 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No active schedules."}}}
		}
		var sb strings.Builder
		sb.WriteString("Active schedules:\n")
		for _, sched := range schedules {
			kind := "every"
			if sched.OneShot {
				kind = "once"
			}
			fmt.Fprintf(&sb, "- %s: %q (%s %s)\n", sched.ID, sched.Instruction, kind, sched.Interval)
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.TrimSpace(sb.String())}}}
	case "remove":
		if len(args) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /schedule remove <id>"}}}
		}
		if err := d.deps.RemoveScheduleFn(strings.TrimSpace(args[1])); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Schedule %s removed.", strings.TrimSpace(args[1]))}}}
	case "add":
		instruction, every, duration, err := parseScheduleArgs(args[1:])
		if err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		def, addErr := d.deps.AddScheduleFn(instruction, duration, !every)
		if addErr != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: addErr.Error()}}}
		}
		kind := "every"
		if def.OneShot {
			kind = "once"
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Schedule %s created for %q (%s %s).", def.ID, def.Instruction, kind, def.Interval)}}}
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Unknown schedule subcommand. Use list, add, or remove."}}}
	}
}

func learnPrompt() string {
	return "Capture reusable project knowledge from the current conversation. Ask follow-up questions first if the scope or output format is unclear."
}

func teamworkPreviewPrompt(goal string) string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		goal = "Use the current plan.md/tasks.md in the brain as the project goal."
	}
	return fmt.Sprintf("Project goal for this teamwork preview: %s", goal)
}

func grillMePrompt() string {
	return "Interview me about the current plan until the important ambiguities are resolved."
}

func goalKickoffPrompt(goal string) string {
	return strings.TrimSpace(fmt.Sprintf(`A persistent goal has been set for this session:

%s

Requirements:
- If tasks.md does not exist or is too vague, create or refine it first.
- Continue executing the next unfinished task.
- Keep tasks.md updated with [x] as tasks complete.
- The system will keep prompting you to continue until all tasks are done or the user clears the goal.`, goal))
}

func countTaskCheckboxes(br *brain.Brain) (pending int, completed int) {
	if br == nil {
		return 0, 0
	}
	return br.TaskCounts()
}

func parseScheduleArgs(args []string) (instruction string, every bool, duration time.Duration, err error) {
	if len(args) < 3 {
		return "", false, 0, fmt.Errorf("usage: /schedule add <instruction> every|once <duration>")
	}
	marker := -1
	for i, arg := range args {
		if strings.EqualFold(arg, "every") || strings.EqualFold(arg, "once") {
			marker = i
			break
		}
	}
	if marker <= 0 || marker >= len(args)-1 {
		return "", false, 0, fmt.Errorf("usage: /schedule add <instruction> every|once <duration>")
	}
	instruction = strings.Trim(strings.Join(args[:marker], " "), `"`)
	every = strings.EqualFold(args[marker], "every")
	duration, err = time.ParseDuration(strings.TrimSpace(args[marker+1]))
	if err != nil {
		return "", false, 0, fmt.Errorf("invalid duration: %v", err)
	}
	if strings.TrimSpace(instruction) == "" {
		return "", false, 0, fmt.Errorf("instruction cannot be empty")
	}
	return instruction, every, duration, nil
}

func (d *Dispatcher) handleMCPCommand(args []string) types.Response {
	var servers []mcp.ServerStatus
	if d.deps.MCPServersFn != nil {
		servers = d.deps.MCPServersFn()
	}

	if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "servers") || strings.EqualFold(args[0], "status") {
		if len(servers) == 0 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No MCP servers configured or running.\nAdd entries to .agents/mcp.json or application config."}}}
		}
		lines := []string{"MCP Servers:"}
		for _, s := range servers {
			statusStr := "Connected"
			if !s.Connected {
				statusStr = "Disconnected"
			}
			if s.Err != nil {
				statusStr = fmt.Sprintf("Error (%v)", s.Err)
			}
			lines = append(lines, fmt.Sprintf("• %s [%s] - %s (%d tools)", s.Name, s.Transport, statusStr, s.ToolCount))
			if len(s.Tools) > 0 {
				for _, t := range s.Tools {
					lines = append(lines, fmt.Sprintf("  └ %s", t))
				}
			}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "add":
		if len(args) < 3 {
			usage := "Usage:\n  /mcp add <name> <command> [args...]\n  /mcp add <name> http <url>\nExamples:\n  /mcp add git npx -y @modelcontextprotocol/server-git .\n  /mcp add remote http https://mcp.example.com/sse"
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: usage}}}
		}
		name := args[1]
		var srv config.MCPServerConfig
		if strings.EqualFold(args[2], "http") || strings.EqualFold(args[2], "https") {
			if len(args) < 4 && !strings.Contains(args[2], "://") {
				return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /mcp add <name> http <url>"}}}
			}
			urlStr := args[2]
			if len(args) >= 4 {
				urlStr = args[3]
			}
			srv = config.MCPServerConfig{
				Name:      name,
				Transport: "http",
				URL:       urlStr,
			}
		} else if strings.HasPrefix(strings.ToLower(args[2]), "http://") || strings.HasPrefix(strings.ToLower(args[2]), "https://") {
			srv = config.MCPServerConfig{
				Name:      name,
				Transport: "http",
				URL:       args[2],
			}
		} else {
			srv = config.MCPServerConfig{
				Name:      name,
				Transport: "stdio",
				Command:   args[2],
				Args:      args[3:],
			}
		}

		cfg := d.deps.ConfigFn()
		if cfg != nil {
			cfg.UpsertMCPServer(srv)
			_ = d.deps.SaveConfigFn()
		}
		if d.deps.AddMCPServerFn != nil {
			d.deps.AddMCPServerFn(srv)
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("MCP server added: %s", name)}}}

	case "remove":
		if len(args) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /mcp remove <name>"}}}
		}
		name := args[1]
		removed := false
		cfg := d.deps.ConfigFn()
		if cfg != nil {
			removed = cfg.RemoveMCPServer(name)
			if removed {
				_ = d.deps.SaveConfigFn()
			}
		}
		if d.deps.RemoveMCPServerFn != nil {
			if d.deps.RemoveMCPServerFn(name) {
				removed = true
			}
		}
		if !removed {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("MCP server not found: %s", name)}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("MCP server removed: %s", name)}}}

	case "tools":
		if d.deps.ToolSpecsFn == nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No MCP tools registered."}}}
		}
		specs := d.deps.ToolSpecsFn()
		var lines []string
		lines = append(lines, "Registered MCP Tools:")
		count := 0
		for _, spec := range specs {
			if strings.HasPrefix(strings.ToLower(spec.Name), "mcp_") {
				count++
				lines = append(lines, fmt.Sprintf("• %s: %s", spec.Name, spec.Summary))
			}
		}
		if count == 0 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No MCP tools registered."}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}

	case "info":
		if len(args) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /mcp info <server>"}}}
		}
		name := strings.ToLower(args[1])
		for _, s := range servers {
			if strings.ToLower(s.Name) == name {
				statusStr := "Connected"
				if !s.Connected {
					statusStr = "Disconnected"
				}
				if s.Err != nil {
					statusStr = fmt.Sprintf("Error (%v)", s.Err)
				}
				lines := []string{
					fmt.Sprintf("Server: %s", s.Name),
					fmt.Sprintf("Transport: %s", s.Transport),
					fmt.Sprintf("Status: %s", statusStr),
					fmt.Sprintf("Tools (%d): %s", s.ToolCount, strings.Join(s.Tools, ", ")),
				}
				return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
			}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("MCP server not found: %s", args[1])}}}

	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown subcommand: %s\nUsage: /mcp [list|add|remove|tools|info <server>]", sub)}}}
	}
}
