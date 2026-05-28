package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/ui/timeline"
	tea "github.com/charmbracelet/bubbletea"
)

func longestRuleLen(text string) int {
	maxLen := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		allRule := true
		for _, r := range trimmed {
			if r != '─' {
				allRule = false
				break
			}
		}
		if allRule && len([]rune(trimmed)) > maxLen {
			maxLen = len([]rune(trimmed))
		}
	}
	return maxLen
}

func TestTimelineStreamingAppendsAssistantDeltas(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(80, 20)
	m.SetStreaming(true)

	m.Update(AgentStreamEventMsg{Event: app.AgentStreamEvent{Kind: "assistant_delta", Content: "ho"}})
	m.Update(AgentStreamEventMsg{Event: app.AgentStreamEvent{Kind: "assistant_delta", Content: "la"}})

	if got := timeline.StripANSI(strings.Join(m.model.Messages, "\n")); !strings.Contains(got, "hola") {
		t.Fatalf("expected streamed assistant text in timeline, got %q", got)
	}
}

func TestTimelineStreamingKeepsToolEventsSeparate(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(80, 20)
	m.SetStreaming(true)

	m.Update(AgentStreamEventMsg{Event: app.AgentStreamEvent{Kind: "assistant_delta", Content: "buscando"}})
	m.Update(AgentStreamEventMsg{Event: app.AgentStreamEvent{Kind: "tool", Title: "read", Content: "README.md"}})
	m.Update(finalizeStreamMsg{Text: "hecho"})

	got := timeline.StripANSI(strings.Join(m.model.Messages, "\n"))
	if !strings.Contains(got, "tool read") {
		t.Fatalf("expected tool event in timeline, got %q", got)
	}
	if !strings.Contains(got, "hecho") {
		t.Fatalf("expected finalized assistant text in timeline, got %q", got)
	}
}

func TestTimelineUpdateIgnoresNonStreamingState(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(80, 20)

	var msg tea.Msg = AgentStreamEventMsg{Event: app.AgentStreamEvent{Kind: "assistant_delta", Content: "hola"}}
	m.Update(msg)

	got := timeline.StripANSI(strings.Join(m.model.Messages, "\n"))
	if strings.Contains(got, "hola") {
		t.Fatalf("did not expect assistant delta rendered while streaming disabled, got %q", got)
	}
}

func TestTimelineUserDelimitersRerenderOnResize(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(80, 20)
	m.Update(ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryUser, Text: "hola mundo"}}}})

	before := timeline.StripANSI(strings.Join(m.model.Messages, "\n"))
	beforeLen := longestRuleLen(before)
	if beforeLen < 40 {
		t.Fatalf("expected wide delimiter before resize, got %q", before)
	}

	m.SyncLayout(40, 20)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	after := timeline.StripANSI(strings.Join(m.model.Messages, "\n"))
	afterLen := longestRuleLen(after)
	if afterLen >= beforeLen {
		t.Fatalf("expected delimiters to rerender after resize, got %q", after)
	}
	if afterLen < 20 {
		t.Fatalf("expected narrow delimiter after resize, got %q", after)
	}
}

func TestTimelineSelectionReturnsExactWrappedText(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(40, 20)
	m.Update(ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryAssistant, Text: "alpha beta gamma delta epsilon zeta"}}}})

	startLine := -1
	endLine := -1
	for i, line := range m.model.RenderLines {
		if strings.Contains(line.Plain, "alpha beta gamma") {
			startLine = i - int(m.model.Viewport.YOffset)
		}
		if strings.Contains(line.Plain, "delta") {
			endLine = i - int(m.model.Viewport.YOffset)
		}
	}
	if startLine < 0 || endLine < 0 {
		t.Fatalf("expected wrapped lines in render map: %#v", m.model.RenderLines)
	}

	if !m.BeginSelection(0, startLine) {
		t.Fatalf("expected selection to start")
	}
	if !m.UpdateSelection(100, endLine) {
		t.Fatalf("expected selection to update")
	}

	got, ok := m.model.SelectedText()
	if !ok {
		t.Fatalf("expected selected text")
	}
	if !strings.Contains(got, "alpha beta gamma") {
		t.Fatalf("expected selected text to include wrapped content, got %q", got)
	}
	if !strings.Contains(got, "delta") {
		t.Fatalf("expected selected text to continue on next line, got %q", got)
	}
	if !m.model.SelectionDragged {
		t.Fatalf("expected drag state to be recorded")
	}
}

func TestTimelineSelectionCancelsOutsideCopyableArea(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(50, 12)
	m.Update(ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntrySystem, Text: "no copiar"}}}})

	if m.BeginSelection(0, 0) {
		t.Fatalf("expected logo area to be non-selectable")
	}
	if m.model.HasSelectionRange() {
		t.Fatalf("expected no selection range after invalid start")
	}

	m.Update(ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryAssistant, Text: "texto util"}}}})
	assistantLine := -1
	for i, line := range m.model.RenderLines {
		if strings.Contains(line.Plain, "texto util") {
			assistantLine = i - int(m.model.Viewport.YOffset)
			break
		}
	}
	if assistantLine < 0 {
		t.Fatalf("expected assistant line in render map")
	}
	if !m.BeginSelection(0, assistantLine) {
		t.Fatalf("expected assistant text to be selectable")
	}
	if !m.CancelSelection() {
		t.Fatalf("expected cancel to report change")
	}
	if m.model.HasSelectionRange() {
		t.Fatalf("expected selection to be cleared")
	}
}

func TestTimelineMouseContentCoordsRespectFrameOffsets(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(60, 12)

	if _, _, ok := m.MouseContentCoords(0, 0); ok {
		t.Fatalf("expected border area to be outside content")
	}
	x, y, ok := m.MouseContentCoords(4, 2)
	if !ok {
		t.Fatalf("expected first content cell to be addressable")
	}
	if x != 0 || y != 0 {
		t.Fatalf("unexpected content coords (%d,%d)", x, y)
	}
}

func TestInsertANSIHighlight(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		start    int
		end      int
		expected string
	}{
		{
			"plain text",
			"hello world",
			0, 5,
			timeline.SelectionBgOn + "hello" + timeline.SelectionBgOff + " world",
		},
		{
			"middle range",
			"hello world",
			6, 11,
			"hello " + timeline.SelectionBgOn + "world" + timeline.SelectionBgOff,
		},
		{
			"with existing ansi",
			"\x1b[31mred\x1b[0m text",
			0, 3,
			"\x1b[31m" + timeline.SelectionBgOn + "red" + "\x1b[0m" + timeline.SelectionBgOff + " text",
		},
		{
			"range across ansi",
			"a\x1b[31mb\x1b[0mc",
			0, 3,
			timeline.SelectionBgOn + "a\x1b[31mb\x1b[0mc" + timeline.SelectionBgOff,
		},
		{
			"range inside ansi",
			"a\x1b[31mbc\x1b[0md",
			1, 3,
			"a\x1b[31m" + timeline.SelectionBgOn + "bc" + "\x1b[0m" + timeline.SelectionBgOff + "d",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := timeline.InsertANSIHighlight(tc.input, tc.start, tc.end)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
