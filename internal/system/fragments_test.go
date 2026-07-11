package system

import (
	"strings"
	"testing"
)

func TestLoadFragment(t *testing.T) {
	tests := []struct {
		name         string
		contains     string
		expectExists bool
	}{
		{
			name:         "plan_active",
			expectExists: true,
			contains:     "PLAN MODE",
		},
		{
			name:         "build_switch",
			expectExists: true,
			contains:     "BUILD MODE",
		},
		{
			name:         "max_steps",
			expectExists: true,
			contains:     "maximum number of iterations",
		},
		{
			name:         "thinking_default",
			expectExists: true,
			contains:     "REASONING STYLE",
		},
		{
			name:         "non_existent_fragment",
			expectExists: false,
			contains:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LoadFragment(tt.name)
			if tt.expectExists {
				if got == "" {
					t.Fatalf("expected fragment %s to exist, but got empty string", tt.name)
				}
				if tt.contains != "" && !strings.Contains(got, tt.contains) {
					t.Errorf("expected fragment %s to contain %q, got: %q", tt.name, tt.contains, got)
				}
			} else {
				if got != "" {
					t.Errorf("expected fragment %s not to exist, but got: %q", tt.name, got)
				}
			}
		})
	}
}

func TestLoadFragmentNoSystemInjectionLabel(t *testing.T) {
	names := []string{"plan_active", "build_switch", "thinking_concise", "thinking_caveman", "thinking_default"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			got := LoadFragment(name)
			if strings.Contains(got, "SYSTEM INJECTION") {
				t.Errorf("fragment %s still uses the old 'SYSTEM INJECTION' label, got: %q", name, got)
			}
		})
	}
}

func TestThinkingFragmentsAreSubstantive(t *testing.T) {
	cases := []struct {
		name            string
		requireHeadings []string
		minLines        int
	}{
		{
			name:     "thinking_concise",
			minLines: 30,
			requireHeadings: []string{
				"## Goal",
				"## Compress aggressively",
				"## Preserve always",
				"## Anti-patterns",
			},
		},
		{
			name:     "thinking_caveman",
			minLines: 30,
			requireHeadings: []string{
				"## Goal",
				"## Hard rules",
				"## Format",
				"## Examples",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LoadFragment(tc.name)
			if got == "" {
				t.Fatalf("fragment %s is empty", tc.name)
			}
			lineCount := strings.Count(got, "\n") + 1
			if lineCount < tc.minLines {
				t.Errorf("fragment %s has %d lines, expected at least %d", tc.name, lineCount, tc.minLines)
			}
			for _, h := range tc.requireHeadings {
				if !strings.Contains(got, h) {
					t.Errorf("fragment %s missing required heading %q", tc.name, h)
				}
			}
		})
	}
}
