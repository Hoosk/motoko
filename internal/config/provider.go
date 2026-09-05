package config

import (
	"fmt"
	"sort"
	"strings"
)

func (c *AppConfig) UpsertProvider(provider ProviderConfig) {
	provider = NormalizeProvider(provider)

	for i, existing := range c.Providers {
		if strings.EqualFold(existing.Name, provider.Name) {
			c.Providers[i] = mergeProvider(existing, provider)
			c.sortProviders()
			return
		}
	}

	c.Providers = append(c.Providers, provider)
	c.sortProviders()
}

// mergeProvider returns next with any zero-value metadata fields filled in
// from prev. This prevents UI forms that only carry Name/Preset/BaseURL/APIKey
// from inadvertently wiping the model selection and capability metadata that
// was set by /models use or SetThinkingBudget.
//
// Fields that are always overwritten (identity + credentials + connectivity):
//
//	Name, Preset, Kind, BaseURL, APIKey, UseSDK,
//	EnableGoogleSearch, EnableCodeExecution
//
// Fields preserved from prev when next is zero:
//
//	Model, Models, ContextWindow, SupportsThinking,
//	EffortPresets, BudgetMin, BudgetMax, ThinkingBudget
func mergeProvider(prev, next ProviderConfig) ProviderConfig {
	if strings.TrimSpace(next.Model) == "" {
		next.Model = prev.Model
	}
	if len(next.Models) == 0 {
		next.Models = append([]string(nil), prev.Models...)
	}
	if next.ContextWindow == 0 {
		next.ContextWindow = prev.ContextWindow
	}
	if !next.SupportsThinking {
		next.SupportsThinking = prev.SupportsThinking
	}
	if len(next.EffortPresets) == 0 {
		next.EffortPresets = append([]string(nil), prev.EffortPresets...)
	}
	if next.BudgetMin == 0 {
		next.BudgetMin = prev.BudgetMin
	}
	if next.BudgetMax == 0 {
		next.BudgetMax = prev.BudgetMax
	}
	if next.ThinkingBudget == 0 {
		next.ThinkingBudget = prev.ThinkingBudget
	}
	return next
}

func (c *AppConfig) RemoveProvider(name string) bool {
	for i, provider := range c.Providers {
		if strings.EqualFold(provider.Name, name) {
			c.Providers = append(c.Providers[:i], c.Providers[i+1:]...)
			if strings.EqualFold(c.ActiveProvider, name) {
				c.ActiveProvider = ""
			}
			return true
		}
	}
	return false
}

func (c *AppConfig) Provider(name string) (ProviderConfig, bool) {
	for _, provider := range c.Providers {
		if strings.EqualFold(provider.Name, name) {
			return provider, true
		}
	}
	return ProviderConfig{}, false
}

func (c *AppConfig) Active() (ProviderConfig, bool) {
	if strings.TrimSpace(c.ActiveProvider) == "" {
		return ProviderConfig{}, false
	}
	return c.Provider(c.ActiveProvider)
}

func (c *AppConfig) SetActive(name string) error {
	provider, ok := c.Provider(name)
	if !ok {
		return fmt.Errorf("provider no encontrado: %s", name)
	}
	c.ActiveProvider = provider.Name
	return nil
}

func NormalizeProvider(provider ProviderConfig) ProviderConfig {
	provider.Name = strings.TrimSpace(provider.Name)
	provider.Preset = normalizePreset(provider.Preset, provider.Kind)
	provider.Kind = normalizeKind(provider.Kind, provider.Preset)
	provider.BaseURL = strings.TrimSpace(provider.BaseURL)
	provider.APIKey = strings.TrimSpace(provider.APIKey)
	provider.Model = strings.TrimSpace(provider.Model)
	provider.Models = uniqueSorted(provider.Models)

	// If the preset or base URL targets Google's Gemini API endpoints, we force it to the native Gemini provider preset and kind.
	// Gemini is officially supported by Google GenAI Go SDK and should always run natively.
	isGemini := provider.Preset == ProviderPresetGemini ||
		strings.Contains(strings.ToLower(provider.BaseURL), "generativelanguage.googleapis.com") ||
		strings.Contains(strings.ToLower(provider.BaseURL), "googleapis.com")

	if isGemini {
		provider.Preset = ProviderPresetGemini
		provider.Kind = ProviderKindGemini
	}

	isLMStudio := provider.Preset == ProviderPresetLMStudio ||
		provider.Kind == ProviderKindLMStudio ||
		strings.Contains(provider.BaseURL, ":1234")

	if isLMStudio {
		provider.Preset = ProviderPresetLMStudio
		provider.Kind = ProviderKindLMStudio
	}

	if provider.ContextWindow <= 0 {
		if provider.Preset == ProviderPresetLMStudio || provider.Preset == ProviderPresetOpenAICompatible {
			provider.ContextWindow = 8192
		}
	}

	if provider.APIKey == "" && (provider.Preset == ProviderPresetOpenAICompatible || provider.Preset == ProviderPresetLMStudio) {
		provider.APIKey = "lm-studio"
	}

	if provider.Name == "" {
		provider.Name = DefaultProviderName(provider.Preset)
		if provider.Name == "" {
			provider.Name = "provider"
		}
	}
	if provider.BaseURL == "" {
		provider.BaseURL = DefaultBaseURL(provider.Preset, provider.Kind)
	}

	if (provider.Preset == ProviderPresetOpenAICompatible || provider.Preset == ProviderPresetLMStudio) && provider.BaseURL != "" {
		trimmedURL := strings.TrimRight(provider.BaseURL, "/")
		if !strings.HasSuffix(trimmedURL, "/v1") {
			provider.BaseURL = trimmedURL + "/v1"
		}
	}
	return provider
}

