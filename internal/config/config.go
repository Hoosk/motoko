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

	ProviderPresetOpenAI     ProviderPreset = "openai"
	ProviderPresetOpenRouter ProviderPreset = "openrouter"
	ProviderPresetAnthropic  ProviderPreset = "anthropic"
	ProviderPresetGemini     ProviderPreset = "gemini"
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

type AppConfig struct {
	ActiveProvider string           `json:"active_provider"`
	Providers      []ProviderConfig `json:"providers"`
}

func Load() (*AppConfig, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AppConfig{}, nil
		}
		return nil, err
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
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
	data, err := json.MarshalIndent(c, "", "  ")
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
	switch normalizePreset(preset, kind) {
	case ProviderPresetOpenAI:
		return "https://api.openai.com/v1"
	case ProviderPresetOpenRouter:
		return "https://openrouter.ai/api/v1"
	case ProviderPresetAnthropic:
		return "https://api.anthropic.com"
	case ProviderPresetGemini:
		return "https://generativelanguage.googleapis.com/v1beta"
	default:
		switch normalizeKind(kind, preset) {
		case ProviderKindOpenAICompatible:
			return "https://api.openai.com/v1"
		case ProviderKindAnthropic:
			return "https://api.anthropic.com"
		case ProviderKindGemini:
			return "https://generativelanguage.googleapis.com/v1beta"
		default:
			return ""
		}
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
	return []ProviderPreset{ProviderPresetOpenAI, ProviderPresetOpenRouter, ProviderPresetAnthropic, ProviderPresetGemini}
}

func DefaultProviderName(preset ProviderPreset) string {
	switch preset {
	case ProviderPresetOpenAI:
		return "openai"
	case ProviderPresetOpenRouter:
		return "openrouter"
	case ProviderPresetAnthropic:
		return "anthropic"
	case ProviderPresetGemini:
		return "gemini"
	default:
		return ""
	}
}

func normalizePreset(preset ProviderPreset, kind ProviderKind) ProviderPreset {
	switch strings.TrimSpace(string(preset)) {
	case string(ProviderPresetOpenAI):
		return ProviderPresetOpenAI
	case string(ProviderPresetOpenRouter):
		return ProviderPresetOpenRouter
	case string(ProviderPresetAnthropic):
		return ProviderPresetAnthropic
	case string(ProviderPresetGemini):
		return ProviderPresetGemini
	}
	switch strings.TrimSpace(string(kind)) {
	case "openai", string(ProviderKindOpenAICompatible):
		return ProviderPresetOpenAI
	case string(ProviderPresetOpenRouter):
		return ProviderPresetOpenRouter
	case string(ProviderKindAnthropic):
		return ProviderPresetAnthropic
	case string(ProviderKindGemini):
		return ProviderPresetGemini
	default:
		return ""
	}
}

func normalizeKind(kind ProviderKind, preset ProviderPreset) ProviderKind {
	switch strings.TrimSpace(string(kind)) {
	case string(ProviderKindOpenAICompatible), "openai":
		return ProviderKindOpenAICompatible
	case string(ProviderKindAnthropic):
		return ProviderKindAnthropic
	case string(ProviderKindGemini):
		return ProviderKindGemini
	}
	switch normalizePreset(preset, kind) {
	case ProviderPresetOpenAI, ProviderPresetOpenRouter:
		return ProviderKindOpenAICompatible
	case ProviderPresetAnthropic:
		return ProviderKindAnthropic
	case ProviderPresetGemini:
		return ProviderKindGemini
	default:
		return ""
	}
}

func (c *AppConfig) sortProviders() {
	sort.Slice(c.Providers, func(i, j int) bool {
		return strings.ToLower(c.Providers[i].Name) < strings.ToLower(c.Providers[j].Name)
	})
}
