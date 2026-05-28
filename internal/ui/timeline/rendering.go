package timeline

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
)

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
	if idx < 2 {
		plainLines := strings.Split(StripANSI(m.Messages[idx]), "\n")
		meta := make([]RenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, RenderLine{Plain: line, Content: line, Selectable: false})
		}
		return meta
	}
	entryIdx := idx - 2
	if entryIdx < 0 || entryIdx >= len(m.Entries) {
		return nil
	}
	entry := m.Entries[entryIdx]
	switch entry.Kind {
	case app.EntryAssistant, app.EntryReasoning:
		wrapped := strings.Split(WrapText(entry.Text, m.AssistantInnerWidth()), "\n")
		meta := make([]RenderLine, 0, len(wrapped))
		for _, line := range wrapped {
			meta = append(meta, RenderLine{Content: line, ContentX: AssistantContentX, Selectable: true})
		}
		return meta
	case app.EntryUser:
		body := " >  " + entry.Text
		return []RenderLine{
			{Content: strings.Repeat("─", max(20, m.Viewport.Width)), Selectable: false},
			{Content: body, ContentX: UserContentX, Selectable: true},
			{Content: strings.Repeat("─", max(20, m.Viewport.Width)), Selectable: false},
		}
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

	logoHeight := strings.Count(LogoArt, "\n") + 1
	if y >= currentY && y < currentY+logoHeight {
		return -1
	}
	currentY += logoHeight + 2

	welcomeMsg := "Motoko online. /provider add opens the configuration form; /models lists or selects models."
	welcomeHeight := strings.Count(WrapText(welcomeMsg, m.Viewport.Width), "\n") + 1
	if y >= currentY && y < currentY+welcomeHeight {
		return -1
	}
	currentY += welcomeHeight + 2

	for i, entry := range m.Entries {
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
			return i + 2
		}

		currentY += height + 2
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
	return clamp(x, 0, m.Viewport.Width-1), clamp(y, 0, m.Viewport.Height-1)
}
