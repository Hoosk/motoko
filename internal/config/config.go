package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ProviderKind string
type ProviderPreset string

const (
	ProviderKindOpenAICompatible ProviderKind = "openai-compatible"
	ProviderKindAnthropic        ProviderKind = "anthropic"
	ProviderKindGemini           ProviderKind = "gemini"
	ProviderKindLMStudio         ProviderKind = "lmstudio"

	ProviderPresetOpenAI           ProviderPreset = "openai"
	ProviderPresetOpenRouter       ProviderPreset = "openrouter"
	ProviderPresetAnthropic        ProviderPreset = "anthropic"
	ProviderPresetGemini           ProviderPreset = "gemini"
	ProviderPresetOpenAICompatible ProviderPreset = "openai-compatible"
	ProviderPresetLMStudio         ProviderPreset = "lmstudio"
)

type ProviderConfig struct {
	Name                string         `json:"name"`
	Preset              ProviderPreset `json:"preset,omitempty"`
	Kind                ProviderKind   `json:"kind"`
	BaseURL             string         `json:"base_url"`
	APIKey              string         `json:"api_key"`
	Model               string         `json:"model"`
	Models              []string       `json:"models,omitempty"`
	ContextWindow       int            `json:"context_window,omitempty"`
	ThinkingBudget      int            `json:"thinking_budget,omitempty"`
	UseSDK              bool           `json:"use_sdk,omitempty"`
	EnableGoogleSearch  bool           `json:"enable_google_search,omitempty"`
	EnableCodeExecution bool           `json:"enable_code_execution,omitempty"`
	SupportsThinking    bool           `json:"supports_thinking,omitempty"`
}

type SearchConfig struct {
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`
	MaxResults      int      `json:"max_results,omitempty"`
	CaseSensitive   bool     `json:"case_sensitive,omitempty"`
}

type AgentOverride struct {
	Model          string   `json:"model,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Temperature    *float64 `json:"temperature,omitempty"`
	ThinkingBudget *int     `json:"thinking_budget,omitempty"`
	MaxIterations  *int     `json:"max_iterations,omitempty"`
	SystemPrompt   string   `json:"system_prompt,omitempty"`
	ToolFilter     []string `json:"tool_filter,omitempty"`
	ExcludeTools   []string `json:"exclude_tools,omitempty"`
	Disabled       bool     `json:"disabled,omitempty"`
}

type AppConfig struct {
	Agents         map[string]AgentOverride `json:"agents,omitempty"`
	ActiveProvider string                   `json:"active_provider"`
	Theme          string                   `json:"theme,omitempty"`
	Density        string                   `json:"density,omitempty"`
	Providers      []ProviderConfig         `json:"providers"`
	Search         SearchConfig             `json:"search,omitempty"`
}

func (c *AppConfig) Merge(other *AppConfig) {
	if other == nil {
		return
	}
	if other.ActiveProvider != "" {
		c.ActiveProvider = other.ActiveProvider
	}
	if other.Theme != "" {
		c.Theme = other.Theme
	}
	if other.Density != "" {
		c.Density = other.Density
	}
	for _, op := range other.Providers {
		op = NormalizeProvider(op)
		found := false
		for i, p := range c.Providers {
			if strings.EqualFold(p.Name, op.Name) {
				c.Providers[i] = op
				found = true
				break
			}
		}
		if !found {
			c.Providers = append(c.Providers, op)
		}
	}
	if len(other.Search.ExcludePatterns) > 0 {
		c.Search.ExcludePatterns = UniqueSortedKeep(c.Search.ExcludePatterns, other.Search.ExcludePatterns...)
	}
	if other.Search.MaxResults > 0 {
		c.Search.MaxResults = other.Search.MaxResults
	}
	if other.Search.CaseSensitive {
		c.Search.CaseSensitive = true
	}
	if c.Agents == nil {
		c.Agents = make(map[string]AgentOverride)
	}
	for name, override := range other.Agents {
		existing := c.Agents[name]
		if override.Model != "" {
			existing.Model = override.Model
		}
		if override.Provider != "" {
			existing.Provider = override.Provider
		}
		if override.Temperature != nil {
			existing.Temperature = override.Temperature
		}
		if override.ThinkingBudget != nil {
			existing.ThinkingBudget = override.ThinkingBudget
		}
		if override.MaxIterations != nil {
			existing.MaxIterations = override.MaxIterations
		}
		if override.SystemPrompt != "" {
			existing.SystemPrompt = override.SystemPrompt
		}
		if len(override.ToolFilter) > 0 {
			existing.ToolFilter = UniqueSortedKeep(existing.ToolFilter, override.ToolFilter...)
		}
		if len(override.ExcludeTools) > 0 {
			existing.ExcludeTools = UniqueSortedKeep(existing.ExcludeTools, override.ExcludeTools...)
		}
		if override.Disabled {
			existing.Disabled = true
		}
		c.Agents[name] = existing
	}
}

func Load(workspacePath ...string) (*AppConfig, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	var cfg AppConfig
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cfg.Search.MaxResults = 100
		cfg.Search.ExcludePatterns = []string{".git", "node_modules", "vendor", "dist", "tmp"}
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to decode config: %w", err)
		}
		// Decrypt API keys
		for i, p := range cfg.Providers {
			if strings.HasPrefix(p.APIKey, "enc:") {
				decKey, decErr := Decrypt(p.APIKey)
				if decErr == nil {
					cfg.Providers[i].APIKey = decKey
				}
			}
		}
	}

	// Load project-scoped config if exists
	if len(workspacePath) > 0 && workspacePath[0] != "" {
		localPath := filepath.Join(workspacePath[0], ".agents", "config.json")
		if localData, err := os.ReadFile(localPath); err == nil {
			var localCfg AppConfig
			if err := json.Unmarshal(localData, &localCfg); err == nil {
				cfg.Merge(&localCfg)
			}
		}
	}

	return &cfg, nil
}

func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "motoko", "config.json"), nil
}

func (c *AppConfig) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		return mkdirErr
	}
	c.sortProviders()

	// Create a copy of config with encrypted API keys
	var encryptedCfg AppConfig
	encryptedCfg.ActiveProvider = c.ActiveProvider
	encryptedCfg.Search = c.Search
	encryptedCfg.Agents = c.Agents
	encryptedCfg.Theme = c.Theme
	encryptedCfg.Density = c.Density
	encryptedCfg.Providers = make([]ProviderConfig, len(c.Providers))
	for i, p := range c.Providers {
		encryptedCfg.Providers[i] = p
		if p.APIKey != "" && !strings.HasPrefix(p.APIKey, "enc:") {
			encKey, encErr := Encrypt(p.APIKey)
			if encErr != nil {
				return encErr
			}
			encryptedCfg.Providers[i].APIKey = encKey
		}
	}

	data, err := json.MarshalIndent(encryptedCfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c *AppConfig) UpsertProvider(provider ProviderConfig) {
	provider = NormalizeProvider(provider)

	for i, existing := range c.Providers {
		if strings.EqualFold(existing.Name, provider.Name) {
			c.Providers[i] = provider
			c.sortProviders()
			return
		}
	}

	c.Providers = append(c.Providers, provider)
	c.sortProviders()
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
