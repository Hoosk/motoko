package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	EffortPresets       []string       `json:"effort_presets,omitempty"`
	ThinkingBudget      int            `json:"thinking_budget,omitempty"`
	ContextWindow       int            `json:"context_window,omitempty"`
	BudgetMin           int            `json:"budget_min,omitempty"`
	BudgetMax           int            `json:"budget_max,omitempty"`
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
	Agents            map[string]AgentOverride `json:"agents,omitempty"`
	Providers         []ProviderConfig         `json:"providers"`
	MCPServers        []MCPServerConfig        `json:"mcp_servers,omitempty"`
	ActiveProvider    string                   `json:"active_provider"`
	Theme             string                   `json:"theme,omitempty"`
	Density           string                   `json:"density,omitempty"`
	ThinkingVerbosity string                   `json:"thinking_verbosity,omitempty"`
	Search            SearchConfig             `json:"search"`
	MaxIterations     int                      `json:"max_iterations,omitempty"`
}

// MCPServerConfig describes a single MCP server. Both stdio and HTTP
// transports are accepted.
type MCPServerConfig struct {
	Env       map[string]string `json:"env,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Name      string            `json:"name"`
	Transport string            `json:"transport,omitempty"`
	Command   string            `json:"command,omitempty"`
	URL       string            `json:"url,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Disabled  bool              `json:"disabled,omitempty"`
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
	if other.ThinkingVerbosity != "" {
		c.ThinkingVerbosity = other.ThinkingVerbosity
	}
	if other.MaxIterations > 0 {
		c.MaxIterations = other.MaxIterations
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
	if len(other.MCPServers) > 0 {
		c.MCPServers = mergeMCPServers(c.MCPServers, other.MCPServers)
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
	if cfg.Search.MaxResults <= 0 {
		cfg.Search.MaxResults = 100
	}
	if len(cfg.Search.ExcludePatterns) == 0 {
		cfg.Search.ExcludePatterns = []string{".git", "node_modules", "vendor", "dist", "tmp"}
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

		// Dedicated MCP file (`.agents/mcp.json`) merged on top.
		mcpPath := filepath.Join(workspacePath[0], ".agents", "mcp.json")
		if extra, err := LoadMCPFile(mcpPath); err == nil && len(extra) > 0 {
			cfg.MCPServers = mergeMCPServers(cfg.MCPServers, extra)
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
	encryptedCfg.ThinkingVerbosity = c.ThinkingVerbosity
	encryptedCfg.MaxIterations = c.MaxIterations
	encryptedCfg.MCPServers = c.MCPServers
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
