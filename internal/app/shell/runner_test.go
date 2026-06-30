package shell

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app/types"
)

func TestClassify(t *testing.T) {
	if got := Classify(types.ModeBuild, "rm -rf /tmp/x"); !got.Deny {
		t.Fatalf("expected dangerous command denied, got %#v", got)
	}
	if got := Classify(types.ModeBuild, "touch test.txt"); !got.RequiresApproval {
		t.Fatalf("expected mutating command to require approval, got %#v", got)
	}
	if got := Classify(types.ModePlan, "pwd"); !got.RequiresApproval {
		t.Fatalf("expected plan mode to require approval, got %#v", got)
	}
	if got := Classify(types.ModeBuild, "pwd"); got.RequiresApproval || got.Deny {
		t.Fatalf("expected pwd allowed in build, got %#v", got)
	}
}

func TestRunCommandAndTrimOutput(t *testing.T) {
	result := RunCommand(context.Background(), "printf hola")
	if result.ExitCode != 0 || result.Output != "hola" {
		t.Fatalf("unexpected shell result %#v", result)
	}
	failure := RunCommand(context.Background(), "exit 9")
	if failure.ExitCode != 9 {
		t.Fatalf("expected exit code 9, got %#v", failure)
	}
	trimmed := trimOutput(strings.Repeat("a", maxOutputBytes+10))
	if !strings.Contains(trimmed, "[salida truncada]") {
		t.Fatalf("expected truncated output marker, got %q", trimmed)
	}
}
