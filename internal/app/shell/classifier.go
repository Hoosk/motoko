package shell

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app/types"
)

func Classify(mode types.Mode, command string) types.ShellDecision {
	normalized := strings.ToLower(strings.TrimSpace(command))
	if normalized == "" {
		return types.ShellDecision{Deny: true, Reason: "Empty command."}
	}

	dangerousPatterns := []string{
		"rm -rf",
		"git reset --hard",
		"git checkout --",
		"git clean -fd",
		":(){",
		"mkfs",
		"dd if=",
		"shutdown",
		"reboot",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(normalized, pattern) {
			return types.ShellDecision{Deny: true, Reason: "Command blocked by security policy."}
		}
	}

	mutatingPatterns := []string{
		" >",
		"> ",
		">>",
		"touch ",
		"mkdir ",
		"mv ",
		"cp ",
		"git add",
		"git commit",
		"git restore",
		"git checkout ",
		"go generate",
		"go mod tidy",
		"npm install",
		"pnpm install",
		"yarn add",
		"tee ",
	}
	for _, pattern := range mutatingPatterns {
		if strings.Contains(normalized, pattern) {
			return types.ShellDecision{RequiresApproval: true, Reason: "The command may modify files or repository state."}
		}
	}

	if mode == types.ModePlan {
		return types.ShellDecision{RequiresApproval: true, Reason: "Plan mode requires approval for shell commands."}
	}

	return types.ShellDecision{}
}

// PrepareCommand turns a user shell request into either an immediate action,
// a dialog-backed approval action, or a policy error.
func PrepareCommand(mode types.Mode, command string) types.Response {
	command = strings.TrimSpace(command)
	if command == "" {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Missing command after !"}}}
	}

	decision := Classify(mode, command)
	if decision.Deny {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: decision.Reason}}}
	}

	actionType := types.ActionShell
	status := "Executing command..."
	if decision.RequiresApproval {
		actionType = types.ActionShellApproval
		status = "Awaiting shell approval..."
	}

	action := &types.Action{
		Type:         actionType,
		ShellCommand: command,
		ShellReason:  decision.Reason,
	}
	return types.Response{
		Entries: []types.Entry{
			{Kind: types.EntryCommand, Text: "$ " + command},
			{Kind: types.EntrySystem, Text: status},
		},
		Action: action,
	}
}
