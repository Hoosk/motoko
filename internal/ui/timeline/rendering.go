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

// SyncRenderCache aligns the per-entry render cache with the current entry
// list. A width change discards all cached renders; appends only extend the
// cache, clearing re-exposed slots so stale entries can never be served.
func (m *Model) SyncRenderCache(width int) {
	n := len(m.Entries)
	if m.cacheWidth != width {
		m.renderCache = make([]renderedEntry, n)
		m.cacheWidth = width
		return
	}
	old := len(m.renderCache)
	if old == n {
		return
	}
	if cap(m.renderCache) >= n {
		grown := m.renderCache[:n]
		if n > old {
			clear(grown[old:n])
		}
		m.renderCache = grown
		return
	}
	cache := make([]renderedEntry, n)
	copy(cache, m.renderCache)
	m.renderCache = cache
}

// RenderedFor returns the rendered block for the entry together with its
// line metadata. Callers must have called SyncRenderCache for the current
// width first; out-of-range indices fall back to an uncached render. The
// returned metadata is shared cache state and must be treated as read-only.
func (m *Model) RenderedFor(idx int, entry app.Entry) (string, []RenderLine) {
	if idx < 0 || idx >= len(m.renderCache) {
		rendered := m.RenderEntry(entry)
		return rendered, m.renderEntryMetadata(entry, rendered)
	}
	cached := &m.renderCache[idx]
	if cached.valid && cached.source == entry.Text && cached.kind == entry.Kind {
		return cached.rendered, cached.meta
	}
	cached.rendered = m.RenderEntry(entry)
	cached.meta = m.renderEntryMetadata(entry, cached.rendered)
	cached.source = entry.Text
	cached.kind = entry.Kind
	cached.valid = true
	return cached.rendered, cached.meta
}

// renderEntryMetadata builds the per-line metadata for a rendered entry. The
// plain lines are stripped once per cache miss, not once per render.
func (m *Model) renderEntryMetadata(entry app.Entry, rendered string) []RenderLine {
	plainLines := strings.Split(StripANSI(rendered), "\n")
	switch entry.Kind {
	case app.EntryAssistant:
		meta := make([]RenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, RenderLine{
				Plain:      line,
				Content:    strings.TrimPrefix(line, "▎ "),
				ContentX:   AssistantContentX,
				Selectable: true,
			})
		}
		return meta
	case app.EntryReasoning:
		meta := make([]RenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, RenderLine{
				Plain:      line,
				Content:    strings.TrimPrefix(line, "  "),
				ContentX:   ReasoningContentX,
				Selectable: true,
			})
		}
		return meta
	case app.EntryUser:
		meta := make([]RenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, RenderLine{
				Plain:      line,
				Content:    line,
				ContentX:   UserContentX,
				Selectable: true,
			})
		}
		return meta
	case app.EntryCommand, app.EntryOutput, app.EntryError, app.EntrySystem, app.EntryHelp:
		return plainMetaFromLines(plainLines, true)
	default:
		return plainMetaFromLines(plainLines, false)
	}
}

func plainMetaFromLines(lines []string, selectable bool) []RenderLine {
	meta := make([]RenderLine, 0, len(lines))
	for _, line := range lines {
		meta = append(meta, RenderLine{Plain: line, Content: line, Selectable: selectable})
	}
	return meta
}

// PlainLineMetadata maps a rendered block to plain, non-content lines used
// for startup chrome that must stay out of text selection.
func PlainLineMetadata(rendered string, selectable bool) []RenderLine {
	return plainMetaFromLines(strings.Split(StripANSI(rendered), "\n"), selectable)
}

func (m *Model) AppendRenderedBlock(styled string, meta []RenderLine, addSpacer bool) {
	styledLines := strings.Split(styled, "\n")
	for i, line := range styledLines {
		var lineMeta RenderLine
		if i < len(meta) {
			lineMeta = meta[i]
			if lineMeta.Plain == "" {
				lineMeta.Plain = StripANSI(line)
			}
		} else {
			plain := StripANSI(line)
			lineMeta = RenderLine{Plain: plain, Content: plain}
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
