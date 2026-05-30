package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
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
			"/help     Show this help message",
			"/clear    Clear the timeline history",
			"/compact  Manually compact the active session",
			"/mode     Open the agent mode selector",
			"/plan     Activate read-only plan mode",
			"/build    Activate active build mode",
			"/agent    Switch or show active agent mode",
			"/shell    Activate direct shell execution mode",
			"/chat     Return to normal chat mode",
			"/status   Summarize mode, permissions, and approvals",
			"/trace    Toggle trace logging to file (if compiled with -tags motoko_trace)",
			"/context  Show raw system prompt being sent to the agent",
			"/provider Manage configured LLM providers",
			"/models   List or select models from the active provider",
			"/sessions List or switch between workspace sessions",
			"/tools    Show all registered tools",
			"/tool     Execute a specific runtime tool",
			"/approve  Execute the pending shell action",
			"/deny     Cancel the pending shell action",
			"!<cmd>    Execute an explicit shell command",
		}, "\n")}}}
	case "clear":
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
	case "plan":
		r.SetAgentMode("plan")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Mode set to: plan. Shell commands require explicit approval."}}}
	case "build":
		r.SetAgentMode("build")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Mode set to: build. Safe commands run directly; sensitive ones require approval."}}}
	case "agent":
		if len(parts) < 2 {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Agente activo: %s. Agentes disponibles: %s", r.AgentName(), strings.Join(r.AgentNames(), ", "))}}}
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
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Agente desconocido: %s", agentName)}}}
		}
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Agente cambiado a: %s", r.AgentName())}}}
	case "mode":
		return Response{Signal: "open-mode-popup"}
	case "shell":
		r.inputMode = InputModeShell
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Input mode: shell. Any line not starting with / will be executed as a command."}}}
	case "chat":
		r.inputMode = InputModeChat
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Input mode: chat. Normal input will be treated as a prompt."}}}
	case "status":
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
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: formatToolList(r.tools.Specs())}}}
	case "tool":
		if len(parts) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Usage: /tool <name> <args>. Use /tools to list available ones."}}}
		}

		toolName := parts[1]
		toolArgs := ""
		if len(parts) > 2 {
			toolArgs = strings.Join(parts[2:], " ")
		}
		if strings.EqualFold(toolName, "bash") {
			return r.handleShell(toolArgs)
		}

		result, err := r.tools.Run(context.Background(), toolName, toolArgs)
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

		command := r.pending.Command
		r.pending = nil
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Action cancelled: %s", command)}}}
	case "trace":
		if !tracelog.Available() {
			return Response{}
		}
		enabled := tracelog.SetEnabled(!tracelog.Enabled())
		if enabled {
			tracelog.Logf("=== TRACE ENABLED ===")
		}
		return Response{}
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
	pending := "none"
	if r.pending != nil {
		pending = r.pending.Command
	}

	return strings.Join([]string{
		fmt.Sprintf("mode: %s", r.mode),
		fmt.Sprintf("input: %s", r.inputMode),
		fmt.Sprintf("agent configured: %t", r.AgentConfigured()),
		fmt.Sprintf("active provider: %s", r.ProviderSummary()),
		fmt.Sprintf("workspace: %s", info.Workspace),
		fmt.Sprintf("git: %s", info.GitSummary()),
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
	case "list":
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: r.providerListText()}}}
	case "add":
		return Response{Signal: "open-provider-popup", Entries: []Entry{{Kind: EntrySystem, Text: "Opening provider configuration form..."}}}
	case "use":
		if len(args) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Usage: /provider use <name>"}}}
		}
		if err := r.config.SetActive(args[1]); err != nil {
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
		if !r.config.RemoveProvider(args[1]) {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Provider not found: %s", args[1])}}}
		}
		if err := r.config.Save(); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		r.refreshAgent()
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Provider removed: %s", args[1])}}}
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

	model := strings.TrimSpace(strings.Join(args, " "))
	active.Model = model
	active.Models = config.UniqueSortedKeep(active.Models, model)
	active.ContextWindow = 0
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
