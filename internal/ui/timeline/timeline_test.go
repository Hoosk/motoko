package timeline

import (
	"testing"

	"github.com/Hoosk/motoko/internal/app"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "hello world", "hello world"},
		{"red text", "\x1b[31mred\x1b[0m text", "red text"},
		{"multiple styles", "\x1b[1;3;38;5;214mheavy styling\x1b[0m", "heavy styling"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripANSI(tt.input)
			if got != tt.expected {
				t.Errorf("StripANSI(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
		width    int
	}{
		{
			name:     "no wrap needed",
			text:     "hello world",
			width:    20,
			expected: "hello world",
		},
		{
			name:     "basic wrap",
			text:     "hello world",
			width:    5,
			expected: "hello\nworld",
		},
		{
			name:     "wrap long word",
			text:     "supercalifragilistic",
			width:    5,
			expected: "super\ncalif\nragil\nistic",
		},
		{
			name:     "zero width returns original",
			text:     "hello world",
			width:    0,
			expected: "hello world",
		},
		{
			name:     "negative width returns original",
			text:     "hello world",
			width:    -5,
			expected: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapText(tt.text, tt.width)
			if got != tt.expected {
				t.Errorf("WrapText(%q, %d) =\n%q\nwant:\n%q", tt.text, tt.width, got, tt.expected)
			}
		})
	}
}

func TestModelNew(t *testing.T) {
	m := New(100, 30)
	if m.Width != 100 {
		t.Errorf("expected width 100, got %d", m.Width)
	}
	if m.Height != 30 {
		t.Errorf("expected height 30, got %d", m.Height)
	}
	if !m.AutoScroll {
		t.Error("expected AutoScroll to be true by default")
	}
	if m.SelectedMessage != -1 {
		t.Errorf("expected SelectedMessage to be -1, got %d", m.SelectedMessage)
	}
	if !m.ShowReasoning {
		t.Error("expected ShowReasoning to be true by default")
	}
}

func TestMaxViewportOffset(t *testing.T) {
	m := New(80, 5)
	if offset := m.MaxViewportOffset(); offset != 0 {
		t.Errorf("expected max offset 0 for empty viewport content, got %d", offset)
	}

	m.ViewportContent = "line1\nline2\nline3"
	if offset := m.MaxViewportOffset(); offset != 0 {
		t.Errorf("expected max offset 0 when lines are less than height, got %d", offset)
	}

	m.ViewportContent = "1\n2\n3\n4\n5\n6\n7\n8\n9\n10"
	expectedOffset := 5 // 10 lines - height 5
	if offset := m.MaxViewportOffset(); offset != expectedOffset {
		t.Errorf("expected max offset %d, got %d", expectedOffset, offset)
	}
}

func TestVisibleEntries(t *testing.T) {
	entries := []app.Entry{
		{Kind: app.EntryUser, Text: "hello"},
		{Kind: app.EntryReasoning, Text: "thinking..."},
		{Kind: app.EntryAssistant, Text: "world"},
	}

	m := New(80, 20)
	m.Entries = entries

	t.Run("ShowReasoning true", func(t *testing.T) {
		m.ShowReasoning = true
		visible := m.VisibleEntries()
		if len(visible) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(visible))
		}
	})

	t.Run("ShowReasoning false", func(t *testing.T) {
		m.ShowReasoning = false
		visible := m.VisibleEntries()
		if len(visible) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(visible))
		}
		for _, e := range visible {
			if e.Kind == app.EntryReasoning {
				t.Error("did not expect reasoning entry to be visible")
			}
		}
	})
}

func TestInsertANSIHighlight(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
		start    int
		end      int
	}{
		{
			name:     "plain text",
			input:    "hello world",
			start:    0,
			end:      5,
			expected: SelectionBgOn + "hello" + SelectionBgOff + " world",
		},
		{
			name:     "middle range",
			input:    "hello world",
			start:    6,
			end:      11,
			expected: "hello " + SelectionBgOn + "world" + SelectionBgOff,
		},
		{
			name:     "with existing ansi",
			input:    "\x1b[31mred\x1b[0m text",
			start:    0,
			end:      3,
			expected: "\x1b[31m" + SelectionBgOn + "red" + "\x1b[0m" + SelectionBgOff + " text",
		},
		{
			name:     "range across ansi",
			input:    "a\x1b[31mb\x1b[0mc",
			start:    0,
			end:      3,
			expected: SelectionBgOn + "a\x1b[31mb\x1b[0mc" + SelectionBgOff,
		},
		{
			name:     "range inside ansi",
			input:    "a\x1b[31mbc\x1b[0md",
			start:    1,
			end:      3,
			expected: "a\x1b[31m" + SelectionBgOn + "bc" + "\x1b[0m" + SelectionBgOff + "d",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := InsertANSIHighlight(tc.input, tc.start, tc.end)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestSelectionBasic(t *testing.T) {
	m := New(40, 10)
	m.RenderLines = []RenderLine{
		{Plain: "line 0 word 0", Content: "line 0 word 0", Selectable: true},
		{Plain: "line 1 word 1", Content: "line 1 word 1", Selectable: true},
		{Plain: "line 2 word 2", Content: "line 2 word 2", Selectable: false},
	}

	// BeginSelection out of bounds or non-selectable
	if m.BeginSelection(0, 5) {
		t.Error("expected BeginSelection to return false out of bounds")
	}
	if m.HasSelectionRange() {
		t.Error("expected no selection range")
	}

	// CancelSelection
	if m.CancelSelection() {
		t.Error("expected CancelSelection to return false when nothing was selected")
	}
}
