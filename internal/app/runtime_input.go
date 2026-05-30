package app

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/system"
)

func (r *Runtime) HandleInput(input string, info system.ContextInfo) Response {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Response{}
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
	r.mentionedFiles = r.extractMentionedFiles(trimmed)

	if strings.HasPrefix(trimmed, "!") {
		return r.handleShell(strings.TrimSpace(trimmed[1:]))
	}

	if r.inputMode == InputModeShell {
		return r.handleShell(trimmed)
	}

	if r.AgentConfigured() {
		return Response{Entries: []Entry{{Kind: EntryUser, Text: trimmed}}, Action: &Action{Type: ActionAgent, AgentPrompt: trimmed}}
	}

	return Response{Entries: []Entry{
		{Kind: EntryUser, Text: trimmed},
		{Kind: EntryAssistant, Text: "El runtime esta operativo pero el agente no esta listo. Configura un provider con /provider add y luego selecciona modelo con /models <modelo>."},
	}}
}

func (r *Runtime) HandleShellResult(result ShellResult) Response {
	status := fmt.Sprintf("Comando finalizado en %s con salida %d.", result.Duration.Round(10_000_000), result.ExitCode)
	entries := []Entry{{Kind: EntrySystem, Text: status}}

	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = "(sin salida)"
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
		return Response{Entries: []Entry{{Kind: EntryError, Text: "Falta el comando despues de !"}}}
	}

	decision := classifyShell(r.mode, command)
	if decision.Deny {
		return Response{Entries: []Entry{{Kind: EntryError, Text: decision.Reason}}}
	}

	if decision.RequiresApproval {
		r.pending = &pendingShell{Command: command}
		return Response{Entries: []Entry{
			{Kind: EntryCommand, Text: "$ " + command},
			{Kind: EntrySystem, Text: fmt.Sprintf("Accion pendiente: %s Usa /approve o /deny.", decision.Reason)},
		}}
	}

	return Response{
		Entries: []Entry{
			{Kind: EntryCommand, Text: "$ " + command},
			{Kind: EntrySystem, Text: "Ejecutando comando..."},
		},
		Action: &Action{Type: ActionShell, ShellCommand: command},
	}
}
