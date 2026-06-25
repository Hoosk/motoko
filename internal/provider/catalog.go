package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/tracelog"
)

type CatalogModel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Family    string `json:"family"`
	Reasoning bool   `json:"reasoning"`
	Limit     struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
}

var (
	catalogMu     sync.RWMutex
	catalogCache  = make(map[string]ModelInfo)
	catalogLoaded bool
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

	req, err := http.NewRequestWithContext(fetchCtx, "GET", "https://models.dev/models.json", nil)
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
	var rawCatalog map[string]CatalogModel
	if err := json.Unmarshal(data, &rawCatalog); err != nil {
		return err
	}

	catalogCache = make(map[string]ModelInfo)
	for k, m := range rawCatalog {
		catalogCache[strings.ToLower(k)] = ModelInfo{
			ID:               m.ID,
			ContextWindow:    m.Limit.Context,
			SupportsThinking: m.Reasoning,
		}
	}
	tracelog.Logf("catalog: parsed and populated %d models from catalog", len(catalogCache))
	return nil
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
