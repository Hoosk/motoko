package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/tracelog"
)

func (r *Runtime) handleSlashCommand(input string, info system.ContextInfo) Response {
	parts := strings.Fields(strings.TrimPrefix(input, "/"))
	if len(parts) == 0 {
		return Response{}
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "help":
		return Response{Entries: []Entry{{Kind: EntryHelp, Text: strings.Join([]string{
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
		return Response{Signal: "quit"}
	case "themes":
		if len(parts) < 2 {
			current := r.config.Theme
			if current == "" {
				current = "cyberpunk"
			}
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf(
				"Current theme: %s\n" +
				"Available themes:\n" +
				"  cyberpunk    Default dark neon green (default)\n" +
				"  ghost-cyber  Restrained dark cyberpunk with precise accents\n" +
				"  neon-shadow  Dramatic high-contrast magenta and cyan\n" +
				"  black-ice    Cold technical with ice-blue accents\n" +
				"  nord         Arctic blue palette\n" +
				"  dracula      Classic purple and green\n" +
				"  monochrome   Pure green-on-black terminal\n" +
				"Usage: /themes <name>",
				current)}}}
		}
		themeName := strings.ToLower(parts[1])
		switch themeName {
		case "cyberpunk", "ghost-cyber", "neon-shadow", "black-ice", "nord", "dracula", "monochrome":
			r.config.Theme = themeName
			_ = r.config.Save()
			styles.SetTheme(themeName)
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Theme changed to: " + themeName}}}
		default:
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Unknown theme: %s. Available: cyberpunk, ghost-cyber, neon-shadow, black-ice, nord, dracula, monochrome", themeName)}}}
		}
	case cmdClear:
		if r.currentSession != nil {
			r.currentSession.History = nil
			r.currentSession.LastInputTokens = 0
			_ = r.currentSession.Save()
		}
		return Response{Clear: true, Entries: []Entry{{Kind: EntrySystem, Text: "Timeline reset."}}}
	case "compact":
		return Response{
			Entries: []Entry{{Kind: EntrySystem, Text: "Compacting session..."}},
			Action:  &Action{Type: ActionCompact},
		}
	case string(ModePlan):
		r.SetAgentMode(string(ModePlan))
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Mode set to: plan. Shell commands require explicit approval."}}}
	case "build":
		r.SetAgentMode("build")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Mode set to: build. Safe commands run directly; sensitive ones require approval."}}}
	case "agent":
		if len(parts) < 2 {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Active agent: %s. Available agents: %s", r.AgentName(), strings.Join(r.AgentNames(), ", "))}}}
		}
		agentName := parts[1]
		found := false
		for _, name := range r.AgentNames() {
			if strings.EqualFold(name, agentName) {
				r.SetAgentMode(name)
				found = true
				break
			}
		}
		if !found {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Unknown agent: %s", agentName)}}}
		}
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Agent switched to: %s", r.AgentName())}}}
	case "mode":
		return Response{Signal: "open-mode-popup"}
	case "shell":
		r.inputMode = InputModeShell
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Input mode: shell. Any line not starting with / will be executed as a command."}}}
	case "chat":
		r.inputMode = InputModeChat
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Input mode: chat. Normal input will be treated as a prompt."}}}
	case cmdStatus:
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: r.statusText(info)}}}
	case "debug":
		r.debug = !r.debug
		if r.agent != nil {
			r.agent.SetDebug(r.debug)
		}
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Agent debug: %t", r.debug)}}}
	case "context":
		rawPrompt := r.SystemPrompt(info)
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "--- RAW AGENT SYSTEM PROMPT ---\n\n" + rawPrompt}}}
	case "provider":
		return r.handleProviderCommand(parts[1:])
	case "models":
		return r.handleModelsCommand(parts[1:])
	case "sessions":
		return Response{Signal: "open-sessions-popup"}
	case "tools":
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: formatToolList(r.ToolSpecs())}}}
	case cmdTool:
		if len(parts) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Usage: /tool <name> <args>. Use /tools to list available ones."}}}
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
			return r.handleShell(toolArgs)
		}

		runCtx := tools.WithBrain(context.Background(), r.brain)
		runCtx = tools.WithMaxOutputSize(runCtx, system.MaxToolOutputBytes(r.contextWindow))
		result, err := r.tools.Run(runCtx, toolName, toolArgs)
		if err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}

		entries := []Entry{
			{Kind: EntryCommand, Text: fmt.Sprintf("tool %s %s", toolName, strings.TrimSpace(toolArgs))},
			{Kind: EntrySystem, Text: result.Summary},
		}
		if strings.TrimSpace(result.Output) != "" {
			entries = append(entries, Entry{Kind: EntryOutput, Text: result.Output})
		}
		return Response{Entries: entries}
	case "approve":
		if r.pending == nil {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No pending action."}}}
		}

		pending := r.pending
		r.pending = nil
		return Response{
			Entries: []Entry{
				{Kind: EntryCommand, Text: "$ " + pending.Command},
				{Kind: EntrySystem, Text: "Approval received. Executing command..."},
			},
			Action: &Action{Type: ActionShell, ShellCommand: pending.Command},
		}
	case "deny":
		if r.pending == nil {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No pending action."}}}
		}

		pendingCmd := r.pending.Command
		r.pending = nil
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Action cancelled: %s", pendingCmd)}}}
	case "trace":
		if !tracelog.Available() {
			return Response{}
		}
		enabled := tracelog.SetEnabled(!tracelog.Enabled())
		if enabled {
			tracelog.Logf("=== TRACE ENABLED ===")
		}
		return Response{}
	case "task":
		if len(parts) < 2 {
			tasks := r.ListTasks()
			if len(tasks) == 0 {
				return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No active background tasks."}}}
			}
			var sb strings.Builder
			sb.WriteString("Active tasks:\n")
			for _, t := range tasks {
				fmt.Fprintf(&sb, "- %s: %q (started %s ago)\n", t.ID, t.Command, time.Since(t.Started).Round(time.Second))
			}
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: strings.TrimSpace(sb.String())}}}
		}

		subcmd := strings.ToLower(parts[1])
		switch subcmd {
		case cmdList:
			tasks := r.ListTasks()
			if len(tasks) == 0 {
				return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No active background tasks."}}}
			}
			var sb strings.Builder
			sb.WriteString("Active tasks:\n")
			for _, t := range tasks {
				fmt.Fprintf(&sb, "- %s: %q (started %s ago)\n", t.ID, t.Command, time.Since(t.Started).Round(time.Second))
			}
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: strings.TrimSpace(sb.String())}}}
		case "terminate":
			if len(parts) < 3 {
				return Response{Entries: []Entry{{Kind: EntryError, Text: "Usage: /task terminate <idTask>"}}}
			}
			id := parts[2]
			if err := r.TerminateTask(id); err != nil {
				return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
			}
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Task %s terminated.", id)}}}
		default:
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Unknown subcommand: %s. Usage: /task or /task terminate <idTask>", subcmd)}}}
		}
	case "brain":
		return r.handleBrainCommand(parts[1:])
	case "metrics":
		if r.currentSession == nil {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No active session."}}}
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Current session metrics (%s):\n", r.currentSession.ID)
		fmt.Fprintf(&sb, "- Created at: %s\n", r.currentSession.CreatedAt.Local().Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&sb, "- History messages: %d\n", len(r.currentSession.History))
		sb.WriteString("\nAccumulated Token Usage:\n")
		fmt.Fprintf(&sb, "- Input Tokens: %d\n", r.currentSession.TotalInputTokens)
		if r.currentSession.TotalInputTokens > 0 && r.currentSession.TotalCacheReadTokens > 0 {
			fmt.Fprintf(&sb, "  * Read from cache: %d (%.1f%% of input)\n", 
				r.currentSession.TotalCacheReadTokens, 
				float64(r.currentSession.TotalCacheReadTokens)/float64(r.currentSession.TotalInputTokens)*100)
		}
		if r.currentSession.TotalCacheWriteTokens > 0 {
			fmt.Fprintf(&sb, "  * Written to cache: %d\n", r.currentSession.TotalCacheWriteTokens)
		}
		fmt.Fprintf(&sb, "- Output Tokens:  %d\n", r.currentSession.TotalOutputTokens)
		if r.currentSession.TotalOutputTokens > 0 && r.currentSession.TotalReasoningTokens > 0 {
			fmt.Fprintf(&sb, "  * Reasoning (Thinking) Tokens: %d (%.1f%% of output)\n", 
				r.currentSession.TotalReasoningTokens, 
				float64(r.currentSession.TotalReasoningTokens)/float64(r.currentSession.TotalOutputTokens)*100)
		}
		fmt.Fprintf(&sb, "- Total Tokens:       %d\n", r.currentSession.TotalTokens)
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: sb.String()}}}
	default:
		return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Unknown command: /%s", command)}}}
	}
}

