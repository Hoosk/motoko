package provider

import "testing"

func TestGetThinkingBudgetsUsesEffortPresets(t *testing.T) {
	info := ModelInfo{EffortPresets: []string{"low", "medium", "high", "max"}}
	got := GetThinkingBudgets(info)
	want := []int{0, 1024, 8192, 24576, 65536}
	if len(got) != len(want) {
		t.Fatalf("expected %d budgets, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected budgets %v, got %v", want, got)
		}
	}
}

func TestGetThinkingBudgetsUsesBudgetRange(t *testing.T) {
	info := ModelInfo{BudgetMin: 1024, BudgetMax: 32000}
	got := GetThinkingBudgets(info)
	want := []int{0, 1024, 8192, 24576, 32000}
	if len(got) != len(want) {
		t.Fatalf("expected %d budgets, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected budgets %v, got %v", want, got)
		}
	}
}

func TestGetThinkingLabelsUsesEffortPresets(t *testing.T) {
	info := ModelInfo{EffortPresets: []string{"low", "medium", "high"}}
	got := GetThinkingLabels(info)
	want := []string{"off", "low", "medium", "high"}
	if len(got) != len(want) {
		t.Fatalf("expected %d labels, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected labels %v, got %v", want, got)
		}
	}
}
