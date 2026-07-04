package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/tracelog"
)

const (
	apiStyleAnthropic        = "anthropic"
	apiStyleOpenAICompatible = "openai-compatible"
	apiStyleOpenAI           = "openai"
	apiStyleGemini           = "gemini"
)

type CatalogModelOverride struct {
	NPM string `json:"npm"`
}

type CatalogModel struct {
	Provider         *CatalogModelOverride `json:"provider,omitempty"`
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	Family           string                `json:"family"`
	ReasoningOptions []ReasoningOption     `json:"reasoning_options,omitempty"`
	Limit            struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
	Reasoning bool `json:"reasoning"`
}

type CatalogProvider struct {
	Models map[string]CatalogModel `json:"models,omitempty"`
	ID     string                  `json:"id"`
	Name   string                  `json:"name"`
	API    string                  `json:"api"`
	NPM    string                  `json:"npm"`
	Env    []string                `json:"env"`
}

type CatalogContainer struct {
	Models    map[string]CatalogModel    `json:"models"`
	Providers map[string]CatalogProvider `json:"providers"`
}

var (
	catalogMu           sync.RWMutex
	catalogCache        = make(map[string]ModelInfo)
	providersCache      = make(map[string]CatalogProvider)
	providerModelsCache = make(map[string]map[string]CatalogModel)
	catalogLoaded       bool
)

// LoadCatalog loads the catalog from the daily cache or downloads it if expired.
func LoadCatalog(ctx context.Context) error {
	catalogMu.Lock()
	defer catalogMu.Unlock()

	if catalogLoaded {
		return nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		tracelog.Logf("catalog: failed to get user cache dir: %v", err)
		return err
	}
	cachePath := filepath.Join(cacheDir, "motoko", "models_cache.json")

	useCache := false
	if info, statErr := os.Stat(cachePath); statErr == nil {
		// If modification time is less than 24 hours ago, use cache
		if time.Since(info.ModTime()) < 24*time.Hour {
			useCache = true
		}
	}

	var data []byte
	if useCache {
		tracelog.Logf("catalog: reading from cache file %s", cachePath)
		data, err = os.ReadFile(cachePath)
		if err == nil {
			if parseErr := parseAndPopulate(data); parseErr == nil {
				catalogLoaded = true
				return nil
			}
			tracelog.Logf("catalog: failed to parse cached data: %v", err)
		} else {
			tracelog.Logf("catalog: failed to read cached file: %v", err)
		}
	}

	// Try fetching from models.dev
	tracelog.Logf("catalog: fetching models catalog from models.dev...")
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, "GET", "https://models.dev/catalog.json", nil)
	if err != nil {
		tracelog.Logf("catalog: failed to create request: %v", err)
		return fallbackToCache(cachePath)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		tracelog.Logf("catalog: request failed: %v", err)
		return fallbackToCache(cachePath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tracelog.Logf("catalog: request returned non-200 status: %d", resp.StatusCode)
		return fallbackToCache(cachePath)
	}

	data, err = io.ReadAll(resp.Body)
	if err != nil {
		tracelog.Logf("catalog: failed to read response body: %v", err)
		return fallbackToCache(cachePath)
	}

	// Write cache file in background/best-effort
	_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		tracelog.Logf("catalog: failed to write cache file: %v", err)
	} else {
		tracelog.Logf("catalog: cached models catalog successfully to %s", cachePath)
	}

	if err := parseAndPopulate(data); err != nil {
		return err
	}

	catalogLoaded = true
	return nil
}

func fallbackToCache(cachePath string) error {
	tracelog.Logf("catalog: falling back to reading cache file %s", cachePath)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		tracelog.Logf("catalog: failed to read fallback cache file: %v", err)
		return err
	}
	if err := parseAndPopulate(data); err != nil {
		tracelog.Logf("catalog: failed to parse fallback cache file: %v", err)
		return err
	}
	catalogLoaded = true
	return nil
}

