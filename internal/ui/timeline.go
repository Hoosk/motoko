package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type timelineRenderLine struct {
	styled     string
	plain      string
	content    string
	contentX   int
	selectable bool
}

type timelineTextPos struct {
	line   int
	column int
}

type TimelineModel struct {
	viewport         viewport.Model
	viewportContent  string
	messages         []string
	entries          []app.Entry
	renderLines      []timelineRenderLine
	selectedMessage  int
	width            int
	height           int
	autoScroll       bool
	streaming        bool
	streamedRunes    []rune
	streamEntryIndex int
	thinking         bool
	thinkingFrame    int
	selecting        bool
	selectionDragged bool
	selectionAnchor  timelineTextPos
	selectionFocus   timelineTextPos
}

const (
	timelineMouseOffsetX = 4
	timelineMouseOffsetY = 2
	assistantContentX    = 3
	userContentX         = 4
)

// ANSI background highlight constants for drag selection.
const (
	selectionBgOn  = "\x1b[48;2;30;61;88m" // #1E3D58 dark navy
	selectionBgOff = "\x1b[49m"             // reset background
)

func NewTimelineModel() TimelineModel {
	vp := viewport.New(80, 20)
	m := TimelineModel{
		viewport:        vp,
		autoScroll:      true,
		selectedMessage: -1,
	}
	m.resetMessages()
	return m
}

func (m TimelineModel) Init() tea.Cmd {
	return nil
}

func (m *TimelineModel) Update(msg tea.Msg) tea.Cmd {
	var vpCmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.renderMessages()

	case ClearMessagesMsg:
		m.resetMessages()

	case ResponseAppliedMsg:
		for _, entry := range msg.Response.Entries {
			m.appendEntry(entry)
		}
		m.renderMessages()

	case AgentStreamEventMsg:
		if m.streaming {
			event := msg.Event
			if event.Kind == "assistant_delta" || event.Kind == "thinking_delta" {
				targetKind := app.EntryAssistant
				content := event.Content
				if event.Kind == "thinking_delta" {
					targetKind = app.EntryReasoning
					content = event.ReasoningContent
				}

				if m.streamEntryIndex == -1 || m.entries[m.streamEntryIndex].Kind != targetKind {
					m.appendEntry(app.Entry{Kind: targetKind, Text: ""})
					m.streamEntryIndex = len(m.entries) - 1
					m.streamedRunes = nil
				}
				if content != "" {
					m.streamedRunes = append(m.streamedRunes, []rune(content)...)
					if m.streamEntryIndex >= 0 && m.streamEntryIndex < len(m.entries) {
						m.entries[m.streamEntryIndex].Text = string(m.streamedRunes)
					}
				}
			} else {
				switch event.Kind {
				case "tool":
					m.appendEntry(app.Entry{Kind: app.EntryCommand, Text: fmt.Sprintf("tool %s", event.Title)})
					if strings.TrimSpace(event.Content) != "" {
						m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: event.Content})
					}
				case "output":
					m.appendEntry(app.Entry{Kind: app.EntryOutput, Text: event.Content})
				case "error":
					m.appendEntry(app.Entry{Kind: app.EntryError, Text: event.Content})
				case "debug":
					m.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "[debug] " + event.Content})
				}
				if m.streamEntryIndex != -1 {
					m.streamEntryIndex = -1
					m.streamedRunes = nil
				}
			}
			m.renderMessages()
		}

	case finalizeStreamMsg:
		m.CompleteStreaming(msg.Text)

	case ProviderModelsMsg:
		for _, entry := range entriesForProviderModels(msg.Models, msg.Err) {
			m.appendEntry(entry)
		}
		m.renderMessages()

	case ThinkingTickMsg:
		if m.thinking {
			m.thinkingFrame = (m.thinkingFrame + 1) % len(thinkingFrames)
			m.renderMessages()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "down":
			// Reserved for composer history navigation; do not scroll viewport.
			return nil
		case "alt+up":
			if len(m.messages) > 0 {
				if m.selectedMessage < 0 {
					m.selectedMessage = len(m.messages) - 1
				} else {
					m.selectedMessage = clamp(m.selectedMessage-1, 0, len(m.messages)-1)
				}
				m.renderMessages()
			}
			return nil
		case "alt+down":
			if len(m.messages) > 0 {
				if m.selectedMessage < 0 {
					m.selectedMessage = 0
				} else {
					m.selectedMessage = clamp(m.selectedMessage+1, 0, len(m.messages)-1)
				}
				m.renderMessages()
			}
			return nil
		case "alt+c":
			if text, ok := m.selectedText(); ok {
				return copySelection(text)
			}
			if m.selectedMessage >= 0 && m.selectedMessage < len(m.messages) {
				return copySelection(stripANSI(m.messages[m.selectedMessage]))
			}
		}
	}

	m.viewport, vpCmd = m.viewport.Update(msg)
	m.autoScroll = m.viewport.YOffset >= m.maxViewportOffset()
	cmds = append(cmds, vpCmd)

	return tea.Batch(cmds...)
}

