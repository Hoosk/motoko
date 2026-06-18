package system

import (
	"testing"
)

//nolint:dupl
func TestMaxToolOutputBytes(t *testing.T) {
	tests := []struct {
		name          string
		contextWindow int
		want          int
	}{
		{"unknown window defaults to 12000", 0, 12000},
		{"negative window defaults to 12000", -1, 12000},
		{"small window hits floor", 2000, 1000}, // 2000 * 4 * 0.02 = 160 -> 1000
		{"standard 8k window", 8192, 1000}, // 8192 * 4 * 0.02 = 655 -> 1000
		{"medium window", 32768, 2621}, // 32768 * 4 * 0.02 = 2621.44 -> 2621
		{"large window", 128000, 10240}, // 128000 * 4 * 0.02 = 10240
		{"huge window", 1048576, 83886}, // 1048576 * 4 * 0.02 = 83886
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaxToolOutputBytes(tt.contextWindow); got != tt.want {
				t.Errorf("MaxToolOutputBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

//nolint:dupl
func TestPreserveHistoryTokens(t *testing.T) {
	tests := []struct {
		name          string
		contextWindow int
		want          int
	}{
		{"unknown window defaults to 8000", 0, 8000},
		{"negative window defaults to 8000", -1, 8000},
		{"small window hits floor", 2048, 1000}, // 204.8 -> 1000
		{"standard 8k hits floor", 8192, 1000}, // 819.2 -> 1000
		{"medium window", 32768, 3276}, // 3276.8 -> 3276
		{"large window hits cap", 128000, 8000}, // 12800 -> 8000
		{"huge window hits cap", 1048576, 8000}, // 104857 -> 8000
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PreserveHistoryTokens(tt.contextWindow); got != tt.want {
				t.Errorf("PreserveHistoryTokens() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSemanticLimits(t *testing.T) {
	tests := []struct {
		name          string
		contextWindow int
		wantFiles     int
		wantSnippets  int
	}{
		{"unknown", 0, 5, 10},
		{"negative", -100, 5, 10},
		{"small", 4096, 3, 5},
		{"medium", 10000, 5, 10},
		{"large", 32000, 10, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSemanticLimits(tt.contextWindow)
			if got.NumFiles != tt.wantFiles {
				t.Errorf("NumFiles = %v, want %v", got.NumFiles, tt.wantFiles)
			}
			if got.NumSnippets != tt.wantSnippets {
				t.Errorf("NumSnippets = %v, want %v", got.NumSnippets, tt.wantSnippets)
			}
		})
	}
}
