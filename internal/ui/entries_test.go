package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/ui/timeline"
)

func TestEntriesRendering(t *testing.T) {
	cases := []struct {
		kind app.EntryKind
		text string
		want string
	}{
		{app.EntryUser, "hello", ">"},
		{app.EntrySystem, "ready", "ready"},
		{app.EntryError, "fail", "fail"},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			m := NewTimelineModel()
			m.appendEntry(app.Entry{Kind: tc.kind, Text: tc.text})
			m.renderMessages()

			got := strings.Join(m.model.Messages, "\n")
			if !strings.Contains(got, tc.want) {
				t.Errorf("expected kind %s to contain %q, got %q", tc.kind, tc.want, got)
			}
		})
	}
}

func TestDiffOutputHighlighter(t *testing.T) {
	input := `--- a/main.go
+++ b/main.go
@@ -1,1 +1,1 @@
-old
+new`

	got := timeline.RenderDiffOutput(input)

	if !strings.Contains(got, styles.DiffAddStyle.Render("+new")) {
		t.Error("expected diff add style")
	}
	if !strings.Contains(got, styles.DiffRemoveStyle.Render("-old")) {
		t.Error("expected diff remove style")
	}
}

func TestDiffOutputCollapsing(t *testing.T) {
	var lines []string
	lines = append(lines, "--- a/file.go", "+++ b/file.go", "@@ -1,1 +1,1 @@")
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("+line %d", i))
	}
	input := strings.Join(lines, "\n")

	got := timeline.RenderDiffOutput(input)
	if !strings.Contains(got, "collapsed") {
		t.Errorf("expected large diff to be collapsed, got:\n%s", got)
	}
}

func TestMessageSelection(t *testing.T) {
	m := NewTimelineModel()
	m.SyncLayout(80, 20)

	m.appendEntry(app.Entry{Kind: app.EntryAssistant, Text: "hello world"})
	m.renderMessages()

	// Find the line index for "hello world"
	lineIdx := -1
	for i, line := range m.model.RenderLines {
		if strings.Contains(line.Plain, "hello world") {
			lineIdx = i
			break
		}
	}

	if lineIdx == -1 {
		t.Fatal("could not find rendered assistant line")
	}

	// Y coordinate for PositionAt is lineIdx - YOffset
	y := lineIdx - int(m.model.Viewport.YOffset)

	if !m.BeginSelection(3, y) {
		t.Fatal("expected selection to start")
	}
	m.UpdateSelection(10, y)

	text, ok := m.model.SelectedText()
	if !ok || text == "" {
		t.Error("expected non-empty selection")
	}
}

func TestModelCreation(t *testing.T) {
	r := app.NewRuntime()
	m := NewModel(r)
	if m.runtime == nil {
		t.Error("expected runtime to be set")
	}
}
