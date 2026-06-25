package provider

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/config"
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
	if cfg.Preset != "" {
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

type baseClient struct {
	httpClient   *http.Client
	providerName string
	baseURL      string
	apiKey       string
	model        string
}

func newBaseClient(providerName, baseURL, apiKey, model string) baseClient {
	return baseClient{
		providerName: providerName,
		baseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:       strings.TrimSpace(apiKey),
		model:        strings.TrimSpace(model),
		httpClient: &http.Client{
			Timeout: 15 * time.Minute,
		},
	}
}

func (c baseClient) Configured() bool {
	return c.baseURL != "" && c.apiKey != "" && c.model != ""
}

func (c baseClient) ConfigurationError() error {
	if c.baseURL == "" {
		return fmt.Errorf("provider no configurado: URL base vacía")
	}
	if c.apiKey == "" {
		return fmt.Errorf("provider no configurado: API Key vacía")
	}
	if c.model == "" {
		return fmt.Errorf("provider no configurado: modelo no especificado")
	}
	return nil
}

func (c baseClient) listReady() bool {
	return c.baseURL != "" && c.apiKey != ""
}

func (c baseClient) ProviderKind() string {
	return c.providerName
}

func (c baseClient) Summary() string {
	return fmt.Sprintf("%s:%s", c.providerName, c.model)
}
