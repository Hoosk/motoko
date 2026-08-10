package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/app/types"
)

func (d *Dispatcher) handleScheduleCommand(args []string) types.Response {
	if len(args) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /schedule [list|add <instruction> every|once <duration>|remove <id>]"}}}
	}
	subcmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch subcmd {
	case CmdList:
		return d.handleScheduleList()
	case "remove":
		return d.handleScheduleRemove(args[1:])
	case "add":
		return d.handleScheduleAdd(args[1:])
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Unknown schedule subcommand. Use list, add, or remove."}}}
	}
}

func (d *Dispatcher) handleScheduleList() types.Response {
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
}

func (d *Dispatcher) handleScheduleRemove(args []string) types.Response {
	if len(args) < 1 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /schedule remove <id>"}}}
	}
	if err := d.deps.RemoveScheduleFn(strings.TrimSpace(args[0])); err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Schedule %s removed.", strings.TrimSpace(args[0]))}}}
}

func (d *Dispatcher) handleScheduleAdd(args []string) types.Response {
	instruction, every, duration, err := parseScheduleArgs(args)
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
