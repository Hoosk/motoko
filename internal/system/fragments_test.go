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
