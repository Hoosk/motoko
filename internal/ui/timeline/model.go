package timeline

import (
	"github.com/Hoosk/motoko/internal/app"
	"github.com/charmbracelet/bubbles/viewport"
)

type RenderLine struct {
	Styled     string
	Plain      string
	Content    string
	ContentX   int
	Selectable bool
}

type TextPos struct {
	Line   int
	Column int
}

type Model struct {
	ViewportContent  string
	Messages         []string
	Entries          []app.Entry
	RenderLines      []RenderLine
	StreamedRunes    []rune
	Viewport         viewport.Model
	SelectionFocus   TextPos
	SelectionAnchor  TextPos
	Height           int
	Width            int
	ThinkingFrame    int
	SelectedMessage  int
	StreamEntryIndex int
	AutoScroll       bool
	Selecting        bool
	SelectionDragged bool
	Thinking         bool
	Streaming        bool
	ShowReasoning    bool
}

func New(width, height int) Model {
	vp := viewport.New(width, height)
	m := Model{
		Viewport:        vp,
		AutoScroll:      true,
		SelectedMessage: -1,
		Width:           width,
		Height:          height,
		ShowReasoning:   true,
	}
	return m
}
