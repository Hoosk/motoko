package commands

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app/types"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tracelog"
)

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

func (d *Dispatcher) handleModePresetCommand(command string) Handler {
	return func(inv Invocation) types.Response {
		d.deps.SetAgentModeFn(command)
		if command == string(types.ModePlan) {
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Mode set to: plan. Shell commands require explicit approval."}}}
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Mode set to: build. Safe commands run directly; sensitive ones require approval."}}}
	}
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
