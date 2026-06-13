package provider

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/config"
)

type clientFactory func(config.ProviderConfig) Client

var clientFactories = map[config.ProviderKind]clientFactory{
	config.ProviderKindOpenAICompatible: newOpenAIClient,
	config.ProviderKindAnthropic:        newAnthropicClient,
	config.ProviderKindGemini:           newGeminiClient,
}

func NewClient(cfg config.ProviderConfig) (Client, error) {
	cfg = config.NormalizeProvider(cfg)
	factory, ok := clientFactories[cfg.Kind]
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
			Timeout: 60 * time.Second,
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