func formatToolList(specs []tools.Spec) string {
	lines := []string{"Registered tools:"}
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("- %s: %s", spec.Usage, spec.Summary))
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) statusText(info system.ContextInfo) string {
	pending := valNone
	if r.pending != nil {
		pending = r.pending.Command
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
		fmt.Sprintf("mode: %s", r.mode),
		fmt.Sprintf("input: %s", r.inputMode),
		fmt.Sprintf("agent configured: %t", r.AgentConfigured()),
		fmt.Sprintf("active provider: %s", r.ProviderSummary()),
		fmt.Sprintf("workspace: %s", info.Workspace),
		fmt.Sprintf("git: %s", info.GitSummary()),
		fmt.Sprintf("agents.md guidelines: %s", agentsStatus),
		fmt.Sprintf("design.md specification: %s", designStatus),
		fmt.Sprintf("pending approval: %s", pending),
		"policy: plan asks for shell approval; build runs safe commands and asks for sensitive ones.",
	}, "\n")
}

func (r *Runtime) handleProviderCommand(args []string) Response {
	if len(args) == 0 {
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: strings.Join([]string{
			"Provider usage:",
			"/provider list",
			"/provider add",
			"/provider use <name>",
			"/provider remove <name>",
		}, "\n")}}}
	}

	subcommand := strings.ToLower(args[0])
	switch subcommand {
	case cmdList:
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: r.providerListText()}}}
	case "add":
		if len(args) >= 5 {
			name := args[1]
			preset := config.ProviderPreset(args[2])
			baseURL := args[3]
			apiKey := args[4]
			newProv := config.ProviderConfig{
				Name:    name,
				Preset:  preset,
				BaseURL: baseURL,
				APIKey:  apiKey,
			}
			newProv = config.NormalizeProvider(newProv)
			r.config.UpsertProvider(newProv)
			if err := r.config.Save(); err != nil {
				return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
			}
			r.refreshAgent()
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Provider added and saved: %s", name)}}}
		}
		return Response{Signal: "open-provider-popup", Entries: []Entry{{Kind: EntrySystem, Text: "Opening provider configuration form..."}}}
	case "use":
		if len(args) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Usage: /provider use <name>"}}}
		}
		name := strings.Join(args[1:], " ")
		if err := r.config.SetActive(name); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		if err := r.config.Save(); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		r.refreshAgent()
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Active provider: %s", r.ProviderSummary())}}}
	case "remove":
		if len(args) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Usage: /provider remove <name>"}}}
		}
		name := strings.Join(args[1:], " ")
		if !r.config.RemoveProvider(name) {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Provider not found: %s", name)}}}
		}
		if err := r.config.Save(); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		r.refreshAgent()
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Provider removed: %s", name)}}}
	default:
		return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Unknown subcommand: %s", subcommand)}}}
	}
}