func (m TimelineModel) View() string {
	if m.width == 0 {
		return ""
	}
	return styles.TimelineStyle.Width(m.width).Height(m.viewport.Height).Render(m.viewport.View())
}

func (m TimelineModel) MouseContentCoords(x, y int) (int, int, bool) {
	x -= timelineMouseOffsetX
	y -= timelineMouseOffsetY
	if x < 0 || y < 0 || x >= m.viewport.Width || y >= m.viewport.Height {
		return 0, 0, false
	}
	return x, y, true
}

func (m TimelineModel) ClampMouseContentCoords(x, y int) (int, int) {
	x -= timelineMouseOffsetX
	y -= timelineMouseOffsetY
	if m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		return 0, 0
	}
	return clamp(x, 0, m.viewport.Width-1), clamp(y, 0, m.viewport.Height-1)
}

func (m *TimelineModel) SyncLayout(width, height int) {
	widthChanged := m.width != width
	heightChanged := m.viewport.Height != height

	if !widthChanged && !heightChanged {
		return
	}

	m.width = width
	m.viewport.Width = width - 6
	m.viewport.Height = height

	if widthChanged {
		m.renderMessages()
	} else if heightChanged {
		m.syncHighlight()
	}
}

func (m *TimelineModel) resetMessages() {
	styledLogo := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(logoArt)
	m.messages = []string{
		styledLogo,
		styles.SystemStyle.Render("Motoko online. /provider add opens the configuration form; /models lists or selects models."),
	}
	m.entries = nil
	m.selectedMessage = -1
	if m.viewport.Width > 0 {
		m.renderMessages()
	}
}

func (m *TimelineModel) appendEntry(entry app.Entry) {
	m.entries = append(m.entries, entry)
}

func (m *TimelineModel) renderMessages() {
	width := m.viewport.Width
	if width <= 0 {
		return
	}
	selectedIdx := -1
	m.renderLines = m.renderLines[:0]
	m.messages = m.messages[:0]
	styledLogo := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(logoArt)
	m.messages = append(m.messages,
		styledLogo,
		styles.SystemStyle.Render("Motoko online. /provider add opens the configuration form; /models lists or selects models."),
	)
	for _, entry := range m.entries {
		m.messages = append(m.messages, m.renderEntry(entry))
	}
	if m.selectedMessage >= 0 && len(m.messages) > 0 {
		selectedIdx = clamp(m.selectedMessage, 0, len(m.messages)-1)
	}
	for i, msg := range m.messages {
		rendered := msg
		if i == selectedIdx {
			rendered = styles.SelectedMessageStyle.Render(msg)
		}
		m.appendRenderedBlock(rendered, m.renderLineMetadata(i), i < len(m.messages)-1)
	}
	if m.thinking {
		spinner := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(thinkingFrames[m.thinkingFrame])
		label := lipgloss.NewStyle().Foreground(styles.Gray).Italic(true).Render("  processing")
		m.appendRenderedBlock(spinner+label, []timelineRenderLine{{content: stripANSI(spinner + label)}}, false)
	}
	// Store plain styled lines for line-count / scroll math (maxViewportOffset).
	styledLines := make([]string, len(m.renderLines))
	for i, line := range m.renderLines {
		styledLines[i] = line.styled
	}
	m.viewportContent = strings.Join(styledLines, "\n")
	// Apply any active selection highlight and push to viewport (preserves scroll).
	m.syncHighlight()
}

