package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
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

	if got := stripANSI(strings.Join(m.messages, "\n")); !strings.Contains(got, "hola") {
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

	got := stripANSI(strings.Join(m.messages, "\n"))
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

	got := stripANSI(strings.Join(m.messages, "\n"))
	if strings.Contains(got, "hola") {
		t.Fatalf("did not expect assistant delta rendered while streaming disabled, got %q", got)
	}
}

func TestTimelineUserDelimitersRerenderOnResize(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(80, 20)
	m.Update(ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryUser, Text: "hola mundo"}}}})

	before := stripANSI(strings.Join(m.messages, "\n"))
	beforeLen := longestRuleLen(before)
	if beforeLen < 40 {
		t.Fatalf("expected wide delimiter before resize, got %q", before)
	}

	m.SyncLayout(40, 20)
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	after := stripANSI(strings.Join(m.messages, "\n"))
	afterLen := longestRuleLen(after)
	if afterLen >= beforeLen {
		t.Fatalf("expected delimiters to rerender after resize, got %q", after)
	}
	if afterLen < 20 {
		t.Fatalf("expected narrow delimiter after resize, got %q", after)
	}
}
