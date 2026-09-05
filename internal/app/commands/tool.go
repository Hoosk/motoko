package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app/types"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
)

func (d *Dispatcher) handleToolCommand(inv Invocation) types.Response {
	toolName, toolArgs := toolInvocation(inv)
	if toolName == "" {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /tool <name> <args>. Use /tools to list available ones."}}}
	}
	if strings.EqualFold(toolName, "bash") {
		return d.handleShell(toolArgs)
	}
	runCtx := tools.WithBrain(context.Background(), d.deps.BrainFn())
	cfg := d.deps.ConfigFn()
	runCtx = tools.WithConfig(runCtx, cfg)
	runCtx = tools.WithMaxOutputSize(runCtx, system.MaxToolOutputBytes(d.deps.ContextWindowFn()))
	if cfg != nil && tools.IsWriteTool(toolName) && config.NormalizeEditApproval(cfg.EditApproval) == config.EditApprovalAsk {
		return types.Response{
			Entries: []types.Entry{
				{Kind: types.EntryCommand, Text: fmt.Sprintf("tool %s %s", toolName, strings.TrimSpace(toolArgs))},
				{Kind: types.EntrySystem, Text: "Awaiting file change approval..."},
			},
			Action: &types.Action{Type: types.ActionTool, ToolName: toolName, ToolArgs: toolArgs},
		}
	}
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

// toolInvocation splits a raw /tool invocation into the tool name and the
// argument string that follows it, preserving internal spacing so the tool
// receives the exact command text the user typed.
func toolInvocation(inv Invocation) (name, args string) {
	if len(inv.Args) < 1 {
		return "", ""
	}
	name = inv.Args[0]
	raw := strings.TrimPrefix(inv.RawInput, "/")
	rest := raw
	for len(rest) > 0 && isSpaceChar(rest[0]) {
		rest = rest[1:]
	}
	for len(rest) > 0 && !isSpaceChar(rest[0]) {
		rest = rest[1:]
	}
	for len(rest) > 0 && isSpaceChar(rest[0]) {
		rest = rest[1:]
	}
	for len(rest) > 0 && !isSpaceChar(rest[0]) {
		rest = rest[1:]
	}
	for len(rest) > 0 && isSpaceChar(rest[0]) {
		rest = rest[1:]
	}
	return name, rest
}

func isSpaceChar(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func formatToolList(specs []tools.Spec) string {
	lines := []string{"Registered tools:"}
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("- %s: %s", spec.Usage, spec.Summary))
	}
	return strings.Join(lines, "\n")
}
