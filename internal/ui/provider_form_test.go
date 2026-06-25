package ui

import (
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func TestProviderFormFlow(t *testing.T) {
	r := app.NewRuntime()
	form := &providerForm{}

	// 1. Initially closed
	if form.active {
		t.Error("expected active to be false initially")
	}

	// 2. Open form
	form.Open(r)
	if !form.active {
		t.Error("expected active to be true after Open")
	}
	if !form.showPicker {
		t.Error("expected showPicker to be true after Open")
	}
	if form.picker == nil {
		t.Fatal("expected picker to be initialized")
	}

	// 3. Verify items in picker
	presets := r.ProviderPresets()
	if len(form.picker.Items) != len(presets) {
		t.Errorf("expected %d items in picker, got %d", len(presets), len(form.picker.Items))
	}

	// 4. Filter the picker
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("open")}, r)
	if form.picker.SearchQuery != "open" {
		t.Errorf("expected query 'open', got %q", form.picker.SearchQuery)
	}

	// 5. Cancel picker
	form.Update(tea.KeyMsg{Type: tea.KeyEsc}, r)
	if form.active {
		t.Error("expected form to deactivate on cancel/esc in picker")
	}

	// 6. Re-open and select
	form.Open(r)
	// We want to select the first item by pressing Enter
	form.Update(tea.KeyMsg{Type: tea.KeyEnter}, r)
	if form.showPicker {
		t.Error("expected showPicker to be false after item selection")
	}

	preset := form.currentProviderPreset(r)
	if preset != presets[0] {
		t.Errorf("expected selected preset to be %s, got %s", presets[0], preset)
	}

	// Test backspace and field input
	form.apiKey = ""
	form.fieldIndex = 1
	form.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sk-key")}, r)
	if form.apiKey != "sk-key" {
		t.Errorf("expected apiKey 'sk-key', got %s", form.apiKey)
	}

	form.Update(tea.KeyMsg{Type: tea.KeyBackspace}, r)
	if form.apiKey != "sk-ke" {
		t.Errorf("expected apiKey 'sk-ke', got %s", form.apiKey)
	}
}
