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
		{app.EntryAssistant, "hello", "▎"},
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
+new
 kept`

	got := timeline.RenderDiffOutput(input, 80)

	if !strings.Contains(got, styles.DiffAddStyle.Bold(true).Render("+")+styles.DiffAddStyle.Render("new")) {
		t.Errorf("expected styled add line, got:\n%s", got)
	}
	if !strings.Contains(got, styles.DiffRemoveStyle.Bold(true).Render("-")+styles.DiffRemoveStyle.Render("old")) {
		t.Errorf("expected styled remove line, got:\n%s", got)
	}
	if !strings.Contains(got, styles.DiffContextStyle.Render("kept")) {
		t.Errorf("expected styled context line, got:\n%s", got)
	}
	if strings.Contains(got, "--- a/main.go") || strings.Contains(got, "+++ b/main.go") {
		t.Errorf("expected headers merged into summary, got:\n%s", got)
	}
	if !strings.Contains(got, styles.DiffMetaStyle.Render("main.go")+" "+styles.DiffAddStyle.Render("+1")+" "+styles.DiffRemoveStyle.Render("-1")) {
		t.Errorf("expected merged summary with counts, got:\n%s", got)
	}
}

func TestDiffOutputWrapsLongLines(t *testing.T) {
	long := "+" + strings.Repeat("a", 60)
	input := "--- a/big.txt\n+++ b/big.txt\n" + long

	got := timeline.RenderDiffOutput(input, 40)
	rendered := strings.Split(got, "\n")

	if len(rendered) != 3 {
		t.Fatalf("expected summary + 2 wrapped lines, got %d lines:\n%s", len(rendered), got)
	}
	if !strings.HasPrefix(rendered[1], styles.DiffAddStyle.Bold(true).Render("+")) {
		t.Errorf("expected add marker on first segment, got:\n%s", rendered[1])
	}
	if !strings.HasPrefix(rendered[2], styles.DiffMetaStyle.Render(timeline.DiffWrapMarker+" ")) {
		t.Errorf("expected wrap marker on continuation, got:\n%s", rendered[2])
	}
	if !strings.Contains(rendered[2], styles.DiffAddStyle.Render(strings.Repeat("a", 20))) {
		t.Errorf("expected continuation content in add style, got:\n%s", rendered[2])
	}
}

func TestDiffOutputNoNewlineMarker(t *testing.T) {
	input := `--- a/n.txt
+++ b/n.txt
@@ -1 +1 @@
-old
\ No newline at end of file
+new
\ No newline at end of file`

	got := timeline.RenderDiffOutput(input, 80)

	if !strings.Contains(got, styles.DiffMetaStyle.Render("(no newline at end of file)")) {
		t.Errorf("expected muted no-newline note, got:\n%s", got)
	}
	if strings.Contains(got, `\ No newline`) {
		t.Errorf("expected raw marker replaced, got:\n%s", got)
	}
}

func TestDiffOutputCollapsing(t *testing.T) {
	var lines []string
	lines = append(lines, "--- a/file.go", "+++ b/file.go", "@@ -1,1 +1,1 @@")
	for i := range 30 {
		lines = append(lines, fmt.Sprintf("+line %d", i))
	}
	input := strings.Join(lines, "\n")

	got := timeline.RenderDiffOutput(input, 80)
	if !strings.Contains(got, "collapsed") {
		t.Errorf("expected large diff to be collapsed, got:\n%s", got)
	}
	if !strings.Contains(got, "file.go +30 -0") {
		t.Errorf("expected summary with counts in collapsed output, got:\n%s", got)
	}
}

func TestFullDiffOutputKeepsLargeChangesVisible(t *testing.T) {
	var lines []string
	lines = append(lines, "--- a/file.go", "+++ b/file.go", "@@ -1,1 +1,1 @@")
	for i := range 30 {
		lines = append(lines, fmt.Sprintf("+line %d", i))
	}

	got := timeline.RenderFullDiffOutput(strings.Join(lines, "\n"), 80)
	if strings.Contains(got, "collapsed") || !strings.Contains(got, "+line 29") {
		t.Fatalf("expected full diff output, got:\n%s", got)
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

func TestModelResumeHistory(t *testing.T) {
	r := app.NewRuntime(app.RuntimeOptions{Resume: true})
	m := NewModel(r)

	expectedEntries := r.StartupEntries()
	visibleEntries := m.timeline.model.VisibleEntries()

	if len(visibleEntries) != len(expectedEntries) {
		t.Errorf("expected %d entries, got %d", len(expectedEntries), len(visibleEntries))
	}
	for i, entry := range expectedEntries {
		if visibleEntries[i].Text != entry.Text || visibleEntries[i].Kind != entry.Kind {
			t.Errorf("entry %d mismatch: got %v, want %v", i, visibleEntries[i], entry)
		}
	}
}
