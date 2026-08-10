package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/app/types"
)

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

	switch strings.ToLower(parts[0]) {
	case CmdList:
		return d.listBrainFiles()
	case "read":
		return d.handleBrainRead(br, parts[1:])
	case "plan":
		return readBrainFile(br, "plan.md", "Session Plan (plan.md)")
	case "tasks":
		return readBrainFile(br, "tasks.md", "Session Tasks (tasks.md)")
	case "summary":
		return readBrainFile(br, "summary.md", "Session Summary (summary.md)")
	case CmdClear:
		return d.handleBrainClear(br)
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown subcommand: %s. Available subcommands: list, read, plan, tasks, summary, clear.", parts[0])}}}
	}
}

func (d *Dispatcher) handleBrainRead(br *brain.Brain, args []string) types.Response {
	if len(args) < 1 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /brain read <filename>"}}}
	}
	filename := args[0]
	content, err := br.Read(filename)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Failed to read brain file: %v", err)}}}
	}
	return types.Response{Entries: []types.Entry{
		{Kind: types.EntrySystem, Text: fmt.Sprintf("--- Brain File: %s ---", filename)},
		{Kind: types.EntrySystem, Text: content},
	}}
}

func (d *Dispatcher) handleBrainClear(br *brain.Brain) types.Response {
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

// readBrainFile renders a named brain file under a friendly header.
func readBrainFile(br *brain.Brain, filename, title string) types.Response {
	content, err := br.Read(filename)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("No %s found in session brain.", filename)}}}
	}
	return types.Response{Entries: []types.Entry{
		{Kind: types.EntrySystem, Text: "--- " + title + " ---"},
		{Kind: types.EntrySystem, Text: content},
	}}
}

func countTaskCheckboxes(br *brain.Brain) (pending int, completed int) {
	if br == nil {
		return 0, 0
	}
	return br.TaskCounts()
}
