package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/app/commands"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type helpTab int

const (
	helpTabShortcuts helpTab = iota
	helpTabCommands
	helpTabTools
)

type helpOverlayState struct {
	active     bool
	tab        helpTab
	offset     int
	viewHeight int
}

func (h *helpOverlayState) Open() {
	h.active = true
	h.tab = helpTabShortcuts
	h.offset = 0
	if h.viewHeight == 0 {
		h.viewHeight = 14
	}
}

func (h *helpOverlayState) Update(msg tea.Msg) tea.Cmd {
	if !h.active {
		return nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.viewHeight = max(8, msg.Height-14)
	case tea.KeyMsg:
		switch msg.String() {
		case keyEsc, keyCtrlH:
			h.active = false
		case keyTab, keyRight:
			h.tab = (h.tab + 1) % 3
			h.offset = 0
		case keyShiftTab, keyLeft:
			h.tab = (h.tab + 2) % 3
			h.offset = 0
		case keyDown, keyCtrlN:
			h.offset++
		case keyUp, keyCtrlP:
			if h.offset > 0 {
				h.offset--
			}
		}
	}
	return nil
}

func (h helpOverlayState) View(runtime *app.Runtime) string {
	if !h.active {
		return ""
	}

	tabs := []string{
		h.renderTab("Shortcuts", h.tab == helpTabShortcuts),
		h.renderTab("Commands", h.tab == helpTabCommands),
		h.renderTab("Tools", h.tab == helpTabTools),
	}

	lines := h.activeLines(runtime)
	body := h.renderBody(lines)
	help := styles.PopupMutedStyle.Render("Tab switch tabs  ↑↓ scroll  Esc close")

	return strings.Join([]string{
		styles.PopupTitleStyle.Render("HELP"),
		strings.Join(tabs, "  "),
		help,
		"",
		body,
	}, "\n")
}

func (h helpOverlayState) renderTab(label string, active bool) string {
	if active {
		return styles.PopupSelectionStyle.Render("[" + label + "]")
	}
	return styles.PopupMutedStyle.Render(label)
}

func (h helpOverlayState) activeLines(runtime *app.Runtime) []string {
	switch h.tab {
	case helpTabCommands:
		defs := commands.CommandDefinitions()
		lines := make([]string, 0, len(defs)+2)
		for _, def := range defs {
			lines = append(lines, formatShortcut(def.Usage, def.Summary))
		}
		lines = append(lines, formatShortcut("!<cmd>", "explicit shell command"))
		lines = append(lines, formatShortcut("@<file|agent>", "mention file or agent"))
		return lines
	case helpTabTools:
		return strings.Split(renderToolList(runtime.ToolSpecs()), "\n")
	default:
		return []string{
			formatShortcut("enter", "send / select"),
			formatShortcut("tab/right", "next suggestion or tab"),
			formatShortcut("shift+tab/left", "previous suggestion or tab"),
			formatShortcut("up/down", "history, lists, scroll"),
			formatShortcut("ctrl+k", "command palette"),
			formatShortcut("ctrl+h", "toggle help overlay"),
			formatShortcut("ctrl+t", "toggle tool catalog"),
			formatShortcut("ctrl+m", "open model selector"),
			formatShortcut("ctrl+p", "open provider form"),
			formatShortcut("ctrl+o", "open session picker"),
			formatShortcut("ctrl+a", "open agent mode selector"),
			formatShortcut("ctrl+s/alt+s", "toggle sidebar"),
			formatShortcut("ctrl+q", "focus queued prompts"),
			formatShortcut("ctrl+up/down", "reorder queued prompts"),
			formatShortcut("alt+up/down", "select previous/next message"),
			formatShortcut("alt+c", "copy selected message"),
			formatShortcut("ctrl+r", "toggle reasoning visibility"),
			formatShortcut("esc", "close overlay or stop request"),
			formatShortcut("ctrl+c twice", "exit motoko"),
			formatShortcut("mouse drag", "select text and copy"),
		}
	}
}

func (h helpOverlayState) renderBody(lines []string) string {
	if len(lines) == 0 {
		return styles.PopupMutedStyle.Render("Nothing to show.")
	}
	maxOffset := max(0, len(lines)-h.viewHeight)
	offset := clamp(h.offset, maxOffset)
	end := min(len(lines), offset+h.viewHeight)
	visible := lines[offset:end]
	if offset > 0 {
		visible = append([]string{styles.PopupMutedStyle.Render("▲ more above")}, visible...)
	}
	if end < len(lines) {
		visible = append(visible, styles.PopupMutedStyle.Render(fmt.Sprintf("▼ %d more lines", len(lines)-end)))
	}
	return strings.Join(visible, "\n")
}
