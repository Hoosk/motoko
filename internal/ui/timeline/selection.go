package timeline

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ANSI background highlight constants for drag selection.
const (
	SelectionBgOn  = "\x1b[48;2;30;61;88m" // #1E3D58 dark navy
	SelectionBgOff = "\x1b[49m"            // reset background
)

func (m *Model) BeginSelection(x, y int) bool {
	pos, ok := m.PositionAt(x, y)
	if !ok {
		return m.CancelSelection()
	}
	m.Selecting = true
	m.SelectionDragged = false
	m.SelectionAnchor = pos
	m.SelectionFocus = pos
	return true
}

func (m *Model) UpdateSelection(x, y int) bool {
	if !m.Selecting {
		return false
	}
	pos, ok := m.PositionAt(x, y)
	if !ok {
		return false
	}
	if pos == m.SelectionFocus {
		return false
	}
	m.SelectionFocus = pos
	m.SelectionDragged = m.SelectionDragged || pos != m.SelectionAnchor
	m.SyncHighlight()
	return true
}

func (m *Model) EndSelection(x, y int) string {
	if !m.Selecting {
		return ""
	}
	if pos, ok := m.PositionAt(x, y); ok {
		m.SelectionFocus = pos
		m.SelectionDragged = m.SelectionDragged || pos != m.SelectionAnchor
	}
	m.Selecting = false
	m.SyncHighlight()
	if !m.SelectionDragged {
		return ""
	}
	text, _ := m.SelectedText()
	return text
}

func (m *Model) CancelSelection() bool {
	changed := m.Selecting || m.SelectionDragged || m.HasSelectionRange()
	m.Selecting = false
	m.SelectionDragged = false
	m.SelectionAnchor = TextPos{}
	m.SelectionFocus = TextPos{}
	m.SyncHighlight()
	return changed
}

func (m *Model) HasSelectionRange() bool {
	if len(m.RenderLines) == 0 {
		return false
	}
	_, _, ok := m.NormalizedSelection()
	return ok
}

func (m *Model) PositionAt(x, y int) (TextPos, bool) {
	if len(m.RenderLines) == 0 || m.Viewport.Height <= 0 {
		return TextPos{}, false
	}
	lineIdx := clamp(y+m.Viewport.YOffset, len(m.RenderLines)-1)
	line := m.RenderLines[lineIdx]
	if !line.Selectable {
		return TextPos{}, false
	}
	relX := max(0, x-line.ContentX)
	return TextPos{Line: lineIdx, Column: ColumnAt(line.Content, relX)}, true
}

func (m *Model) SelectedText() (string, bool) {
	start, end, ok := m.NormalizedSelection()
	if !ok {
		return "", false
	}
	var parts []string
	for lineIdx := start.Line; lineIdx <= end.Line; lineIdx++ {
		line := m.RenderLines[lineIdx]
		if !line.Selectable {
			continue
		}
		segment := line.Content
		switch {
		case start.Line == end.Line:
			segment = SliceColumns(line.Content, start.Column, end.Column+1)
		case lineIdx == start.Line:
			segment = SliceColumns(line.Content, start.Column, LineColumns(line.Content))
		case lineIdx == end.Line:
			segment = SliceColumns(line.Content, 0, end.Column+1)
		}
		parts = append(parts, strings.TrimRight(segment, " "))
	}
	if len(parts) == 0 {
		return "", false
	}
	text := strings.Join(parts, "\n")
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

func (m *Model) NormalizedSelection() (TextPos, TextPos, bool) {
	if len(m.RenderLines) == 0 {
		return TextPos{}, TextPos{}, false
	}
	start := m.SelectionAnchor
	end := m.SelectionFocus
	if end.Line < start.Line || (end.Line == start.Line && end.Column < start.Column) {
		start, end = end, start
	}
	if start.Line == end.Line && start.Column == end.Column {
		return TextPos{}, TextPos{}, false
	}
	return start, end, true
}

func (m *Model) HighlightedStyledLine(lineIdx int) string {
	line := m.RenderLines[lineIdx]
	if !m.Selecting || !m.SelectionDragged {
		return line.Styled
	}
	start, end, ok := m.NormalizedSelection()
	if !ok {
		return line.Styled
	}
	if lineIdx < start.Line || lineIdx > end.Line || !line.Selectable {
		return line.Styled
	}
	contentLen := LineColumns(line.Content)
	var startCol, endCol int
	if lineIdx == start.Line {
		startCol = line.ContentX + start.Column
	} else {
		startCol = line.ContentX
	}
	if lineIdx == end.Line {
		endCol = line.ContentX + end.Column + 1
	} else {
		endCol = line.ContentX + contentLen
	}
	return InsertANSIHighlight(line.Styled, startCol, endCol)
}

func (m *Model) SyncHighlight() {
	currentOffset := m.Viewport.YOffset
	lines := make([]string, len(m.RenderLines))
	for i := range m.RenderLines {
		lines[i] = m.HighlightedStyledLine(i)
	}
	m.Viewport.SetContent(strings.Join(lines, "\n"))

	if m.Viewport.Height <= 0 || len(m.RenderLines) == 0 {
		return
	}

	maxOffset := m.MaxViewportOffset()
	if m.AutoScroll || currentOffset >= maxOffset {
		m.Viewport.GotoBottom()
		m.AutoScroll = true
		return
	}
	m.Viewport.YOffset = clamp(currentOffset, maxOffset)
}

func InsertANSIHighlight(s string, startCol, endCol int) string {
	if startCol >= endCol || startCol < 0 {
		return s
	}
	var out strings.Builder
	col := 0
	highlightActive := false
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			j := i + 2
			for j < len(runes) {
				c := runes[j]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
					j++
					break
				}
				j++
			}
			out.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		if !highlightActive && col >= startCol && col < endCol {
			out.WriteString(SelectionBgOn)
			highlightActive = true
		}
		if highlightActive && col >= endCol {
			out.WriteString(SelectionBgOff)
			highlightActive = false
		}
		rw := lipgloss.Width(string(r))
		if rw == 0 {
			rw = 1
		}
		out.WriteRune(r)
		col += rw
		i++
	}
	if highlightActive {
		out.WriteString(SelectionBgOff)
	}
	return out.String()
}

func ColumnAt(line string, x int) int {
	if x <= 0 {
		return 0
	}
	width := 0
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		if rw == 0 {
			rw = 1
		}
		if x < width+rw {
			return width
		}
		width += rw
	}
	return width
}

func LineColumns(line string) int {
	return lipgloss.Width(line)
}

func SliceColumns(line string, start, end int) string {
	start = max(0, start)
	end = max(start, end)
	current := 0
	var out []rune
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		if rw == 0 {
			rw = 1
		}
		next := current + rw
		if next <= start {
			current = next
			continue
		}
		if current >= end {
			break
		}
		out = append(out, r)
		current = next
	}
	return string(out)
}

func clamp(v, max int) int {
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
