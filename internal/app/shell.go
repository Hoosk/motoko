package app

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

const shellTimeout = 20 * time.Second
const maxOutputBytes = 12_000

type ShellDecision struct {
	Reason           string
	RequiresApproval bool
	Deny             bool
}

type ShellResult struct {
	Command  string
	Output   string
	ExitCode int
	Duration time.Duration
}

func classifyShell(mode Mode, command string) ShellDecision {
	normalized := strings.ToLower(strings.TrimSpace(command))
	if normalized == "" {
		return ShellDecision{Deny: true, Reason: "Comando vacio."}
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
			return ShellDecision{Deny: true, Reason: "Comando bloqueado por politica de seguridad."}
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
			return ShellDecision{RequiresApproval: true, Reason: "El comando puede modificar archivos o el estado del repositorio."}
		}
	}

	if mode == ModePlan {
		return ShellDecision{RequiresApproval: true, Reason: "Plan mode requires approval for shell commands."}
	}

	return ShellDecision{}
}

func RunShellCommand(parent context.Context, command string) ShellResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, shellTimeout)
	defer cancel()

	wd, err := os.Getwd()
	if err != nil {
		return ShellResult{Command: command, Output: "Could not resolve the current workspace.", ExitCode: -1, Duration: time.Since(start)}
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()

	result := ShellResult{
		Command:  command,
		Output:   trimOutput(string(output)),
		ExitCode: 0,
		Duration: time.Since(start),
	}

	if err == nil {
		return result
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.Output = trimOutput(result.Output + "\nCommand cancelled due to timeout.")
		result.ExitCode = 124
		return result
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result
	}

	result.ExitCode = -1
	if strings.TrimSpace(result.Output) == "" {
		result.Output = "El comando no pudo ejecutarse."
	}
	return result
}

func trimOutput(output string) string {
	trimmed := strings.TrimSpace(output)
	if len(trimmed) <= maxOutputBytes {
		return trimmed
	}

	return trimmed[:maxOutputBytes] + "\n...[salida truncada]"
}
