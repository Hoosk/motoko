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
	Viewport         viewport.Model
	ViewportContent  string
	Messages         []string
	Entries          []app.Entry
	RenderLines      []RenderLine
	SelectedMessage  int
	Width            int
	Height           int
	AutoScroll       bool
	Streaming        bool
	StreamedRunes    []rune
	StreamEntryIndex int
	Thinking         bool
	ThinkingFrame    int
	Selecting        bool
	SelectionDragged bool
	SelectionAnchor  TextPos
	SelectionFocus   TextPos
}

func New(width, height int) Model {
	vp := viewport.New(width, height)
	m := Model{
		Viewport:        vp,
		AutoScroll:      true,
		SelectedMessage: -1,
		Width:           width,
		Height:          height,
	}
	return m
}
