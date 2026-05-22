package provider

import "strings"

// isOpenAIReasoningModel reports whether the model name is a reasoning model
// that supports reasoning_effort. This includes the legacy o-series (o1, o3, o4)
// and the current gpt-5.x reasoning models (gpt-5.5, gpt-5.4, gpt-5.5-pro, etc.).
func isOpenAIReasoningModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		strings.HasPrefix(lower, "gpt-5")
}

// budgetToReasoningEffort maps a token budget to an OpenAI reasoning_effort string.
// Thresholds align with ThinkingBudgetLevels: low=1024, medium=8192, high=24576, xhigh=65536.
func budgetToReasoningEffort(budget int) string {
	switch {
	case budget >= 65536:
		return "xhigh"
	case budget >= 24576:
		return "high"
	case budget >= 8192:
		return "medium"
	default:
		return "low"
	}
}

// isAnthropicAdaptiveThinkingModel reports whether the model uses the newer
// adaptive thinking API ({type:"adaptive"}) instead of the manual budget API.
// claude-opus-4-7 and newer generation models require adaptive thinking.
func isAnthropicAdaptiveThinkingModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "opus-4-7")
}

// isGemini3Model reports whether the model belongs to the Gemini 3.x series,
// which uses thinkingLevel instead of thinkingBudget.
func isGemini3Model(model string) bool {
	lower := strings.ToLower(model)
	return strings.HasPrefix(lower, "gemini-3")
}

// budgetToGeminiThinkingLevel maps a token budget to a Gemini 3 thinkingLevel string.
func budgetToGeminiThinkingLevel(budget int) string {
	switch {
	case budget >= 24576:
		return "high"
	case budget >= 8192:
		return "medium"
	default:
		return "low"
	}
}
