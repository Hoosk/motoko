package app

import (
	"context"
	"strings"
	"testing"
)

func TestClassifyShell(t *testing.T) {
	if got := classifyShell(ModeBuild, "rm -rf /tmp/x"); !got.Deny {
		t.Fatalf("expected dangerous command denied, got %#v", got)
	}
	if got := classifyShell(ModeBuild, "touch test.txt"); !got.RequiresApproval {
		t.Fatalf("expected mutating command to require approval, got %#v", got)
	}
	if got := classifyShell(ModePlan, "pwd"); !got.RequiresApproval {
		t.Fatalf("expected plan mode to require approval, got %#v", got)
	}
	if got := classifyShell(ModeBuild, "pwd"); got.RequiresApproval || got.Deny {
		t.Fatalf("expected pwd allowed in build, got %#v", got)
	}
}

func TestRunShellCommandAndTrimOutput(t *testing.T) {
	result := RunShellCommand(context.Background(), "printf hola")
	if result.ExitCode != 0 || result.Output != "hola" {
		t.Fatalf("unexpected shell result %#v", result)
	}
	failure := RunShellCommand(context.Background(), "exit 9")
	if failure.ExitCode != 9 {
		t.Fatalf("expected exit code 9, got %#v", failure)
	}
	trimmed := trimOutput(strings.Repeat("a", maxOutputBytes+10))
	if !strings.Contains(trimmed, "[salida truncada]") {
		t.Fatalf("expected truncated output marker, got %q", trimmed)
	}
}
