package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/ui/timeline"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type approvalPopupState struct {
	pending  *tools.Pending
	viewport viewport.Model
	active   bool
	width    int
	height   int
}

func (p *approvalPopupState) Open(pending *tools.Pending, width, height int) {
	p.pending = pending
	p.active = pending != nil
	p.resize(width, height)
	p.viewport = viewport.New(p.width, p.height)
	if pending != nil {
		p.viewport.SetContent(approvalContent(pending))
	}
}

func (p *approvalPopupState) resize(width, height int) {
	if width <= 0 {
		width = 84
	}
	if height <= 0 {
		height = 24
	}
	p.width = max(24, min(width-16, 68))
	p.height = max(8, min(height-14, 18))
	if p.viewport.Width > 0 {
		p.viewport.Width = p.width
		p.viewport.Height = p.height
	}
}

func (p *approvalPopupState) Update(msg tea.Msg) bool {
	if !p.active || p.pending == nil {
		return false
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		p.resize(size.Width, size.Height)
		return false
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case keyEnter, "y", "Y":
			p.pending.Resolve(tools.DialogDecision{Approved: true})
			p.active = false
			return true
		case keyEsc, "n", "N":
			p.pending.Resolve(tools.DialogDecision{Approved: false})
			p.active = false
			return true
		}
	}
	p.viewport, _ = p.viewport.Update(msg)
	return false
}

func (p approvalPopupState) View() string {
	if !p.active || p.pending == nil {
		return ""
	}
	title := "Approve file change"
	label := strings.TrimSpace(p.pending.Change.Path)
	if p.pending.Kind == tools.DialogShellCommand {
		title = "Approve shell command"
		label = "$ " + strings.TrimSpace(p.pending.ShellCommand.Command)
	}
	if label == "" {
		label = "pending dialog"
	}
	rows := []string{
		styles.PopupTitleStyle.Render(title),
		styles.PopupFieldLabelStyle.Render(label),
		styles.PopupMutedStyle.Render("↑↓ scroll  Enter/y approve  Esc/n reject"),
		"",
		p.viewport.View(),
		"",
		lipgloss.JoinHorizontal(lipgloss.Left,
			styles.PopupSelectionStyle.Render("[approve]"),
			" ",
			styles.PopupMutedStyle.Render("[reject]"),
		),
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func approvalContent(pending *tools.Pending) string {
	if pending == nil {
		return ""
	}
	if pending.Kind == tools.DialogShellCommand {
		return strings.TrimSpace(fmt.Sprintf("$ %s\n\n%s", pending.ShellCommand.Command, pending.ShellCommand.Reason))
	}
	return timeline.RenderFullDiffOutput(pending.Change.Diff)
}
