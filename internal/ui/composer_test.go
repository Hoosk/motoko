package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func TestComposerUpdateResetsOnEnter(t *testing.T) {
	r := app.NewRuntime()
	m := NewComposerModel(r)
	m.textarea.SetValue("hello")

	// Ensure suggestions are clear to avoid cycling instead of submitting
	m.suggestions = nil
	m.mentionSuggestions = nil

	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.textarea.Value() != "" {
		t.Fatalf("expected textarea to be reset, got %q", m.textarea.Value())
	}

	if cmd == nil {
		t.Fatal("expected SubmitPromptMsg cmd")
	}
}

func TestComposerSuggestionsCycle(t *testing.T) {
	r := app.NewRuntime()
	m := NewComposerModel(r)
	m.suggestions = []string{"/help", "/models", "/provider"}
	m.selectedSuggestion = 0
	m.textarea.SetValue("/help")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.textarea.Value() != "/models" {
		t.Fatalf("expected /models after one tab, got %q", m.textarea.Value())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.textarea.Value() != "/provider" {
		t.Fatalf("expected /provider after two tabs, got %q", m.textarea.Value())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.textarea.Value() != "/help" {
		t.Fatalf("expected cycle back to /help, got %q", m.textarea.Value())
	}
}

func TestComposerMentionsCycle(t *testing.T) {
	r := app.NewRuntime()
	m := NewComposerModel(r)
	m.textarea.SetValue("read @")
	m.mentionSuggestions = []string{"main.go", "app.go"}
	m.mentionIndex = 0

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.mentionIndex != 1 {
		t.Fatalf("expected mentionIndex 1, got %d", m.mentionIndex)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// The runtime might prepend @ or not depending on context, check both or use contains
	if !strings.Contains(m.textarea.Value(), "app.go") {
		t.Fatalf("expected mention applied, got %q", m.textarea.Value())
	}
}

func TestComposerHistoryNavigation(t *testing.T) {
	r := app.NewRuntime()
	m := NewComposerModel(r)
	m.history = []string{"first", "second"}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.textarea.Value() != "second" {
		t.Fatalf("expected second, got %q", m.textarea.Value())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.textarea.Value() != "first" {
		t.Fatalf("expected first, got %q", m.textarea.Value())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.textarea.Value() != "second" {
		t.Fatalf("expected second back, got %q", m.textarea.Value())
	}
}

func TestComposerHistoryNavigationPreservesPartialInput(t *testing.T) {
	r := app.NewRuntime()
	m := NewComposerModel(r)
	m.history = []string{"past"}
	m.textarea.SetValue("partial")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.textarea.Value() != "past" {
		t.Fatalf("expected past, got %q", m.textarea.Value())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.textarea.Value() != "partial" {
		t.Fatalf("expected partial restored, got %q", m.textarea.Value())
	}
}
