package app

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app/shell"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/system"
)

func (r *Runtime) HandleInput(input string, info system.ContextInfo) Response {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Response{}
	}

	if r.tachikomas != nil {
		r.tachikomas.SetActivePrompt(trimmed)
	}

	if strings.HasPrefix(trimmed, "/") {
		return r.handleSlashCommand(trimmed, info)
	}

	// Detect leading agent mention like "@search find all symbols"
	fields := strings.Fields(trimmed)
	if len(fields) > 0 && strings.HasPrefix(fields[0], "@") && r.isAgentMention(fields[0]) {
		agentName := strings.TrimPrefix(fields[0], "@")
		r.SetAgentMode(agentName)
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
	} else {
		// Generic fallback: check any word in the message
		for _, field := range fields {
			if strings.HasPrefix(field, "@") && r.isAgentMention(field) {
				r.SetAgentMode(strings.TrimPrefix(field, "@"))
				break
			}
		}
	}
	r.agOrch.SetMentionedFiles(r.extractMentionedFiles(trimmed))

	if strings.HasPrefix(trimmed, "!") {
		return r.handleCommand(strings.TrimSpace(trimmed[1:]))
	}

	if r.inputMode == InputModeShell {
		return r.handleCommand(trimmed)
	}

	if r.AgentConfigured() {
		var entries []Entry
		if !strings.HasPrefix(trimmed, "[System:") {
			entries = append(entries, Entry{Kind: EntryUser, Text: trimmed})
		}
		return Response{Entries: entries, Action: &Action{Type: ActionAgent, AgentPrompt: trimmed}}
	}

	return Response{Entries: []Entry{
		{Kind: EntryUser, Text: trimmed},
		{Kind: EntryAssistant, Text: "The runtime is operational but the agent is not ready. Configure a provider using /provider add and then select a model using /models use <model>."},
	}}
}

func (r *Runtime) HandleShellResult(result ShellResult) Response {
	status := fmt.Sprintf("Command finished in %s with exit code %d.", result.Duration.Round(10_000_000), result.ExitCode)
	entries := []Entry{{Kind: EntrySystem, Text: status}}

	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = "(no output)"
	}

	if result.ExitCode == 0 {
		entries = append(entries, Entry{Kind: EntryOutput, Text: output})
		return Response{Entries: entries}
	}

	entries = append(entries, Entry{Kind: EntryError, Text: output})
	return Response{Entries: entries}
}

func (r *Runtime) HandleTaskResult(result TaskEvent) Response {
	status := fmt.Sprintf("Task finished in %s with exit code %d.", result.Duration.Round(10_000_000), result.ExitCode)
	entries := []Entry{{Kind: EntrySystem, Text: status}}

	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = "(no output)"
	}

	if r.sesMgr.CurrentSession() != nil {
		r.sesMgr.CurrentSession().History = append(r.sesMgr.CurrentSession().History, provider.ConversationItem{
			Role:    provider.RoleUser,
			Content: fmt.Sprintf("[System: Task %s finished with exit code %d.\nOutput:\n%s]", result.ID, result.ExitCode, output),
		})
		_ = r.sesMgr.CurrentSession().Save()
	}

	if result.ExitCode == 0 {
		entries = append(entries, Entry{Kind: EntryOutput, Text: output})
		return Response{Entries: entries}
	}

	entries = append(entries, Entry{Kind: EntryError, Text: output})
	return Response{Entries: entries}
}

func (r *Runtime) handleShell(command string) Response {
	if command == "" {
		return Response{Entries: []Entry{{Kind: EntryError, Text: "Missing command after !"}}}
	}

	decision := shell.Classify(r.agOrch.Mode(), command)
	if decision.Deny {
		return Response{Entries: []Entry{{Kind: EntryError, Text: decision.Reason}}}
	}

	if decision.RequiresApproval {
		r.pending = &pendingShell{Command: command}
		return Response{Entries: []Entry{
			{Kind: EntryCommand, Text: "$ " + command},
			{Kind: EntrySystem, Text: fmt.Sprintf("Pending action: %s Use /approve or /deny.", decision.Reason)},
		}}
	}

	return Response{
		Entries: []Entry{
			{Kind: EntryCommand, Text: "$ " + command},
			{Kind: EntrySystem, Text: "Executing command..."},
		},
		Action: &Action{Type: ActionShell, ShellCommand: command},
	}
}

func (r *Runtime) handleCommand(command string) Response {
	return r.handleShell(command)
}