func parseAndPopulate(data []byte) error {
	var container CatalogContainer
	// Try parsing nested structure
	if err := json.Unmarshal(data, &container); err == nil && (len(container.Models) > 0 || len(container.Providers) > 0) {
		catalogCache = make(map[string]ModelInfo)
		for k, m := range container.Models {
			catalogCache[strings.ToLower(k)] = modelInfoFromCatalogModel(m)
		}
		providersCache = make(map[string]CatalogProvider)
		providerModelsCache = make(map[string]map[string]CatalogModel)
		for k, p := range container.Providers {
			provKey := strings.ToLower(k)
			providersCache[provKey] = p
			if len(p.Models) > 0 {
				modelsLower := make(map[string]CatalogModel, len(p.Models))
				for modelID, m := range p.Models {
					modelsLower[strings.ToLower(modelID)] = m
					mergeCatalogModelInfo(catalogKeyForProviderModel(provKey, modelID, m), m)
				}
				providerModelsCache[provKey] = modelsLower
			}
		}
		tracelog.Logf("catalog: parsed nested catalog.json with %d models and %d providers", len(catalogCache), len(providersCache))
		return nil
	}

	// Fallback to legacy flat model mapping
	var rawCatalog map[string]CatalogModel
	if err := json.Unmarshal(data, &rawCatalog); err != nil {
		return err
	}

	catalogCache = make(map[string]ModelInfo)
	for k, m := range rawCatalog {
		catalogCache[strings.ToLower(k)] = modelInfoFromCatalogModel(m)
	}
	// Clear providers cache in legacy mode
	providersCache = make(map[string]CatalogProvider)
	providerModelsCache = make(map[string]map[string]CatalogModel)
	tracelog.Logf("catalog: parsed legacy models.json with %d models", len(catalogCache))
	return nil
}

func modelInfoFromCatalogModel(m CatalogModel) ModelInfo {
	info := ModelInfo{
		ID:               m.ID,
		ContextWindow:    m.Limit.Context,
		SupportsThinking: m.Reasoning,
	}
	applyReasoningOptions(&info, m.ReasoningOptions)
	return info
}

func applyReasoningOptions(info *ModelInfo, options []ReasoningOption) {
	if info == nil {
		return
	}
	for _, option := range options {
		switch strings.ToLower(strings.TrimSpace(option.Type)) {
		case "effort":
			if len(info.EffortPresets) == 0 && len(option.Values) > 0 {
				info.EffortPresets = append([]string(nil), option.Values...)
			}
		case "budget_tokens":
			if info.BudgetMin == 0 && option.Min > 0 {
				info.BudgetMin = option.Min
			}
			if info.BudgetMax == 0 && option.Max > 0 {
				info.BudgetMax = option.Max
			}
		}
	}
}

func mergeCatalogModelInfo(key string, model CatalogModel) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return
	}
	merged := catalogCache[key]
	if merged.ID == "" {
		merged = modelInfoFromCatalogModel(model)
	}
	applyReasoningOptions(&merged, model.ReasoningOptions)
	if merged.ID == "" {
		merged.ID = model.ID
	}
	if merged.ContextWindow == 0 {
		merged.ContextWindow = model.Limit.Context
	}
	merged.SupportsThinking = merged.SupportsThinking || model.Reasoning
	catalogCache[key] = merged
}

func catalogKeyForProviderModel(providerID, modelID string, model CatalogModel) string {
	if strings.TrimSpace(model.ID) != "" {
		return strings.ToLower(strings.TrimSpace(model.ID))
	}
	return strings.ToLower(strings.TrimSpace(providerID) + "/" + strings.TrimSpace(modelID))
}

func normalizeAPIURL(raw string) string {
	u := strings.ToLower(strings.TrimSpace(raw))
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, "/v1")
	return u
}

func LookupProviderByAPI(baseURL string) (CatalogProvider, string, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	norm := normalizeAPIURL(baseURL)
	for provID, p := range providersCache {
		if normalizeAPIURL(p.API) == norm {
			return p, provID, true
		}
	}
	return CatalogProvider{}, "", false
}

