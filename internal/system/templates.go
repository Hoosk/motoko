package system

import (
	"embed"
	"path/filepath"
	"strings"
)

//go:embed providers/*.md
var providersFS embed.FS

// LoadProviderHeader returns the specific system prompt header for the given provider kind.
// If it is not found, it returns the default header.
func LoadProviderHeader(providerKind string) string {
	providerKind = strings.ToLower(providerKind)
	// Try to match anthropic, openai, gemini from the kind
	// e.g. "anthropic", "openai", "gemini-fast"
	var matched string
	if strings.Contains(providerKind, "anthropic") || strings.Contains(providerKind, "claude") {
		matched = "anthropic"
	} else if strings.Contains(providerKind, "openai") || strings.Contains(providerKind, "gpt") || strings.Contains(providerKind, "o1") || strings.Contains(providerKind, "o3") || strings.Contains(providerKind, "o4") {
		matched = "openai"
	} else if strings.Contains(providerKind, "gemini") {
		matched = "gemini"
	} else {
		matched = "default"
	}

	data, err := providersFS.ReadFile(filepath.Join("providers", matched+".md"))
	if err != nil {
		// Fallback to default
		data, _ = providersFS.ReadFile("providers/default.md")
	}
	return string(data)
}
