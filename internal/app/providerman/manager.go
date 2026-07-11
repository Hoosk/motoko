package providerman

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"

	"github.com/Hoosk/motoko/internal/app/types"
)

type Manager struct {
	cfgFn            func() *config.AppConfig
	providerClientFn func() func(config.ProviderConfig) (provider.Client, error)
	onRefresh        func()
}

func NewManager(cfgFn func() *config.AppConfig, providerFn func() func(config.ProviderConfig) (provider.Client, error), onRefresh func()) *Manager {
	return &Manager{
		cfgFn:            cfgFn,
		providerClientFn: providerFn,
		onRefresh:        onRefresh,
	}
}

func (m *Manager) Config() *config.AppConfig { return m.cfgFn() }

func (m *Manager) ProviderSummary() string {
	cfg := m.cfgFn()
	if cfg == nil {
		return "none"
	}
	active, ok := cfg.Active()
	if !ok {
		return "none"
	}
	if strings.TrimSpace(active.Model) == "" {
		return fmt.Sprintf("%s (%s:no-model)", active.Name, active.Preset)
	}
	return fmt.Sprintf("%s (%s:%s)", active.Name, active.Preset, active.Model)
}

func (m *Manager) ProviderPresets() []config.ProviderPreset {
	presets := config.ValidProviderPresets()
	seen := make(map[config.ProviderPreset]bool)
	for _, p := range presets {
		seen[p] = true
	}

	for _, cp := range provider.ListCatalogProviders() {
		preset := config.ProviderPreset(cp)
		if !seen[preset] {
			presets = append(presets, preset)
			seen[preset] = true
		}
	}
	return presets
}

func (m *Manager) LookupCatalogProvider(id string) (provider.CatalogProvider, bool) {
	return provider.LookupProvider(id)
}

func (m *Manager) GetActiveProviderConfig() (config.ProviderConfig, bool) {
	cfg := m.cfgFn()
	if cfg == nil {
		return config.ProviderConfig{}, false
	}
	return cfg.Active()
}

func (m *Manager) SetActiveModelInfo(model provider.ModelInfo) error {
	cfg := m.cfgFn()
	if cfg == nil {
		return fmt.Errorf("no configuration")
	}
	active, ok := cfg.Active()
	if !ok {
		return fmt.Errorf("no active provider")
	}
	active.Model = model.ID
	active.Models = config.UniqueSortedKeep(active.Models, model.ID)
	active.ContextWindow = model.ContextWindow
	active.SupportsThinking = model.SupportsThinking
	active.EffortPresets = append([]string(nil), model.EffortPresets...)
	active.BudgetMin = model.BudgetMin
	active.BudgetMax = model.BudgetMax
	cfg.UpsertProvider(active)
	if err := cfg.Save(); err != nil {
		return err
	}
	if m.onRefresh != nil {
		m.onRefresh()
	}
	return nil
}

func (m *Manager) GetModelInfoForActiveProvider(ctx context.Context, modelID string) (provider.ModelInfo, error) {
	cfg := m.cfgFn()
	active, ok := cfg.Active()
	if !ok {
		return provider.ModelInfo{}, fmt.Errorf("no active provider")
	}
	active.Model = ""
	client, err := m.ProviderClient(active)
	if err != nil {
		return provider.ModelInfo{}, err
	}
	info, err := client.GetModel(ctx, modelID)
	if err != nil {
		return provider.ModelInfo{}, err
	}
	return provider.EnrichModelInfo(active.Name, info), nil
}

func (m *Manager) SetThinkingBudget(budget int) error {
	cfg := m.cfgFn()
	if cfg == nil {
		return fmt.Errorf("no configuration")
	}
	active, ok := cfg.Active()
	if !ok {
		return fmt.Errorf("no active provider")
	}
	if active.ContextWindow > 0 {
		maxAllowed := active.ContextWindow / 2
		if budget > maxAllowed {
			budget = maxAllowed
		}
	}
	active.ThinkingBudget = budget
	cfg.UpsertProvider(active)
	if err := cfg.Save(); err != nil {
		return err
	}
	if m.onRefresh != nil {
		m.onRefresh()
	}
	return nil
}

