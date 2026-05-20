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
	if m.Value() == "/he" {
		t.Fatalf("expected selected suggestion applied, got %q", m.Value())
	}
	if !strings.HasPrefix(m.Value(), "/") {
		t.Fatalf("expected selected suggestion applied, got %q", m.Value())
	}
}

func TestComposerTabRotatesSuggestions(t *testing.T) {
	m := NewComposerModel(app.NewRuntime())
	m.SyncLayout(80, 30)
	m.textarea.SetValue("/")
	m.refreshSuggestions()
	if len(m.suggestions) < 2 {
		t.Fatalf("expected at least two suggestions, got %#v", m.suggestions)
	}
	first := m.suggestions[m.selectedSuggestion]
	_ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	afterFirstTab := m.Value()
	_ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	afterSecondTab := m.Value()
	if afterFirstTab == "/" || afterSecondTab == "/" {
		t.Fatalf("expected suggestions to apply, got %q and %q", afterFirstTab, afterSecondTab)
	}
	if afterFirstTab == afterSecondTab {
		t.Fatalf("expected tab to rotate suggestions, got same value %q", afterFirstTab)
	}
	if afterSecondTab == first {
		t.Fatalf("expected second tab to move off first suggestion, got %q", afterSecondTab)
	}
}

func TestComposerMentionSuggestionsApplyAtTokenPosition(t *testing.T) {
	m := NewComposerModel(app.NewRuntime())
	m.SyncLayout(80, 30)
	m.textarea.SetValue("revisa @pl")
	m.refreshSuggestions()
	if len(m.suggestions) == 0 {
		t.Fatal("expected mention suggestions")
	}
	_ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !strings.Contains(m.Value(), "@plan") {
		t.Fatalf("expected @ mention replacement, got %q", m.Value())
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

func TestComposerEnterDoesNotApplyPlaceholderSuggestion(t *testing.T) {
	m := NewComposerModel(app.NewRuntime())
	m.SyncLayout(80, 30)
	m.textarea.SetValue("/tool bash echo hola")
	m.suggestions = []string{"/tool bash <comando>"}
	m.selectedSuggestion = 0

	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Value() != "" {
		t.Fatalf("expected textarea reset after submit, got %q", m.Value())
	}
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	got := cmd()
	prompt, ok := got.(SubmitPromptMsg)
	if !ok {
		t.Fatalf("expected SubmitPromptMsg, got %T", got)
	}
	if prompt.Prompt != "/tool bash echo hola" {
		t.Fatalf("expected submitted prompt preserved, got %q", prompt.Prompt)
	}
}

func TestComposerRenderSuggestionsLineKeepsActivitySlot(t *testing.T) {
	m := NewComposerModel(app.NewRuntime())
	idle := stripANSI(m.renderSuggestionsLine())
	m.SetThinking(true)
	m.refreshSuggestions()
	busy := stripANSI(m.renderSuggestionsLine())
	if !strings.Contains(busy, "planning") {
		t.Fatalf("expected activity label while thinking, got %q", busy)
	}
	if len(busy) <= len(idle)-10 {
		t.Fatalf("expected stable activity slot, idle=%q busy=%q", idle, busy)
	}
}
