package commands

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app/types"
)

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

func goalKickoffPrompt(goal string) string {
	return strings.TrimSpace(fmt.Sprintf(`A persistent goal has been set for this session:

%s

Requirements:
- If tasks.md does not exist or is too vague, create or refine it first.
- Continue executing the next unfinished task.
- Keep tasks.md updated with [x] as tasks complete.
- The system will keep prompting you to continue until all tasks are done or the user clears the goal.`, goal))
}
