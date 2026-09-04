package patch

import (
	"strings"
	"testing"
)

func TestUnifiedDiffIncludesChangedLines(t *testing.T) {
	got := UnifiedDiff("main.go", "package old\n", "package new\n")
	for _, want := range []string{"--- a/main.go", "+++ b/main.go", "-package old", "+package new"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diff %q does not contain %q", got, want)
		}
	}
}

func TestUnifiedDiffIsEmptyWhenContentDoesNotChange(t *testing.T) {
	if got := UnifiedDiff("main.go", "same\n", "same\n"); got != "" {
		t.Fatalf("expected empty diff, got %q", got)
	}
}