func (m *Manager) ListModelsForProvider(ctx context.Context, providerCfg config.ProviderConfig) ([]provider.ModelInfo, error) {
	providerCfg.Model = ""
	client, err := m.ProviderClient(providerCfg)
	if err != nil {
		return nil, err
	}
	models, err := client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	for i := range models {
		models[i] = provider.EnrichModelInfo(providerCfg.Name, models[i])
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	m.CacheProviderModels(providerCfg.Name, ids)
	return models, nil
}

func (m *Manager) SaveProvider(providerCfg config.ProviderConfig, activate bool) error {
	cfg := m.cfgFn()
	if cfg == nil {
		cfg = &config.AppConfig{}
	}
	providerCfg = config.NormalizeProvider(providerCfg)
	cfg.UpsertProvider(providerCfg)
	if activate || strings.TrimSpace(cfg.ActiveProvider) == "" {
		if err := cfg.SetActive(providerCfg.Name); err != nil {
			return err
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	if m.onRefresh != nil {
		m.onRefresh()
	}
	return nil
}

func (m *Manager) ProviderClient(cfg config.ProviderConfig) (provider.Client, error) {
	providerFn := m.providerClientFn()
	if providerFn != nil {
		return providerFn(cfg)
	}
	return provider.NewClient(cfg)
}

func (m *Manager) CacheProviderModels(providerName string, models []string) {
	cfg := m.cfgFn()
	if cfg == nil || strings.TrimSpace(providerName) == "" || len(models) == 0 {
		return
	}
	providerCfg, ok := cfg.Provider(providerName)
	if !ok {
		return
	}
	providerCfg.Models = config.UniqueSortedKeep(providerCfg.Models, models...)
	cfg.UpsertProvider(providerCfg)
	_ = cfg.Save()
}

func (m *Manager) ProviderListText() string {
	cfg := m.cfgFn()
	active, ok := cfg.Active()
	activeName := ""
	if ok {
		activeName = active.Name
	}
	providers := cfg.Providers
	var lines []string
	for _, providerCfg := range providers {
		marker := " "
		if strings.EqualFold(providerCfg.Name, activeName) {
			marker = "*"
		}
		model := providerCfg.Model
		if strings.TrimSpace(model) == "" {
			model = "no-model"
		}
		label := string(providerCfg.Preset)
		if strings.TrimSpace(label) == "" {
			label = string(providerCfg.Kind)
		}
		lines = append(lines, fmt.Sprintf("%s %s [%s] %s", marker, providerCfg.Name, label, model))
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) HandleProviderCommand(args []string) types.Response {
	if len(args) == 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join([]string{
			"Provider usage:",
			"/provider list",
			"/provider add",
			"/provider use <name>",
			"/provider remove <name>",
		}, "\n")}}}
	}

	subcommand := strings.ToLower(args[0])
	switch subcommand {
	case "list":
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: m.ProviderListText()}}}
	case "add":
		if len(args) >= 5 {
			name := args[1]
			preset := config.ProviderPreset(args[2])
			baseURL := args[3]
			apiKey := args[4]
			newProv := config.ProviderConfig{
				Name:    name,
				Preset:  preset,
				BaseURL: baseURL,
				APIKey:  apiKey,
			}
			newProv = config.NormalizeProvider(newProv)
			cfg := m.cfgFn()
			cfg.UpsertProvider(newProv)
			if err := cfg.Save(); err != nil {
				return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
			}
			if m.onRefresh != nil {
				m.onRefresh()
			}
			return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Provider added and saved: %s", name)}}}
		}
		return types.Response{Signal: "open-provider-popup", Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Opening provider configuration form..."}}}
	case "use":
		if len(args) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /provider use <name>"}}}
		}
		name := strings.Join(args[1:], " ")
		cfg := m.cfgFn()
		if err := cfg.SetActive(name); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		if err := cfg.Save(); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		if m.onRefresh != nil {
			m.onRefresh()
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Active provider: %s", m.ProviderSummary())}}}
	case "remove":
		if len(args) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /provider remove <name>"}}}
		}
		name := strings.Join(args[1:], " ")
		cfg := m.cfgFn()
		if !cfg.RemoveProvider(name) {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Provider not found: %s", name)}}}
		}
		if err := cfg.Save(); err != nil {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
		}
		if m.onRefresh != nil {
			m.onRefresh()
		}
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Provider removed: %s", name)}}}
	default:
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: fmt.Sprintf("Unknown subcommand: %s", subcommand)}}}
	}
}