func (r *Runtime) handleModelsCommand(args []string) Response {
	active, ok := r.config.Active()
	if !ok {
		return Response{Entries: []Entry{{Kind: EntryError, Text: "No active provider. Use /provider add or /provider use first."}}}
	}

	if len(args) == 0 {
		return Response{Signal: "open-models-popup"}
	}

	if len(args) > 1 && strings.EqualFold(args[0], "use") {
		args = args[1:]
	}

	model := strings.TrimSpace(strings.Join(args, " "))
	ctx := context.Background()
	var supportsThinking bool
	var contextWindow int
	if client, err := r.providerClient(active); err == nil {
		if info, modelErr := client.GetModel(ctx, model); modelErr == nil {
			supportsThinking = info.SupportsThinking
			contextWindow = info.ContextWindow
			tracelog.Logf("runtime handleModelsCommand: model %q resolved: supportsThinking=%t contextWindow=%d", model, supportsThinking, contextWindow)
		} else {
			tracelog.Logf("runtime handleModelsCommand: failed to resolve model %q: %v", model, modelErr)
		}
	} else {
		tracelog.Logf("runtime handleModelsCommand: failed to load provider client: %v", err)
	}

	active.Model = model
	active.Models = config.UniqueSortedKeep(active.Models, model)
	active.ContextWindow = contextWindow
	active.SupportsThinking = supportsThinking
	r.config.UpsertProvider(active)
	if err := r.config.Save(); err != nil {
		return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
	}
	r.refreshAgent()
	return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Active model for %s: %s", active.Name, active.Model)}}}
}

