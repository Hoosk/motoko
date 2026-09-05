package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type approvalBarState struct {
	pending *tools.Pending
	active  bool
	sel     int
}

func (b *approvalBarState) Open(pending *tools.Pending) {
	b.pending = pending
	b.active = pending != nil
	b.sel = 0
}

func (b *approvalBarState) Clear() {
	b.pending = nil
	b.active = false
	b.sel = 0
}

func (b *approvalBarState) Update(msg tea.Msg) (done, approved bool) {
	if !b.active || b.pending == nil {
		return false, false
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, false
	}
	switch key.String() {
	case keyLeft, keyRight, keyTab:
		b.sel = 1 - b.sel
	case keyEnter:
		return true, b.sel == 0
	case "y", "Y":
		return true, true
	case keyEsc, "n", "N":
		return true, false
	}
	return false, false
}

func (b approvalBarState) View(width int) string {
	if !b.active || b.pending == nil || width <= 0 {
		return ""
	}
	contentWidth := max(width-4, 0)
	lines := []string{b.headerLine(contentWidth), b.buttonsLine(contentWidth)}
	body := styles.InputChromeStyle.Width(contentWidth).Render(strings.Join(lines, "\n"))
	separator := styles.GrayStyle.Width(contentWidth).Render(strings.Repeat("─", contentWidth))
	return lipgloss.JoinVertical(lipgloss.Left, separator, body)
}

func (b approvalBarState) headerLine(contentWidth int) string {
	title, label := b.titleAndLabel()
	line := styles.PopupTitleStyle.Render(title) + " " + styles.PopupFieldLabelStyle.Render(label)
	if lipgloss.Width(line) > contentWidth {
		line = truncateANSI(line, contentWidth)
	}
	return line
}

func (b approvalBarState) titleAndLabel() (string, string) {
	if b.pending == nil {
		return "Approve", ""
	}
	if b.pending.Kind == tools.DialogShellCommand {
		return "Approve shell command", "$ " + strings.TrimSpace(b.pending.ShellCommand.Command)
	}
	return "Approve file change", strings.TrimSpace(b.pending.Change.Path)
}

func (b approvalBarState) buttonsLine(contentWidth int) string {
	approve := "[ approve ]"
	reject := "[ reject ]"
	var approveStyled, rejectStyled string
	if b.sel == 0 {
		approveStyled = styles.PopupSelectionStyle.Render(approve)
		rejectStyled = styles.PopupMutedStyle.Render(reject)
	} else {
		approveStyled = styles.PopupMutedStyle.Render(approve)
		rejectStyled = styles.PopupSelectionStyle.Render(reject)
	}
	hint := styles.GrayStyle.Render("←/→ select · Enter confirm · y approve · n reject")
	line := approveStyled + "  " + rejectStyled + "   " + hint
	if lipgloss.Width(line) > contentWidth {
		line = truncateANSI(line, contentWidth)
	}
	return line
}

func (m Model) approvalBarHeight(width int) int {
	if !m.approvalBar.active || width <= 0 {
		return 0
	}
	return lipgloss.Height(m.approvalBar.View(width))
}

func (m *Model) appendApprovalContent(pending *tools.Pending) {
	if pending == nil {
		return
	}
	if pending.Kind == tools.DialogShellCommand {
		m.timeline.appendEntry(app.Entry{Kind: app.EntryCommand, Text: "$ " + strings.TrimSpace(pending.ShellCommand.Command)})
		if reason := strings.TrimSpace(pending.ShellCommand.Reason); reason != "" {
			m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: reason})
		}
	} else {
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Approval requested: " + strings.TrimSpace(pending.Change.Path)})
		m.timeline.appendEntry(app.Entry{Kind: app.EntryOutput, Text: pending.Change.Diff})
	}
	m.timeline.renderMessages()
}

func (m *Model) resolveDialog(approved bool) tea.Cmd {
	pending := m.approvalBar.pending
	m.approvalBar.Clear()
	if pending != nil {
		pending.Resolve(tools.DialogDecision{Approved: approved})
	}
	m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: approvalResultText(pending, approved)})
	m.timeline.renderMessages()
	m.sidebar.dirty = true
	return m.waitDialog()
}

func approvalResultText(pending *tools.Pending, approved bool) string {
	verb := "Rejected"
	if approved {
		verb = "Approved"
	}
	if pending == nil {
		return verb + " pending dialog."
	}
	if pending.Kind == tools.DialogShellCommand {
		return fmt.Sprintf("%s shell command: %s.", verb, strings.TrimSpace(pending.ShellCommand.Command))
	}
	return fmt.Sprintf("%s file change: %s.", verb, strings.TrimSpace(pending.Change.Path))
}

func (m *Model) clearExpiredDialog() tea.Cmd {
	cleared := false
	if m.approvalBar.active && m.approvalBar.pending.Resolved() {
		m.approvalBar.Clear()
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Approval expired without a decision."})
		m.timeline.renderMessages()
		m.sidebar.dirty = true
		cleared = true
	}
	if m.questionPopup.active && m.questionPopup.pending.Resolved() {
		m.questionPopup.active = false
		m.questionPopup.pending = nil
		m.sidebar.dirty = true
		cleared = true
	}
	if cleared {
		return m.waitDialog()
	}
	return nil
}
