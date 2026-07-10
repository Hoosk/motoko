package provider

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/tracelog"
)

type ClientFactory func(config.ProviderConfig) Client

var (
	mu              sync.RWMutex
	clientFactories = make(map[config.ProviderKind]ClientFactory)
)

func Register(kind config.ProviderKind, factory ClientFactory) {
	mu.Lock()
	defer mu.Unlock()
	clientFactories[kind] = factory
}

func NewClient(cfg config.ProviderConfig) (Client, error) {
	cfg = config.NormalizeProvider(cfg)
	if style, ok := ResolveAPIStyle(cfg.BaseURL, cfg.Model); ok {
		tracelog.Logf("NewClient: ResolveAPIStyle(baseURL=%q, model=%q) => style=%q", cfg.BaseURL, cfg.Model, style)
		switch style {
		case apiStyleAnthropic:
			cfg.Kind = config.ProviderKindAnthropic
			cfg.Preset = config.ProviderPresetAnthropic
			cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/v1")
		case apiStyleOpenAICompatible:
			cfg.Kind = config.ProviderKindOpenAICompatible
			cfg.Preset = config.ProviderPresetOpenAICompatible
		case apiStyleOpenAI:
			cfg.Kind = config.ProviderKindOpenAICompatible
			cfg.Preset = config.ProviderPresetOpenAI
		case apiStyleGemini:
			cfg.Kind = config.ProviderKindGemini
			cfg.Preset = config.ProviderPresetGemini
		}
	} else if cfg.Preset != "" {
		if catProv, ok := LookupProvider(string(cfg.Preset)); ok {
			if cfg.BaseURL == "" && catProv.API != "" {
				cfg.BaseURL = catProv.API
			}
			switch catProv.NPM {
			case "@ai-sdk/anthropic":
				cfg.Kind = config.ProviderKindAnthropic
			case "@ai-sdk/google":
				cfg.Kind = config.ProviderKindGemini
			}
		}
	}
	mu.RLock()
	factory, ok := clientFactories[cfg.Kind]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider no soportado: %s", cfg.Kind)
	}
	return factory(cfg), nil
}

type BaseClient struct {
	httpClient   *http.Client
	providerName string
	baseURL      string
	apiKey       string
	model        string
}

func NewBaseClient(providerName, baseURL, apiKey, model string) BaseClient {
	return BaseClient{
		providerName: providerName,
		baseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:       strings.TrimSpace(apiKey),
		model:        strings.TrimSpace(model),
		httpClient: &http.Client{
			Timeout: 15 * time.Minute,
		},
	}
}

func (c BaseClient) Configured() bool {
	return c.baseURL != "" && c.apiKey != "" && c.model != ""
}

func (c BaseClient) ConfigurationError() error {
	if c.baseURL == "" {
		return fmt.Errorf("provider not configured: empty base URL")
	}
	if c.apiKey == "" {
		return fmt.Errorf("provider not configured: empty API Key")
	}
	if c.model == "" {
		return fmt.Errorf("provider not configured: model not specified")
	}
	return nil
}

func (c BaseClient) ListReady() bool {
	return c.baseURL != "" && c.apiKey != ""
}

func (c BaseClient) ProviderKind() string {
	return c.providerName
}

func (c BaseClient) Summary() string {
	return fmt.Sprintf("%s:%s", c.providerName, c.model)
}

func (c BaseClient) BaseURL() string {
	return c.baseURL
}

func (c BaseClient) APIKey() string {
	return c.apiKey
}

func (c BaseClient) Model() string {
	return c.model
}

func (c BaseClient) HTTPClient() *http.Client {
	return c.httpClient
}
