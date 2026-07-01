package ui

import (
	"testing"

	"github.com/Hoosk/motoko/internal/provider"
)

func TestThinkingPickerOpenUsesCurrentBudgetIndex(t *testing.T) {
	var picker thinkingPickerState
	picker.Open(provider.ModelInfo{ID: "deepseek-v4-flash", EffortPresets: []string{"low", "medium", "high", "max"}}, 24576)
	if picker.thinkingIndex != 3 {
		t.Fatalf("expected thinking index 3, got %d", picker.thinkingIndex)
	}
}

func TestThinkingBudgetIndexFallsBackToClosestLowerBudget(t *testing.T) {
	got := thinkingBudgetIndex([]int{0, 1024, 8192, 24576}, 10000)
	if got != 2 {
		t.Fatalf("expected closest lower budget index 2, got %d", got)
	}
}
