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