func (m *TimelineModel) renderEntry(entry app.Entry) string {
	switch entry.Kind {
	case app.EntryUser:
		return renderUserMessage(entry.Text, max(20, m.viewport.Width))
	case app.EntryAssistant:
		wrapped := wrapText(entry.Text, m.assistantInnerWidth())
		return styles.AssistantBlockStyle.Render(wrapped)
	case app.EntryReasoning:
		wrapped := wrapText(entry.Text, m.assistantInnerWidth())
		return styles.ReasoningBlockStyle.Render(wrapped)
	case app.EntrySystem:
		return styles.SystemStyle.Render(entry.Text)
	case app.EntryCommand:
		return styles.CommandStyle.Render(entry.Text)
	case app.EntryOutput:
		return renderDiffOutput(entry.Text)
	case app.EntryError:
		return styles.ErrorStyle.Render(entry.Text)
	case app.EntryHelp:
		return renderHelpEntry(entry.Text)
	default:
		return entry.Text
	}
}

func (m TimelineModel) maxViewportOffset() int {
	if m.viewport.Height <= 0 || m.viewportContent == "" {
		return 0
	}
	lineCount := strings.Count(m.viewportContent, "\n") + 1
	return max(0, lineCount-m.viewport.Height)
}

func (m *TimelineModel) SetThinking(thinking bool) {
	m.thinking = thinking
	if thinking {
		m.thinkingFrame = 0
	}
}

func (m *TimelineModel) SetStreaming(streaming bool) {
	m.streaming = streaming
	if streaming {
		m.streamedRunes = nil
		m.streamEntryIndex = -1
		return
	}
	m.streamedRunes = nil
	m.streamEntryIndex = -1
}

func (m *TimelineModel) CompleteStreaming(text string) {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" {
		if m.streamEntryIndex == -1 {
			m.appendEntry(app.Entry{Kind: app.EntryAssistant, Text: trimmed})
		} else if m.streamEntryIndex >= 0 && m.streamEntryIndex < len(m.entries) {
			m.entries[m.streamEntryIndex].Text = trimmed
		}
	}
	m.streaming = false
	m.streamedRunes = nil
	m.streamEntryIndex = -1
	m.renderMessages()
}

func (m TimelineModel) CopySelected() tea.Cmd {
	if m.selectedMessage >= 0 && m.selectedMessage < len(m.messages) {
		return copySelection(stripANSI(m.messages[m.selectedMessage]))
	}
	return nil
}

func (m TimelineModel) CopyRange(startIdx, endIdx int) tea.Cmd {
	if startIdx < 0 || endIdx < 0 {
		return nil
	}
	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}
	
	var parts []string
	for i := startIdx; i <= endIdx && i < len(m.messages); i++ {
		parts = append(parts, stripANSI(m.messages[i]))
	}
	
	if len(parts) == 0 {
		return nil
	}
	
	return copySelection(strings.Join(parts, "\n\n"))
}

func (m *TimelineModel) BeginSelection(x, y int) bool {
	pos, ok := m.positionAt(x, y)
	if !ok {
		return m.CancelSelection()
	}
	m.selecting = true
	m.selectionDragged = false
	m.selectionAnchor = pos
	m.selectionFocus = pos
	return true
}

func (m *TimelineModel) UpdateSelection(x, y int) bool {
	if !m.selecting {
		return false
	}
	pos, ok := m.positionAt(x, y)
	if !ok {
		return false
	}
	if pos == m.selectionFocus {
		return false
	}
	m.selectionFocus = pos
	m.selectionDragged = m.selectionDragged || pos != m.selectionAnchor
	m.syncHighlight()
	return true
}

func (m *TimelineModel) EndSelection(x, y int) tea.Cmd {
	if !m.selecting {
		return nil
	}
	if pos, ok := m.positionAt(x, y); ok {
		m.selectionFocus = pos
		m.selectionDragged = m.selectionDragged || pos != m.selectionAnchor
	}
	m.selecting = false
	m.syncHighlight()
	if !m.selectionDragged {
		return nil
	}
	text, ok := m.selectedText()
	if !ok {
		return nil
	}
	return copySelection(text)
}

func (m *TimelineModel) CancelSelection() bool {
	changed := m.selecting || m.selectionDragged || m.hasSelectionRange()
	m.selecting = false
	m.selectionDragged = false
	m.selectionAnchor = timelineTextPos{}
	m.selectionFocus = timelineTextPos{}
	m.syncHighlight()
	return changed
}

