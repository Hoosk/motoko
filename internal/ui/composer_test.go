package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/system"
	tea "github.com/charmbracelet/bubbletea"
)

func TestComposerAppliesSuggestionAndHandlesPrompt(t *testing.T) {
	m := NewComposerModel(app.NewRuntime())
	m.SyncLayout(80, 30)
	m.textarea.SetValue("/he")
	m.refreshSuggestions()
	if len(m.suggestions) == 0 {
		t.Fatal("expected suggestions")
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !strings.HasPrefix(m.Value(), "/help") {
		t.Fatalf("expected selected suggestion applied, got %q", m.Value())
	}
}

func TestComposerHintsAndPromptReflectMode(t *testing.T) {
	r := app.NewRuntime()
	m := NewComposerModel(r)
	if got := stripANSI(m.renderSuggestionsLine()); !strings.Contains(got, "/provider add") {
		t.Fatalf("expected default chat hint, got %q", got)
	}
	r.HandleInput("/shell", system.ContextInfo{})
	m.syncInputChrome()
	if got := m.textarea.Placeholder; !strings.Contains(got, "Modo shell activo") {
		t.Fatalf("expected shell placeholder, got %q", got)
	}
	if got := stripANSI(m.renderInputPrompt()); !strings.Contains(got, "$") {
		t.Fatalf("expected shell prompt, got %q", got)
	}
}

func TestComposerSetThinkingSuppressesSuggestions(t *testing.T) {
	m := NewComposerModel(app.NewRuntime())
	m.textarea.SetValue("/he")
	m.refreshSuggestions()
	if len(m.suggestions) == 0 {
		t.Fatal("expected suggestions before thinking")
	}
	m.SetThinking(true)
	m.refreshSuggestions()
	if len(m.suggestions) != 0 {
		t.Fatalf("expected suggestions hidden while thinking, got %#v", m.suggestions)
	}
}
