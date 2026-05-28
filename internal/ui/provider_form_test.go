package ui

import (
	"testing"

	"github.com/Hoosk/motoko/internal/app"
)

func TestProviderFormActivation(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)

	m.Update(ProviderModelsMsg{Models: nil, Err: nil})
	// This opens the picker, not the form.
	// We want to test the form.
}

func TestProviderFormNavigation(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)
	m.providerForm.active = true
	m.providerForm.fieldIndex = 0

	// m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// if m.providerForm.fieldIndex != 1 {
	// 	t.Errorf("expected fieldIndex 1, got %d", m.providerForm.fieldIndex)
	// }
}