func (m *TimelineModel) hasSelectionRange() bool {
	if len(m.renderLines) == 0 {
		return false
	}
	_, _, ok := m.normalizedSelection()
	return ok
}

// insertANSIHighlight applies a background-highlight to visual columns [startCol, endCol)
// within an ANSI-coloured string. It walks rune by rune, skipping ESC[…X sequences (which
// consume zero visual columns) and injects the background-on/off codes at the correct positions.
func insertANSIHighlight(s string, startCol, endCol int) string {
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
		// Skip ANSI escape sequences: ESC '[' <params> <letter>
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
		// Inject highlight-on before the first selected column.
		if !highlightActive && col >= startCol && col < endCol {
			out.WriteString(selectionBgOn)
			highlightActive = true
		}
		// Inject highlight-off at the end of the selected range.
		if highlightActive && col >= endCol {
			out.WriteString(selectionBgOff)
			highlightActive = false
		}
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			rw = 1
		}
		out.WriteRune(r)
		col += rw
		i++
	}
	if highlightActive {
		out.WriteString(selectionBgOff)
	}
	return out.String()
}

// highlightedStyledLine returns the styled string for renderLines[lineIdx] with the
// current drag-selection background applied. The original line.styled is never mutated.
func (m *TimelineModel) highlightedStyledLine(lineIdx int) string {
	line := m.renderLines[lineIdx]
	if !m.selecting || !m.selectionDragged {
		return line.styled
	}
	start, end, ok := m.normalizedSelection()
	if !ok {
		return line.styled
	}
	if lineIdx < start.line || lineIdx > end.line || !line.selectable {
		return line.styled
	}
	contentLen := lineColumns(line.content)
	var startCol, endCol int
	if lineIdx == start.line {
		startCol = line.contentX + start.column
	} else {
		startCol = line.contentX
	}
	if lineIdx == end.line {
		endCol = line.contentX + end.column + 1
	} else {
		endCol = line.contentX + contentLen
	}
	return insertANSIHighlight(line.styled, startCol, endCol)
}

// syncHighlight rebuilds the viewport content from the current renderLines, applying
// any active drag-selection highlight, and sets it on the viewport while preserving
// the current scroll offset.
func (m *TimelineModel) syncHighlight() {
	currentOffset := m.viewport.YOffset
	lines := make([]string, len(m.renderLines))
	for i := range m.renderLines {
		lines[i] = m.highlightedStyledLine(i)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	maxOffset := m.maxViewportOffset()
	if m.autoScroll || currentOffset >= maxOffset {
		m.viewport.GotoBottom()
		m.autoScroll = true
		return
	}
	m.viewport.YOffset = clamp(currentOffset, 0, maxOffset)
}

func (m *TimelineModel) appendRenderedBlock(styled string, meta []timelineRenderLine, addSpacer bool) {
	styledLines := strings.Split(styled, "\n")
	plainLines := strings.Split(stripANSI(styled), "\n")
	for i := range plainLines {
		line := ""
		if i < len(styledLines) {
			line = styledLines[i]
		}
		lineMeta := timelineRenderLine{plain: plainLines[i], content: plainLines[i]}
		if i < len(meta) {
			lineMeta = meta[i]
			if lineMeta.plain == "" {
				lineMeta.plain = plainLines[i]
			}
			if lineMeta.content == "" {
				lineMeta.content = plainLines[i]
			}
		}
		m.renderLines = append(m.renderLines, timelineRenderLine{
			styled:     line,
			plain:      lineMeta.plain,
			content:    lineMeta.content,
			contentX:   lineMeta.contentX,
			selectable: lineMeta.selectable,
		})
	}
	if addSpacer {
		m.renderLines = append(m.renderLines, timelineRenderLine{plain: "", content: "", selectable: false})
	}
}

func (m *TimelineModel) renderLineMetadata(idx int) []timelineRenderLine {
	if idx < 2 {
		plainLines := strings.Split(stripANSI(m.messages[idx]), "\n")
		meta := make([]timelineRenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, timelineRenderLine{plain: line, content: line, selectable: false})
		}
		return meta
	}
	entryIdx := idx - 2
	if entryIdx < 0 || entryIdx >= len(m.entries) {
		return nil
	}
	entry := m.entries[entryIdx]
	switch entry.Kind {
	case app.EntryAssistant, app.EntryReasoning:
		wrapped := strings.Split(wrapText(entry.Text, m.assistantInnerWidth()), "\n")
		meta := make([]timelineRenderLine, 0, len(wrapped))
		for _, line := range wrapped {
			meta = append(meta, timelineRenderLine{content: line, contentX: assistantContentX, selectable: true})
		}
		return meta
	case app.EntryUser:
		body := " >  " + entry.Text
		return []timelineRenderLine{
			{content: strings.Repeat("─", max(20, m.viewport.Width)), selectable: false},
			{content: body, contentX: userContentX, selectable: true},
			{content: strings.Repeat("─", max(20, m.viewport.Width)), selectable: false},
		}
	case app.EntryCommand, app.EntryOutput, app.EntryError, app.EntrySystem, app.EntryHelp:
		plainLines := strings.Split(stripANSI(m.messages[idx]), "\n")
		meta := make([]timelineRenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, timelineRenderLine{content: line, selectable: true})
		}
		return meta
	default:
		plainLines := strings.Split(stripANSI(m.messages[idx]), "\n")
		meta := make([]timelineRenderLine, 0, len(plainLines))
		for _, line := range plainLines {
			meta = append(meta, timelineRenderLine{content: line, selectable: false})
		}
		return meta
	}
}

