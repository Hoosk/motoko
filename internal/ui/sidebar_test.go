package ui

import (
	"testing"
)

func TestContractPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		maxLength int
		want      string
	}{
		{
			name:      "fits fully",
			path:      "internal/ui/model.go",
			maxLength: 30,
			want:      "internal/ui/model.go",
		},
		{
			name:      "contracts first folder",
			path:      "internal/ui/model.go",
			maxLength: 18,
			want:      "i/ui/model.go",
		},
		{
			name:      "contracts multiple folders",
			path:      "internal/ui/model.go",
			maxLength: 15,
			want:      "i/ui/model.go",
		},
		{
			name:      "contracts multiple folders further",
			path:      "internal/ui/model.go",
			maxLength: 12,
			want:      "i/u/model.go",
		},
		{
			name:      "contracts deeply nested",
			path:      "cmd/motoko/internal/app/runtime.go",
			maxLength: 25,
			want:      "c/m/i/app/runtime.go",
		},
		{
			name:      "contracts fully and falls back to filename truncation",
			path:      "internal/ui/model.go",
			maxLength: 10,
			want:      "...odel.go",
		},
		{
			name:      "single file no contraction possible",
			path:      "model.go",
			maxLength: 5,
			want:      "...go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contractPath(tt.path, tt.maxLength)
			if got != tt.want {
				t.Errorf("contractPath(%q, %d) = %q, want %q", tt.path, tt.maxLength, got, tt.want)
			}
		})
	}
}