func (m *Manager) HandleModelsCommand(args []string) types.Response {
	cfg := m.cfgFn()
	active, ok := cfg.Active()
	if !ok {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "No active provider configured."}}}
	}
	if len(args) == 0 {
		return m.listModelsResponse(active)
	}

	subcommand := strings.ToLower(args[0])
	switch subcommand {
	case "list":
		return m.listModelsResponse(active)
	case "use":
		if len(args) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /models use <model>"}}}
		}
		return m.setActiveModel(active, strings.Join(args[1:], " "))
	case "info":
		if len(args) < 2 {
			return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: "Usage: /models info <model>"}}}
		}
		return m.modelInfoResponse(active, strings.Join(args[1:], " "))
	default:
		return m.setActiveModel(active, strings.Join(args, " "))
	}
}

func (m *Manager) listModelsResponse(active config.ProviderConfig) types.Response {
	if len(active.Models) > 0 {
		return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Available models for %s:\n%s", active.Name, strings.Join(active.Models, "\n"))}}}
	}
	return types.Response{Signal: "open-models-popup", Entries: []types.Entry{{Kind: types.EntrySystem, Text: "Fetching models..."}}}
}

func (m *Manager) setActiveModel(active config.ProviderConfig, modelID string) types.Response {
	cfg := m.cfgFn()
	active.Model = ""
	client, err := m.ProviderClient(active)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
	}
	info, err := client.GetModel(context.Background(), modelID)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
	}
	active.Model = info.ID
	active.Models = config.UniqueSortedKeep(active.Models, info.ID)
	active.ContextWindow = info.ContextWindow
	active.SupportsThinking = info.SupportsThinking
	active.EffortPresets = append([]string(nil), info.EffortPresets...)
	active.BudgetMin = info.BudgetMin
	active.BudgetMax = info.BudgetMax
	cfg.UpsertProvider(active)
	if err := cfg.Save(); err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
	}
	if m.onRefresh != nil {
		m.onRefresh()
	}
	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: fmt.Sprintf("Model set: %s", info.ID)}}}
}

func (m *Manager) modelInfoResponse(_ config.ProviderConfig, modelID string) types.Response {
	info, err := m.GetModelInfoForActiveProvider(context.Background(), modelID)
	if err != nil {
		return types.Response{Entries: []types.Entry{{Kind: types.EntryError, Text: err.Error()}}}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Model: %s", info.ID))
	if info.ContextWindow > 0 {
		lines = append(lines, fmt.Sprintf("Context window: %d", info.ContextWindow))
	}
	lines = append(lines, fmt.Sprintf("Thinking: %t", info.SupportsThinking))
	if len(info.EffortPresets) > 0 {
		lines = append(lines, fmt.Sprintf("Effort presets: %s", strings.Join(info.EffortPresets, ", ")))
	}
	if info.BudgetMin > 0 || info.BudgetMax > 0 {
		lines = append(lines, fmt.Sprintf("Thinking budget: %d-%d", info.BudgetMin, info.BudgetMax))
	}

	return types.Response{Entries: []types.Entry{{Kind: types.EntrySystem, Text: strings.Join(lines, "\n")}}}
}

var ThinkingBudgetLevels = []int{0, 1024, 8192, 24576, 65536}
var ThinkingBudgetLabels = []string{"off", "low (1k)", "medium (8k)", "high (24k)", "xhigh (64k)"}
