package shell

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/app/types"
)

const shellTimeout = 20 * time.Second
const maxOutputBytes = 12_000

func RunCommand(parent context.Context, command string) types.ShellResult {
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, shellTimeout)
	defer cancel()

	wd, err := os.Getwd()
	if err != nil {
		return types.ShellResult{Command: command, Output: "Could not resolve the current workspace.", ExitCode: -1, Duration: time.Since(start)}
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()

	result := types.ShellResult{
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
