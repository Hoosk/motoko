package provider

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// budgetToReasoningEffort maps a token budget to an OpenAI reasoning_effort string.
// Thresholds align with ThinkingBudgetLevels: low=1024, medium=8192, high=24576, xhigh=65536.
func budgetToReasoningEffort(budget int) string {
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

// budgetToGeminiThinkingLevel maps a token budget to a Gemini 3 thinkingLevel string.
func budgetToGeminiThinkingLevel(budget int) string {
	switch {
	case budget >= 24576:
		return valHigh
	case budget >= 8192:
		return valMedium
	default:
		return valLow
	}
}

// GetThinkingLabels returns the list of thinking configuration labels for a model.
func GetThinkingLabels(modelID string) []string {
	return []string{"off", valLow, valMedium, valHigh, "xhigh"}
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
