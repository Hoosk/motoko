package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestProviderFormDefaultsAndPresetSync(t *testing.T) {
	m := NewModel(app.NewRuntime(), func() {}, context.Background())
	m.openProviderForm()
	if m.providerForm.name != "openai" {
		t.Fatalf("expected openai default name, got %q", m.providerForm.name)
	}
	m.providerForm.fieldIndex = 0
	_ = m.handleProviderFormKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.currentProviderPreset() != config.ProviderPresetOpenRouter {
		t.Fatalf("expected preset advanced, got %q", m.currentProviderPreset())
	}
	if !strings.Contains(m.providerForm.baseURL, "openrouter.ai") {
		t.Fatalf("expected openrouter base url, got %q", m.providerForm.baseURL)
	}
}

func TestProviderFormValidationAndMaskSecret(t *testing.T) {
	m := NewModel(app.NewRuntime(), func() {}, context.Background())
	m.openProviderForm()
	m.providerForm.fieldIndex = 2 // Save button
	_ = m.handleProviderFormEnter()
	if m.providerForm.status != "La API key es obligatoria." {
		t.Fatalf("expected api key validation, got %q", m.providerForm.status)
	}
	if got := maskSecret("abcd1234"); got != "****1234" {
		t.Fatalf("unexpected secret mask %q", got)
	}
	if got := buttonLabel(true, "guardar"); got != "cargando..." {
		t.Fatalf("unexpected loading button label %q", got)
	}
}
