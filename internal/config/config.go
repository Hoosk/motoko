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

const (
	ProviderOpenAI    ProviderKind = "openai"
	ProviderAnthropic ProviderKind = "anthropic"
	ProviderGemini    ProviderKind = "gemini"
)

type ProviderConfig struct {
	Name    string       `json:"name"`
	Kind    ProviderKind `json:"kind"`
	BaseURL string       `json:"base_url"`
	APIKey  string       `json:"api_key"`
	Model   string       `json:"model"`
	Models  []string     `json:"models,omitempty"`
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
	provider.Name = strings.TrimSpace(provider.Name)
	provider.BaseURL = strings.TrimSpace(provider.BaseURL)
	provider.APIKey = strings.TrimSpace(provider.APIKey)
	provider.Model = strings.TrimSpace(provider.Model)
	provider.Models = uniqueSorted(provider.Models)

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

func DefaultBaseURL(kind ProviderKind) string {
	switch kind {
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderAnthropic:
		return "https://api.anthropic.com"
	case ProviderGemini:
		return "https://generativelanguage.googleapis.com/v1beta"
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

func ValidProviderKinds() []ProviderKind {
	return []ProviderKind{ProviderOpenAI, ProviderAnthropic, ProviderGemini}
}

func (c *AppConfig) sortProviders() {
	sort.Slice(c.Providers, func(i, j int) bool {
		return strings.ToLower(c.Providers[i].Name) < strings.ToLower(c.Providers[j].Name)
	})
}
