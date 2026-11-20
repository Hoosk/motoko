package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newSizedModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(app.NewRuntime())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return updated.(Model)
}

func sendKeys(m Model, keys ...tea.KeyMsg) Model {
	for _, k := range keys {
		updated, _ := m.Update(k)
		m = updated.(Model)
	}
	return m
}

func runes(s string) []tea.KeyMsg {
	keys := make([]tea.KeyMsg, 0, len(s))
	for _, r := range s {
		if r == ' ' {
			keys = append(keys, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return keys
}

func TestTypingDoesNotScrollTimeline(t *testing.T) {
	m := newSizedModel(t)
	for i := range 200 {
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: strings.Repeat("line ", 5) + string(rune('a'+i%26))})
	}
	m.timeline.renderMessages()
	m.timeline.model.Viewport.GotoBottom()
	bottom := m.timeline.model.Viewport.YOffset
	if bottom == 0 {
		t.Fatal("test setup: timeline should be scrollable")
	}

	// The default viewport keymap pages on f/b/space/d/u and moves on j/k.
	m = sendKeys(m, runes("fix bug jk du")...)

	if got := m.timeline.model.Viewport.YOffset; got != bottom {
		t.Fatalf("typing scrolled the timeline from %d to %d", bottom, got)
	}
	if !m.timeline.model.AutoScroll {
		t.Fatal("typing disabled auto-scroll")
	}
	if got := m.composer.textarea.Value(); got != "fix bug jk du" {
		t.Fatalf("composer got %q", got)
	}

	m = sendKeys(m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.timeline.model.Viewport.YOffset >= bottom {
		t.Fatal("expected PgUp to still scroll the timeline")
	}
	if got := m.composer.textarea.Value(); got != "fix bug jk du" {
		t.Fatalf("PgUp leaked into the composer: %q", got)
	}
}

func TestGlobalShortcutsDoNotEditComposer(t *testing.T) {
	tests := []struct {
		isOpen func(Model) bool
		name   string
		key    tea.KeyType
	}{
		{name: "ctrl+k palette (textarea: delete to end of line)", key: tea.KeyCtrlK, isOpen: func(m Model) bool { return m.commandPalette.active }},
		{name: "ctrl+a modes (textarea: line start)", key: tea.KeyCtrlA, isOpen: func(m Model) bool { return m.modePopup.active }},
		{name: "ctrl+t tools (textarea: transpose)", key: tea.KeyCtrlT, isOpen: func(m Model) bool { return m.showTools }},
		{name: "ctrl+g help", key: tea.KeyCtrlG, isOpen: func(m Model) bool { return m.helpOverlay.active }},
		{name: "ctrl+p provider (composer: history up)", key: tea.KeyCtrlP, isOpen: func(m Model) bool { return m.providerForm.active }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newSizedModel(t)
			m.composer.history = []string{"previous prompt"}
			m = sendKeys(m, runes("hello world")...)
			m.composer.textarea.SetCursor(5)

			m = sendKeys(m, tea.KeyMsg{Type: tt.key})

			if !tt.isOpen(m) {
				t.Fatal("expected the shortcut to run")
			}
			if got := m.composer.textarea.Value(); got != "hello world" {
				t.Fatalf("shortcut edited the composer: %q", got)
			}
		})
	}
}

func TestBackspaceDoesNotOpenHelp(t *testing.T) {
	// Many terminals (and tmux setups) send ^H for Backspace, which arrives
	// as ctrl+h; it used to toggle the help overlay instead of deleting.
	m := newSizedModel(t)
	m = sendKeys(m, runes("ab")...)
	m = sendKeys(m, tea.KeyMsg{Type: tea.KeyCtrlH})
	if m.helpOverlay.active {
		t.Fatal("backspace opened help")
	}
	if got := m.composer.textarea.Value(); got != "a" {
		t.Fatalf("expected backspace to delete, got %q", got)
	}
}

func TestEveryPaletteShortcutIsHandled(t *testing.T) {
	ctx := paletteContext{QueueLen: 1, Thinking: true}
	items := append(actionPaletteItems(ctx), shortcutPaletteItems()...)
	for _, item := range items {
		p, ok := item.(paletteItem)
		if !ok || p.shortcut == "" {
			continue
		}
		m := newSizedModel(t)
		m.promptQueue = []string{"queued"}
		if _, handled := m.runShortcut(p.shortcut, nil); !handled {
			t.Errorf("palette item %q uses unhandled shortcut %q", p.title, p.shortcut)
		}
	}
}

func TestComposerArrowsMoveCursorWhileSuggestionsShow(t *testing.T) {
	m := NewComposerModel(app.NewRuntime())
	m.textarea.SetValue("/hel")
	m.suggestions = []string{"/help", "/models"}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})

	if got := m.textarea.Value(); got != "/hel" {
		t.Fatalf("Left replaced the input with a suggestion: %q", got)
	}
	if info := m.textarea.LineInfo(); info.ColumnOffset != 3 {
		t.Fatalf("expected cursor to move left to column 3, got %d", info.ColumnOffset)
	}
}

