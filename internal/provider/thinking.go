package provider

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

var defaultThinkingBudgets = []int{0, 1024, 8192, 24576, 65536}

// BudgetToReasoningEffort maps a token budget to an OpenAI reasoning_effort string.
// Thresholds align with ThinkingBudgetLevels: low=1024, medium=8192, high=24576, xhigh=65536.
func BudgetToReasoningEffort(budget int) string {
	switch {
	case budget >= 65536:
		return "xhigh"
	case budget >= 24576:
		return valHigh
	case budget >= 8192:
		return valMedium
	default:
		return valLow
	}
}

// BudgetToGeminiThinkingLevel maps a token budget to a Gemini 3 thinkingLevel string.
func BudgetToGeminiThinkingLevel(budget int) string {
	switch {
	case budget >= 24576:
		return valHigh
	case budget >= 8192:
		return valMedium
	default:
		return valLow
	}
}

// GetThinkingBudgets returns the thinking budget options for a model.
func GetThinkingBudgets(info ModelInfo) []int {
	if len(info.EffortPresets) > 0 {
		budgets := []int{0}
		for _, preset := range info.EffortPresets {
			switch strings.ToLower(strings.TrimSpace(preset)) {
			case valLow:
				budgets = appendUniqueInt(budgets, 1024)
			case valMedium:
				budgets = appendUniqueInt(budgets, 8192)
			case valHigh:
				budgets = appendUniqueInt(budgets, 24576)
			case "xhigh", "max":
				budgets = appendUniqueInt(budgets, 65536)
			}
		}
		if len(budgets) > 1 {
			return budgets
		}
	}
	if info.BudgetMax > 0 {
		budgets := []int{0}
		for _, budget := range defaultThinkingBudgets[1:] {
			if budget < info.BudgetMin || budget > info.BudgetMax {
				continue
			}
			budgets = append(budgets, budget)
		}
		if len(budgets) == 1 {
			budgets = append(budgets, info.BudgetMax)
		} else if budgets[len(budgets)-1] != info.BudgetMax {
			budgets = append(budgets, info.BudgetMax)
		}
		return budgets
	}
	return append([]int(nil), defaultThinkingBudgets...)
}

// GetThinkingLabels returns the list of thinking configuration labels for a model.
func GetThinkingLabels(info ModelInfo) []string {
	if len(info.EffortPresets) > 0 {
		labels := []string{"off"}
		for _, preset := range info.EffortPresets {
			labels = append(labels, strings.ToLower(strings.TrimSpace(preset)))
		}
		return labels
	}
	budgets := GetThinkingBudgets(info)
	labels := make([]string, 0, len(budgets))
	for _, budget := range budgets {
		labels = append(labels, thinkingBudgetLabel(budget, info.BudgetMax))
	}
	return labels
}

// BudgetToAnthropicEffort maps a token budget to an Anthropic effort level.
func BudgetToAnthropicEffort(budget int) anthropic.OutputConfigEffort {
	switch {
	case budget >= 65536:
		return anthropic.OutputConfigEffortXhigh
	case budget >= 24576:
		return anthropic.OutputConfigEffortHigh
	case budget >= 8192:
		return anthropic.OutputConfigEffortMedium
	default:
		return anthropic.OutputConfigEffortLow
	}
}

func appendUniqueInt(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func thinkingBudgetLabel(budget, maxBudget int) string {
	switch budget {
	case 0:
		return "off"
	case 1024:
		return "low (1k)"
	case 8192:
		return "medium (8k)"
	case 24576:
		return "high (24k)"
	case 65536:
		return "xhigh (64k)"
	}
	if maxBudget > 0 && budget == maxBudget {
		return fmt.Sprintf("max (%dk)", budget/1024)
	}
	return fmt.Sprintf("%dk", budget/1024)
}
