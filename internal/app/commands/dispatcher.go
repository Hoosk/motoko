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
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/tracelog"

	"github.com/Hoosk/motoko/internal/app/providerman"
	"github.com/Hoosk/motoko/internal/app/shell"
	"github.com/Hoosk/motoko/internal/app/taskman"
	"github.com/Hoosk/motoko/internal/app/types"
)

const (
	CmdClear       = "clear"
	CmdStatus      = "status"
	CmdList        = "list"
	CmdTool        = "tool"
	ValNone        = "none"
	ThemeCyberpunk = "cyberpunk"
	DefaultTheme   = ThemeCyberpunk
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

	ListTasksFn     func() []*taskman.TaskState
	TerminateTaskFn func(id string) error

	ToolSpecsFn func() []tools.Spec
	RunToolFn   func(ctx context.Context, name, args string) (tools.Result, error)

	ProvMgr *providerman.Manager

	PendingFn      func() string
	SetPendingFn   func(cmd string)
	ClearPendingFn func() string

	ContextWindowFn func() int
}

type Dispatcher struct {
	deps Deps
}

func New(deps Deps) *Dispatcher {
	return &Dispatcher{deps: deps}
}

func (d *Dispatcher) Handle(input string, info system.ContextInfo) types.Response {
	parts := strings.Fields(strings.TrimPrefix(input, "/"))
	if len(parts) == 0 {
		return types.Response{}
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "help":
		return types.Response{Entries: []types.Entry{{Kind: types.EntryHelp, Text: strings.Join([]string{
			"Available commands:",
			"/help                   Show this help message",
			"/clear                  Clear the timeline history",
			"/compact                Manually compact the active session",
			"/mode                   Open the agent mode selector",
			"/plan                   Activate read-only plan mode",
			"/build                  Activate active build mode",
			"/agent <name>           Switch or show active agent mode",
			"/shell                  Activate direct shell execution mode",
			"/chat                   Return to normal chat mode",
			"/status                 Summarize mode, permissions, and approvals",
			"/context                Show raw system prompt sent to the agent",
			"/provider               Manage configured LLM providers",
			"/models [model]         List or select models from the active provider",
			"/themes [theme]         List or switch visual themes (cyberpunk, ghost-cyber, neon-shadow, black-ice, nord, dracula, monochrome)",
			"/sessions               List or switch between workspace sessions",
			"/tools                  Show all registered tools",
			"/tool <name> [args]     Execute a specific runtime tool",
			"/task                   Interact with background tasks",
			"/approve                Execute the pending tool command",
			"/deny                   Cancel the pending tool command",
			"/brain                  Interact with the session brain (list, read, plan, tasks, summary, clear)",
			"/metrics                Show cumulative token usage for this session",
			"/trace                  Toggle trace logging (requires -tags motoko_trace)",
			"/exit                   Exit the application",
			"/quit                   Exit the application",
			"!<cmd>                  Execute an explicit shell command",
			"@<file|agent>           Mention a file or agent in the prompt",
		}, "\n")}}}
	case "exit", "quit":
		return types.Response{Signal: "quit"}
	case "themes":
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
	case CmdClear:
		if sess := d.deps.SessionFn(); sess != nil {
			sess.History = nil
			sess.LastInputTokens = 0
			_ = d.deps.SaveSessionFn()
		}
		return types.Response{Clear: true, Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Timeline reset."}}}
	case "compact":
		return types.Response{
			Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Compacting session..."}},
			Action:  &types.Action{Type: types.ActionCompact},
		}
	case string(types.ModePlan):
		d.deps.SetAgentModeFn(string(types.ModePlan))
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Mode set to: plan. Shell commands require explicit approval."}}}
	case "build":
		d.deps.SetAgentModeFn("build")
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Mode set to: build. Safe commands run directly; sensitive ones require approval."}}}
	case "agent":
		if len(parts) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Active agent: %s. Available agents: %s", d.deps.AgentNameFn(), strings.Join(d.deps.AgentNamesFn(), ", "))}}}
		}
		agentName := parts[1]
		found := false
		for _, name := range d.deps.AgentNamesFn() {
			if strings.EqualFold(name, agentName) {
				d.deps.SetAgentModeFn(name)
				found = true
				break
			}
		}
		if !found {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown agent: %s", agentName)}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Agent switched to: %s", d.deps.AgentNameFn())}}}
	case "mode":
		return types.Response{Signal: "open-mode-popup"}
	case "shell":
		d.deps.SetInputModeFn(types.InputModeShell)
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Input mode: shell. Any line not starting with / will be executed as a command."}}}
	case "chat":
		d.deps.SetInputModeFn(types.InputModeChat)
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Input mode: chat. Normal input will be treated as a prompt."}}}
	case CmdStatus:
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: d.statusText(info)}}}
	case "debug":
		newDebug := !d.deps.DebugFn()
		d.deps.SetDebugFn(newDebug)
		if ag := d.deps.AgentFn(); ag != nil {
			ag.SetDebug(newDebug)
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Agent debug: %t", newDebug)}}}
	case "context":
		rawPrompt := d.deps.SystemPromptFn(info)
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "--- RAW AGENT SYSTEM PROMPT ---\n\n" + rawPrompt}}}
	case "provider":
		return d.deps.ProvMgr.HandleProviderCommand(parts[1:])
	case "models":
		return d.deps.ProvMgr.HandleModelsCommand(parts[1:])
	case "sessions":
		return types.Response{Signal: "open-sessions-popup"}
	case "tools":
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: formatToolList(d.deps.ToolSpecsFn())}}}
	case CmdTool:
		if len(parts) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /tool <name> <args>. Use /tools to list available ones."}}}
		}

		toolName := parts[1]
		toolArgs := ""
		rawTrimmed := strings.TrimPrefix(input, "/")
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
		runCtx = tools.WithMaxOutputSize(runCtx, system.MaxToolOutputBytes(d.deps.ContextWindowFn()))
		result, err := d.deps.RunToolFn(runCtx, toolName, toolArgs)
		if err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}

		entries := []types.Entry{
			{Kind: types.EntryCommand, Text: fmt.Sprintf("tool %s %s", toolName, strings.TrimSpace(toolArgs))},
			{Kind: types.EntrySystem, Text: result.Summary},
		}
		if strings.TrimSpace(result.Output) != "" {
			entries = append(entries, types.Entry{Kind: types.EntryOutput, Text: result.Output})
		}
		return types.Response{Entries: entries}
	case "approve":
		pending := d.deps.PendingFn()
		if pending == "" {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No pending action."}}}
		}

		cleared := d.deps.ClearPendingFn()
		return types.Response{
			Entries: []types.Entry{
				{Kind: types.EntryCommand, Text: "$ " + cleared},
				{Kind: types.EntrySystem, Text: "Approval received. Executing command..."},
			},
			Action: &types.Action{Type: types.ActionShell, ShellCommand: cleared},
		}
	case "deny":
		pending := d.deps.PendingFn()
		if pending == "" {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No pending action."}}}
		}

		cleared := d.deps.ClearPendingFn()
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Action cancelled: %s", cleared)}}}
	case "trace":
		if !tracelog.Available() {
			return types.Response{}
		}
		enabled := tracelog.SetEnabled(!tracelog.Enabled())
		if enabled {
			tracelog.Logf("=== TRACE ENABLED ===")
		}
		return types.Response{}
	case "task":
		if len(parts) < 2 {
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

		subcmd := strings.ToLower(parts[1])
		switch subcmd {
		case CmdList:
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
	case "brain":
		return d.handleBrainCommand(parts[1:])
	case "metrics":
		sess := d.deps.SessionFn()
		if sess == nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "No active session."}}}
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Current Session Metrics (%s):\n", sess.ID)
		fmt.Fprintf(&sb, "- Created at: %s\n", sess.CreatedAt.Local().Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&sb, "- History Messages: %d\n", len(sess.History))

		sb.WriteString("\nLast Turn Token Usage:\n")
		lastInput := sess.LastInputTokens
		fmt.Fprintf(&sb, "- Input Tokens: %d\n", lastInput)
		if lastInput > 0 {
			fmt.Fprintf(&sb, "  * System Prompt (Static):  %d (%.1f%% of input)\n",
				sess.LastSystemStaticTokens,
				float64(sess.LastSystemStaticTokens)/float64(lastInput)*100)
			fmt.Fprintf(&sb, "  * System Prompt (Dynamic): %d (%.1f%% of input)\n",
				sess.LastSystemDynamicTokens,
				float64(sess.LastSystemDynamicTokens)/float64(lastInput)*100)
			fmt.Fprintf(&sb, "  * Tool Definitions:       %d (%.1f%% of input)\n",
				sess.LastToolsTokens,
				float64(sess.LastToolsTokens)/float64(lastInput)*100)
			fmt.Fprintf(&sb, "  * History & Query:        %d (%.1f%% of input)\n",
				sess.LastHistoryTokens,
				float64(sess.LastHistoryTokens)/float64(lastInput)*100)
		}

		sb.WriteString("\nCumulative Token Usage:\n")
		totalInput := sess.TotalInputTokens
		fmt.Fprintf(&sb, "- Input Tokens: %d\n", totalInput)
		if totalInput > 0 {
			fmt.Fprintf(&sb, "  * System Prompt (Static):  %d (%.1f%% of input)\n",
				sess.TotalSystemStaticTokens,
				float64(sess.TotalSystemStaticTokens)/float64(totalInput)*100)
			fmt.Fprintf(&sb, "  * System Prompt (Dynamic): %d (%.1f%% of input)\n",
				sess.TotalSystemDynamicTokens,
				float64(sess.TotalSystemDynamicTokens)/float64(totalInput)*100)
			fmt.Fprintf(&sb, "  * Tool Definitions:       %d (%.1f%% of input)\n",
				sess.TotalToolsTokens,
				float64(sess.TotalToolsTokens)/float64(totalInput)*100)
			fmt.Fprintf(&sb, "  * History & Query:        %d (%.1f%% of input)\n",
				sess.TotalHistoryTokens,
				float64(sess.TotalHistoryTokens)/float64(totalInput)*100)
		}
		if totalInput > 0 && sess.TotalCacheReadTokens > 0 {
			fmt.Fprintf(&sb, "  * Cache Read:  %d (%.1f%% of input)\n",
				sess.TotalCacheReadTokens,
				float64(sess.TotalCacheReadTokens)/float64(totalInput)*100)
		}
		if sess.TotalCacheWriteTokens > 0 {
			fmt.Fprintf(&sb, "  * Cache Write: %d\n", sess.TotalCacheWriteTokens)
		}
		fmt.Fprintf(&sb, "- Output Tokens: %d\n", sess.TotalOutputTokens)
		if sess.TotalOutputTokens > 0 && sess.TotalReasoningTokens > 0 {
			fmt.Fprintf(&sb, "  * Reasoning (Thinking) Tokens: %d (%.1f%% of output)\n",
				sess.TotalReasoningTokens,
				float64(sess.TotalReasoningTokens)/float64(sess.TotalOutputTokens)*100)
		}
		fmt.Fprintf(&sb, "- Total Tokens:  %d\n", sess.TotalTokens)
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: sb.String()}}}
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown command: /%s", command)}}}
	}
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
			{Kind: types.EntrySystem, Text: fmt.Sprintf("Accion pendiente: %s Usa /approve o /deny.", decision.Reason)},
		}}
	}

	return types.Response{
		Entries: []types.Entry{
			{Kind: types.EntryCommand, Text: "$ " + command},
			{Kind: types.EntrySystem, Text: "Ejecutando comando..."},
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