func DefaultBaseURL(preset ProviderPreset, kind ProviderKind) string {
	switch preset {
	case ProviderPresetOpenAI:
		return "https://api.openai.com/v1"
	case ProviderPresetOpenRouter:
		return "https://openrouter.ai/api/v1"
	case ProviderPresetAnthropic:
		return "https://api.anthropic.com"
	case ProviderPresetGemini:
		return ""
	case ProviderPresetOpenAICompatible:
		return "http://localhost:11434/v1"
	case ProviderPresetLMStudio:
		return "http://127.0.0.1:1234/v1"
	default:
		return ""
	}
}

func uniqueSorted(items []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func UniqueSortedKeep(items []string, extra ...string) []string {
	all := append(append([]string{}, items...), extra...)
	return uniqueSorted(all)
}

func ValidProviderPresets() []ProviderPreset {
	return []ProviderPreset{ProviderPresetOpenAI, ProviderPresetOpenRouter, ProviderPresetAnthropic, ProviderPresetGemini, ProviderPresetOpenAICompatible, ProviderPresetLMStudio}
}

func DefaultProviderName(preset ProviderPreset) string {
	return string(preset)
}

func normalizePreset(preset ProviderPreset, kind ProviderKind) ProviderPreset {
	p := ProviderPreset(strings.ToLower(strings.TrimSpace(string(preset))))
	k := ProviderKind(strings.ToLower(strings.TrimSpace(string(kind))))

	// 1. Exact preset match
	switch p {
	case ProviderPresetOpenAI, ProviderPresetOpenRouter, ProviderPresetAnthropic, ProviderPresetGemini, ProviderPresetOpenAICompatible, ProviderPresetLMStudio:
		return p
	}

	// 2. Preset from kind
	switch k {
	case "openai", ProviderKindOpenAICompatible:
		return ProviderPresetOpenAI
	case ProviderKindAnthropic:
		return ProviderPresetAnthropic
	case ProviderKindGemini:
		return ProviderPresetGemini
	case ProviderKindLMStudio:
		return ProviderPresetLMStudio
	}

	if p != "" {
		return p
	}

	return ""
}

func normalizeKind(kind ProviderKind, preset ProviderPreset) ProviderKind {
	k := ProviderKind(strings.ToLower(strings.TrimSpace(string(kind))))

	// 1. Exact kind match
	switch k {
	case ProviderKindOpenAICompatible, "openai":
		return ProviderKindOpenAICompatible
	case ProviderKindAnthropic:
		return ProviderKindAnthropic
	case ProviderKindGemini:
		return ProviderKindGemini
	case ProviderKindLMStudio:
		return ProviderKindLMStudio
	}

	// 2. Kind from normalized preset
	normalizedPreset := normalizePreset(preset, kind)
	switch normalizedPreset {
	case ProviderPresetOpenAI, ProviderPresetOpenRouter, ProviderPresetOpenAICompatible:
		return ProviderKindOpenAICompatible
	case ProviderPresetAnthropic:
		return ProviderKindAnthropic
	case ProviderPresetGemini:
		return ProviderKindGemini
	case ProviderPresetLMStudio:
		return ProviderKindLMStudio
	}

	if normalizedPreset != "" {
		return ProviderKindOpenAICompatible
	}

	return ""
}

func (c *AppConfig) sortProviders() {
	sort.Slice(c.Providers, func(i, j int) bool {
		return strings.ToLower(c.Providers[i].Name) < strings.ToLower(c.Providers[j].Name)
	})
}