func TestComposerSubmitsAfterCompletingTrailingMention(t *testing.T) {
	r := app.NewRuntime()
	r.SemanticIndex().SetSnapshotForTest(&semantic.Snapshot{Snapshot: symtypes.Snapshot{
		GeneratedAt: time.Now(),
		Files:       []semantic.FileSummary{{Path: "README.md"}},
	}})
	m := NewComposerModel(r)
	m.textarea.SetValue("explain @REA")
	m.refreshSuggestions()
	if len(m.mentionSuggestions) == 0 || m.mentionSuggestions[0] != "@README.md" {
		t.Fatalf("test setup: expected @README.md suggestion, got %v", m.mentionSuggestions)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.textarea.Value(); got != "explain @README.md " {
		t.Fatalf("expected mention applied, got %q", got)
	}
	if len(m.mentionSuggestions) != 0 {
		t.Fatalf("completed mention kept the dropdown open: %v", m.mentionSuggestions)
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected the second Enter to submit")
	}
	if submit, ok := cmd().(SubmitPromptMsg); !ok || submit.Prompt != "explain @README.md " {
		t.Fatalf("unexpected submit message %#v", cmd())
	}
	if m.textarea.Value() != "" {
		t.Fatalf("expected composer reset after submit, got %q", m.textarea.Value())
	}
}

func TestComposerToolbarStaysOnOneLine(t *testing.T) {
	for _, width := range []int{30, 60, 80, 120} {
		m := newSizedModel(t)
		m.timeline.SetThinking(true)
		m.promptQueue = []string{"a", "b"}
		if h := lipgloss.Height(m.renderComposerToolbar(width)); h != 1 {
			t.Errorf("toolbar wrapped to %d lines at width %d", h, width)
		}
	}
}

func TestTruncateIsWidthAndEncodingSafe(t *testing.T) {
	styled := styles.WhiteStyle.Render("/tool brain_read") + " " + styles.GrayStyle.Render(`{"filename": "plan.md"}`)
	tests := []string{
		"ñandú, café y acción",
		"日本語のテキストです",
		styled,
	}
	for _, in := range tests {
		got := truncate(in, 9)
		if !utf8.ValidString(got) {
			t.Errorf("truncate(%q) produced invalid UTF-8: %q", in, got)
		}
		if w := lipgloss.Width(got); w > 9 {
			t.Errorf("truncate(%q) is %d columns wide, want <= 9", in, w)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("truncate(%q) = %q, want ellipsis", in, got)
		}
	}
	if got := truncate("short", 9); got != "short" {
		t.Errorf("short strings must be unchanged, got %q", got)
	}
}

func TestFilterListBackspaceRemovesWholeRune(t *testing.T) {
	fl := NewFilterList("t", "p")
	fl.Active = true
	fl.Filter("añ")
	fl.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if fl.SearchQuery != "a" {
		t.Fatalf("expected %q, got %q", "a", fl.SearchQuery)
	}
}
