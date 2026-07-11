package timeline

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
)

func (m *Model) startupMessageCount() int {
	count := len(m.Messages) - len(m.VisibleEntries())
	if count < 0 {
		return 0
	}
	return count
}

// VisibleEntries returns the subset of entries to display.
func (m *Model) VisibleEntries() []app.Entry {
	if m.ShowReasoning {
		return m.Entries
	}
	var visible []app.Entry
	for _, entry := range m.Entries {
		if entry.Kind == app.EntryReasoning {
			continue
		}
		visible = append(visible, entry)
	}
	return visible
}

func (m *Model) AppendRenderedBlock(styled string, meta []RenderLine, addSpacer bool) {
	styledLines := strings.Split(styled, "\n")
	plainLines := strings.Split(StripANSI(styled), "\n")
	for i := range plainLines {
		line := ""
		if i < len(styledLines) {
			line = styledLines[i]
		}
		lineMeta := RenderLine{Plain: plainLines[i], Content: plainLines[i]}
		if i < len(meta) {
			lineMeta = meta[i]
			if lineMeta.Plain == "" {
				lineMeta.Plain = plainLines[i]
			}
			if lineMeta.Content == "" {
				lineMeta.Content = plainLines[i]
			}
		}
		m.RenderLines = append(m.RenderLines, RenderLine{
			Styled:     line,
			Plain:      lineMeta.Plain,
			Content:    lineMeta.Content,
			ContentX:   lineMeta.ContentX,
			Selectable: lineMeta.Selectable,
		})
	}
	if addSpacer {
		m.RenderLines = append(m.RenderLines, RenderLine{Plain: "", Content: "", Selectable: false})
	}
}

func (m *Model) RenderLineMetadata(idx int) []RenderLine {
	startupMessageCount := m.startupMessageCount()
	if idx < startupMessageCount {
		plainLines := strings.Split(StripANSI(m.Messages[idx]), "\n")
		meta := make([]RenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, RenderLine{Plain: line, Content: line, Selectable: false})
		}
		return meta
	}
	entryIdx := idx - startupMessageCount
	visible := m.VisibleEntries()
	if entryIdx < 0 || entryIdx >= len(visible) {
		return nil
	}
	entry := visible[entryIdx]
	switch entry.Kind {
	case app.EntryAssistant, app.EntryReasoning:
		wrapped := strings.Split(WrapText(entry.Text, m.AssistantInnerWidth()), "\n")
		meta := make([]RenderLine, 0, len(wrapped))
		contentX := AssistantContentX
		if entry.Kind == app.EntryReasoning {
			contentX = ReasoningContentX
		}
		for _, line := range wrapped {
			meta = append(meta, RenderLine{Content: line, ContentX: contentX, Selectable: true})
		}
		return meta
	case app.EntryUser:
		w := max(20, m.Viewport.Width)
		wrapped := strings.Split(WrapText(entry.Text, w-5), "\n")
		meta := make([]RenderLine, 0, len(wrapped))
		// Body lines
		for i, line := range wrapped {
			var bodyLine string
			if i == 0 {
				bodyLine = " >  " + line
			} else {
				bodyLine = "    " + line
			}
			meta = append(meta, RenderLine{Content: bodyLine, ContentX: UserContentX, Selectable: true})
		}
		return meta
	case app.EntryCommand, app.EntryOutput, app.EntryError, app.EntrySystem, app.EntryHelp:
		plainLines := strings.Split(StripANSI(m.Messages[idx]), "\n")
		meta := make([]RenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, RenderLine{Content: line, Selectable: true})
		}
		return meta
	default:
		plainLines := strings.Split(StripANSI(m.Messages[idx]), "\n")
		meta := make([]RenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, RenderLine{Content: line, Selectable: false})
		}
		return meta
	}
}

func (m *Model) MessageAtY(y int) int {
	if y < 0 || y >= m.Viewport.Height {
		return -1
	}

	currentY := -m.Viewport.YOffset
	startupMessageCount := m.startupMessageCount()

	logoHeight := strings.Count(LogoArt, "\n") + 1
	if y >= currentY && y < currentY+logoHeight {
		return -1
	}
	currentY += logoHeight + 1

	welcomeMsg := "Inspect code, edit files, run tools, or ask for a focused review."
	welcomeHeight := strings.Count(WrapText(welcomeMsg, m.Viewport.Width), "\n") + 1
	if y >= currentY && y < currentY+welcomeHeight {
		return -1
	}
	currentY += welcomeHeight + 1

	visible := m.VisibleEntries()
	for i, entry := range visible {
		copyable := entry.Kind == app.EntryAssistant ||
			entry.Kind == app.EntryReasoning ||
			entry.Kind == app.EntryUser ||
			entry.Kind == app.EntryOutput ||
			entry.Kind == app.EntryCommand ||
			entry.Kind == app.EntryError ||
			entry.Kind == app.EntrySystem ||
			entry.Kind == app.EntryHelp

		rendered := m.RenderEntry(entry)
		height := strings.Count(rendered, "\n") + 1

		if y >= currentY && y < currentY+height {
			if !copyable {
				return -1
			}
			return i + startupMessageCount
		}

		currentY += height + 1
	}

	return -1
}

func (m *Model) MouseContentCoords(x, y int) (int, int, bool) {
	x -= TimelineMouseOffsetX
	y -= TimelineMouseOffsetY
	if x < 0 || y < 0 || x >= m.Viewport.Width || y >= m.Viewport.Height {
		return 0, 0, false
	}
	return x, y, true
}

func (m *Model) ClampMouseContentCoords(x, y int) (int, int) {
	x -= TimelineMouseOffsetX
	y -= TimelineMouseOffsetY
	if m.Viewport.Width <= 0 || m.Viewport.Height <= 0 {
		return 0, 0
	}
	return clamp(x, m.Viewport.Width-1), clamp(y, m.Viewport.Height-1)
}
