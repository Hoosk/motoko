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

	ProviderPresetOpenAI           ProviderPreset = "openai"
	ProviderPresetOpenRouter       ProviderPreset = "openrouter"
	ProviderPresetAnthropic        ProviderPreset = "anthropic"
	ProviderPresetGemini           ProviderPreset = "gemini"
	ProviderPresetOpenAICompatible ProviderPreset = "openai-compatible"
)

type ProviderConfig struct {
	Name           string         `json:"name"`
	Preset         ProviderPreset `json:"preset,omitempty"`
	Kind           ProviderKind   `json:"kind"`
	BaseURL        string         `json:"base_url"`
	APIKey         string         `json:"api_key"`
	Model          string         `json:"model"`
	Models         []string       `json:"models,omitempty"`
	ContextWindow  int            `json:"context_window,omitempty"`
	ThinkingBudget int            `json:"thinking_budget,omitempty"`
}

type SearchConfig struct {
	MaxResults      int      `json:"max_results,omitempty"`
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`
	CaseSensitive   bool     `json:"case_sensitive,omitempty"`
}

type AppConfig struct {
	ActiveProvider string           `json:"active_provider"`
	Providers      []ProviderConfig `json:"providers"`
	Search         SearchConfig     `json:"search,omitempty"`
}

func Load() (*AppConfig, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := &AppConfig{}
			cfg.Search.MaxResults = 100
			cfg.Search.ExcludePatterns = []string{".git", "node_modules", "vendor", "dist", "tmp"}
			return cfg, nil
		}
		return nil, err
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Search.MaxResults <= 0 {
		cfg.Search.MaxResults = 100
	}
	if len(cfg.Search.ExcludePatterns) == 0 {
		cfg.Search.ExcludePatterns = []string{".git", "node_modules", "vendor", "dist", "tmp"}
	}
	for i, p := range cfg.Providers {
		if strings.HasPrefix(p.APIKey, "enc:") {
			dec, err := Decrypt(p.APIKey)
			if err != nil {
				return nil, fmt.Errorf("error al descifrar API key para %s: %w", p.Name, err)
			}
			cfg.Providers[i].APIKey = dec
		}
	}
	for i := range cfg.Providers {
		cfg.Providers[i] = NormalizeProvider(cfg.Providers[i])
	}
	cfg.sortProviders()
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	c.sortProviders()

	// Create a copy of config with encrypted API keys
	var encryptedCfg AppConfig
	encryptedCfg.ActiveProvider = c.ActiveProvider
	encryptedCfg.Search = c.Search
	encryptedCfg.Providers = make([]ProviderConfig, len(c.Providers))
	for i, p := range c.Providers {
		encryptedCfg.Providers[i] = p
		if p.APIKey != "" && !strings.HasPrefix(p.APIKey, "enc:") {
			encKey, err := Encrypt(p.APIKey)
			if err != nil {
				return err
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
	if provider.Name == "" {
		provider.Name = DefaultProviderName(provider.Preset)
		if provider.Name == "" {
			provider.Name = "provider"
		}
	}
	if provider.BaseURL == "" {
		provider.BaseURL = DefaultBaseURL(provider.Preset, provider.Kind)
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
		return "https://generativelanguage.googleapis.com/v1beta/openai/"
	case ProviderPresetOpenAICompatible:
		return "http://localhost:11434/v1"
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
	return []ProviderPreset{ProviderPresetOpenAI, ProviderPresetOpenRouter, ProviderPresetAnthropic, ProviderPresetGemini, ProviderPresetOpenAICompatible}
}

func DefaultProviderName(preset ProviderPreset) string {
	return string(preset)
}

func normalizePreset(preset ProviderPreset, kind ProviderKind) ProviderPreset {
	p := ProviderPreset(strings.ToLower(strings.TrimSpace(string(preset))))
	k := ProviderKind(strings.ToLower(strings.TrimSpace(string(kind))))

	// 1. Exact preset match
	switch p {
	case ProviderPresetOpenAI, ProviderPresetOpenRouter, ProviderPresetAnthropic, ProviderPresetGemini, ProviderPresetOpenAICompatible:
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
	}

	// 2. Kind from normalized preset
	switch normalizePreset(preset, kind) {
	case ProviderPresetOpenAI, ProviderPresetOpenRouter, ProviderPresetOpenAICompatible:
		return ProviderKindOpenAICompatible
	case ProviderPresetAnthropic:
		return ProviderKindAnthropic
	case ProviderPresetGemini:
		return ProviderKindGemini
	}

	return ""
}

func (c *AppConfig) sortProviders() {
	sort.Slice(c.Providers, func(i, j int) bool {
		return strings.ToLower(c.Providers[i].Name) < strings.ToLower(c.Providers[j].Name)
	})
}
