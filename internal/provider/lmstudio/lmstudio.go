package lmstudio

import (
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/provider/openai"
)

func init() {
	provider.Register(config.ProviderKindLMStudio, NewClient)
}

func NewClient(cfg config.ProviderConfig) provider.Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:1234/v1"
	}
	if cfg.APIKey == "" {
		cfg.APIKey = "lm-studio"
	}
	return openai.NewClient(cfg)
}