func (r *Runtime) providerListText() string {
	if r.config == nil || len(r.config.Providers) == 0 {
		return "No providers configured. Use /provider add."
	}

	lines := []string{"Configured providers:"}
	for _, providerCfg := range r.config.Providers {
		marker := " "
		if strings.EqualFold(providerCfg.Name, r.config.ActiveProvider) {
			marker = "*"
		}
		model := providerCfg.Model
		if strings.TrimSpace(model) == "" {
			model = "no-model"
		}
		label := string(providerCfg.Preset)
		if strings.TrimSpace(label) == "" {
			label = string(providerCfg.Kind)
		}
		lines = append(lines, fmt.Sprintf("%s %s [%s] %s", marker, providerCfg.Name, label, model))
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) handleBrainCommand(parts []string) Response {
	if r.brain == nil {
		if r.brainInitErr != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Session brain not initialized: %v", r.brainInitErr)}}}
		}
		return Response{Entries: []Entry{{Kind: EntryError, Text: "Session brain not initialized."}}}
	}

	if len(parts) == 0 {
		return r.listBrainFiles()
	}

	subcmd := strings.ToLower(parts[0])
	switch subcmd {
	case cmdList:
		return r.listBrainFiles()
	case "read":
		if len(parts) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Usage: /brain read <filename>"}}}
		}
		filename := parts[1]
		content, err := r.brain.Read(filename)
		if err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Failed to read brain file: %v", err)}}}
		}
		return Response{Entries: []Entry{
			{Kind: EntrySystem, Text: fmt.Sprintf("--- Brain File: %s ---", filename)},
			{Kind: EntrySystem, Text: content},
		}}
	case "plan":
		content, err := r.brain.Read("plan.md")
		if err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "No plan.md found in session brain."}}}
		}
		return Response{Entries: []Entry{
			{Kind: EntrySystem, Text: "--- Session Plan (plan.md) ---"},
			{Kind: EntrySystem, Text: content},
		}}
	case "tasks":
		content, err := r.brain.Read("tasks.md")
		if err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "No tasks.md found in session brain."}}}
		}
		return Response{Entries: []Entry{
			{Kind: EntrySystem, Text: "--- Session Tasks (tasks.md) ---"},
			{Kind: EntrySystem, Text: content},
		}}
	case "summary":
		content, err := r.brain.Read("summary.md")
		if err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "No summary.md found in session brain."}}}
		}
		return Response{Entries: []Entry{
			{Kind: EntrySystem, Text: "--- Session Summary (summary.md) ---"},
			{Kind: EntrySystem, Text: content},
		}}
	case cmdClear:
		files, err := r.brain.List()
		if err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Failed to list brain files: %v", err)}}}
		}
		var deleteErrors []string
		for _, f := range files {
			if err := r.brain.Delete(f.Name); err != nil {
				deleteErrors = append(deleteErrors, fmt.Sprintf("%s: %v", f.Name, err))
			}
		}
		if len(deleteErrors) > 0 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Failed to delete some brain files: %s", strings.Join(deleteErrors, "; "))}}}
		}
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "All session brain files deleted."}}}
	default:
		return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Unknown subcommand: %s. Available subcommands: list, read, plan, tasks, summary, clear.", subcmd)}}}
	}
}

func (r *Runtime) listBrainFiles() Response {
	files, err := r.brain.List()
	if err != nil {
		return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Failed to list brain files: %v", err)}}}
	}
	if len(files) == 0 {
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No brain files in the current session."}}}
	}
	var sb strings.Builder
	sb.WriteString("Session brain files:\n")
	for _, f := range files {
		ago := time.Since(f.ModTime).Truncate(time.Second)
		fmt.Fprintf(&sb, "- %s (%d bytes, updated %s ago)\n", f.Name, f.SizeBytes, ago)
	}
	return Response{Entries: []Entry{{Kind: EntrySystem, Text: strings.TrimSpace(sb.String())}}}
}