func (m *TimelineModel) positionAt(x, y int) (timelineTextPos, bool) {
	if len(m.renderLines) == 0 || m.viewport.Height <= 0 {
		return timelineTextPos{}, false
	}
	lineIdx := clamp(y+m.viewport.YOffset, 0, len(m.renderLines)-1)
	line := m.renderLines[lineIdx]
	if !line.selectable {
		return timelineTextPos{}, false
	}
	relX := max(0, x-line.contentX)
	return timelineTextPos{line: lineIdx, column: columnAt(line.content, relX)}, true
}

func (m *TimelineModel) selectedText() (string, bool) {
	start, end, ok := m.normalizedSelection()
	if !ok {
		return "", false
	}
	var parts []string
	for lineIdx := start.line; lineIdx <= end.line; lineIdx++ {
		line := m.renderLines[lineIdx]
		if !line.selectable {
			continue
		}
		segment := line.content
		switch {
		case start.line == end.line:
			segment = sliceColumns(line.content, start.column, end.column+1)
		case lineIdx == start.line:
			segment = sliceColumns(line.content, start.column, lineColumns(line.content))
		case lineIdx == end.line:
			segment = sliceColumns(line.content, 0, end.column+1)
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

func (m *TimelineModel) normalizedSelection() (timelineTextPos, timelineTextPos, bool) {
	if len(m.renderLines) == 0 {
		return timelineTextPos{}, timelineTextPos{}, false
	}
	start := m.selectionAnchor
	end := m.selectionFocus
	if end.line < start.line || (end.line == start.line && end.column < start.column) {
		start, end = end, start
	}
	if start.line == end.line && start.column == end.column {
		return timelineTextPos{}, timelineTextPos{}, false
	}
	return start, end, true
}

func columnAt(line string, x int) int {
	if x <= 0 {
		return 0
	}
	width := 0
	for _, r := range line {
		rw := runewidth.RuneWidth(r)
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

func lineColumns(line string) int {
	return runewidth.StringWidth(line)
}

func sliceColumns(line string, start, end int) string {
	start = max(0, start)
	end = max(start, end)
	current := 0
	var out []rune
	for _, r := range line {
		rw := runewidth.RuneWidth(r)
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

// MessageAtY returns the index of the message at the given Y coordinate,
// relative to the viewport's top.
func (m *TimelineModel) MessageAtY(y int) int {
	if y < 0 || y >= m.viewport.Height {
		return -1
	}

	// Recalculate positions based on rendered content
	currentY := -m.viewport.YOffset
	
	// Pre-rendered entries (Logo + Welcome message)
	logoHeight := strings.Count(logoArt, "\n") + 1
	if y >= currentY && y < currentY+logoHeight {
		return -1 // Don't copy logo
	}
	currentY += logoHeight + 2

	welcomeMsg := "Motoko online. /provider add opens the configuration form; /models lists or selects models."
	welcomeHeight := strings.Count(wrapText(welcomeMsg, m.viewport.Width), "\n") + 1
	if y >= currentY && y < currentY+welcomeHeight {
		return -1 // Don't copy welcome
	}
	currentY += welcomeHeight + 2

	for i, entry := range m.entries {
		// Only copy useful entries
		copyable := entry.Kind == app.EntryAssistant || 
					entry.Kind == app.EntryReasoning ||
					entry.Kind == app.EntryUser || 
					entry.Kind == app.EntryOutput || 
					entry.Kind == app.EntryCommand ||
					entry.Kind == app.EntryError ||
					entry.Kind == app.EntrySystem ||
					entry.Kind == app.EntryHelp

		rendered := m.renderEntry(entry)
		height := strings.Count(rendered, "\n") + 1
		
		if y >= currentY && y < currentY+height {
			if !copyable {
				return -1
			}
			// Offset is 2 for logo + welcome message
			return i + 2
		}
		
		currentY += height + 2 // spacing
	}
	
	return -1
}

// renderUserMessage renders a user prompt between two thin horizontal rules.
func renderUserMessage(text string, width int) string {
	w := max(20, width)
	ruleStyle := lipgloss.NewStyle().Foreground(styles.AccentViolet)
	rule := ruleStyle.Render(strings.Repeat("─", w))
	body := " " + styles.UserPromptStyle.Render(">") + "  " +
		lipgloss.NewStyle().Foreground(styles.White).Render(text)
	return strings.Join([]string{rule, body, rule}, "\n")
}

// assistantOuterWidth returns the total width of the Assistant block.
func (m *TimelineModel) assistantOuterWidth() int {
	return max(40, m.viewport.Width)
}

// assistantInnerWidth returns the inner text width for AssistantBlockStyle rendering.
// AssistantBlockStyle has BorderLeft(1) + PaddingLeft(2) = 3 chars overhead.
func (m *TimelineModel) assistantInnerWidth() int {
	return max(37, m.assistantOuterWidth()-3)
}

// renderHelpEntry renders the /help output with colourised sections.
// Expected format: first line = title, subsequent lines = "/cmd   description"
// or "!<cmd>   description".
func renderHelpEntry(text string) string {
	lines := strings.Split(text, "\n")
	titleStyle := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true)
	cmdStyle := lipgloss.NewStyle().Foreground(styles.AccentBlue).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(styles.Gray)

	var rendered []string
	for i, line := range lines {
		if i == 0 {
			rendered = append(rendered, titleStyle.Render(line))
			continue
		}
		if line == "" {
			rendered = append(rendered, "")
			continue
		}
		// Split command name from description at first double-space run.
		idx := strings.Index(line, "  ")
		if idx <= 0 {
			rendered = append(rendered, descStyle.Render(line))
			continue
		}
		cmd := line[:idx]
		desc := strings.TrimSpace(line[idx:])
		rendered = append(rendered, cmdStyle.Render(cmd)+"  "+descStyle.Render(desc))
	}
	return strings.Join(rendered, "\n")
}
func renderDiffOutput(text string) string {
	lines := strings.Split(text, "\n")
	isDiff := false
	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "@@ ") {
			isDiff = true
			break
		}
	}
	if !isDiff {
		return styles.OutputStyle.Render(text)
	}

	changedCount := 0
	for _, line := range lines {
		if len(line) > 0 {
			if line[0] == '+' && !strings.HasPrefix(line, "+++ ") {
				changedCount++
			} else if line[0] == '-' && !strings.HasPrefix(line, "--- ") {
				changedCount++
			}
		}
	}

	if changedCount > 20 {
		var result []string
		for _, line := range lines {
			if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
				result = append(result, styles.DiffMetaStyle.Render(line))
			} else if strings.HasPrefix(line, "@@ ") {
				result = append(result, styles.DiffHeaderStyle.Render(line))
			}
		}
		result = append(result, styles.DiffMetaStyle.Render(fmt.Sprintf("... (%d lines changed, collapsed)", changedCount)))
		return strings.Join(result, "\n")
	}

	var result []string
	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			result = append(result, styles.DiffMetaStyle.Render(line))
		} else if strings.HasPrefix(line, "@@ ") {
			result = append(result, styles.DiffHeaderStyle.Render(line))
		} else if len(line) > 0 && line[0] == '+' {
			result = append(result, styles.DiffAddStyle.Render(line))
		} else if len(line) > 0 && line[0] == '-' {
			result = append(result, styles.DiffRemoveStyle.Render(line))
		} else {
			result = append(result, styles.DiffContextStyle.Render(line))
		}
	}
	return strings.Join(result, "\n")
}