func ResolveAPIStyle(baseURL, modelID string) (string, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	norm := normalizeAPIURL(baseURL)
	modelLower := strings.ToLower(strings.TrimSpace(modelID))

	for provID, p := range providersCache {
		if normalizeAPIURL(p.API) != norm {
			continue
		}

		npm := p.NPM
		if provModels, ok := providerModelsCache[provID]; ok {
			if model, found := provModels[modelLower]; found && model.Provider != nil && model.Provider.NPM != "" {
				npm = model.Provider.NPM
			}
		}

		switch npm {
		case "@ai-sdk/anthropic":
			return apiStyleAnthropic, true
		case "@ai-sdk/openai":
			if provID == "openai" {
				return apiStyleOpenAI, true
			}
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/openai-compatible":
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/google":
			return apiStyleGemini, true
		case "@openrouter/ai-sdk-provider":
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/azure":
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/cohere":
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/xai":
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/amazon-bedrock":
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/amazon-bedrock/mantle":
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/google-vertex":
			return apiStyleOpenAICompatible, true
		case "@ai-sdk/google-vertex/anthropic":
			return apiStyleOpenAICompatible, true
		case "ai-gateway-provider":
			return apiStyleOpenAICompatible, true
		default:
			return apiStyleOpenAICompatible, true
		}
	}
	return "", false
}

// LookupProvider searches for a provider in the catalog cache.
func LookupProvider(providerID string) (CatalogProvider, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	p, ok := providersCache[strings.ToLower(strings.TrimSpace(providerID))]
	return p, ok
}

// ListCatalogProviders returns a list of provider IDs from the catalog cache.
func ListCatalogProviders() []string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	var list []string
	for k := range providersCache {
		list = append(list, k)
	}
	sort.Strings(list)
	return list
}

// LookupModel searches for a model in the catalog cache.
func LookupModel(providerName string, model string) (ModelInfo, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	modelLower := strings.ToLower(strings.TrimSpace(model))
	providerLower := strings.ToLower(strings.TrimSpace(providerName))

	// 1. Direct match
	if info, ok := catalogCache[modelLower]; ok {
		return info, true
	}

	// 2. Map provider name to prefixes
	var prefixes []string
	switch {
	case strings.Contains(providerLower, "openai"):
		prefixes = []string{"openai/"}
	case strings.Contains(providerLower, "anthropic"):
		prefixes = []string{"anthropic/"}
	case strings.Contains(providerLower, "gemini") || strings.Contains(providerLower, "google"):
		prefixes = []string{"google/"}
	case strings.Contains(providerLower, "deepseek"):
		prefixes = []string{"deepseek/"}
	case strings.Contains(providerLower, "mistral"):
		prefixes = []string{"mistral/"}
	case strings.Contains(providerLower, "xai") || strings.Contains(providerLower, "grok"):
		prefixes = []string{"xai/"}
	}

	for _, prefix := range prefixes {
		if !strings.HasPrefix(modelLower, prefix) {
			key := prefix + modelLower
			if info, ok := catalogCache[key]; ok {
				return info, true
			}
		}
	}

	// 3. Suffix match (e.g. check if catalog key ends with "/" + model)
	suffix := "/" + modelLower
	for k, info := range catalogCache {
		if strings.HasSuffix(k, suffix) {
			return info, true
		}
	}

	return ModelInfo{}, false
}

func EnrichModelInfo(providerName string, info ModelInfo) ModelInfo {
	if strings.TrimSpace(info.ID) == "" {
		return info
	}
	cached, ok := LookupModel(providerName, info.ID)
	if !ok {
		return info
	}
	if info.ContextWindow == 0 {
		info.ContextWindow = cached.ContextWindow
	}
	info.SupportsThinking = info.SupportsThinking || cached.SupportsThinking
	if len(info.EffortPresets) == 0 && len(cached.EffortPresets) > 0 {
		info.EffortPresets = append([]string(nil), cached.EffortPresets...)
	}
	if info.BudgetMin == 0 {
		info.BudgetMin = cached.BudgetMin
	}
	if info.BudgetMax == 0 {
		info.BudgetMax = cached.BudgetMax
	}
	return info
}
