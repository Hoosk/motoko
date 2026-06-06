package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type sessionPickerState struct {
	sessions []*session.Session
	index    int
	active   bool
	loading  bool
}

func (p *sessionPickerState) Open() {
	p.sessions = nil
	p.index = 0
	p.loading = true
	p.active = true
}

func (p *sessionPickerState) Update(msg tea.Msg, runtime *app.Runtime) tea.Cmd {
	switch msg := msg.(type) {
	case SessionsMsg:
		if msg.Err != nil {
			return func() tea.Msg { return ErrorMsg{Err: msg.Err} }
		}
		p.sessions = msg.Sessions
		p.active = true
		p.index = 0
		p.loading = false
		return nil

	case tea.KeyMsg:
		if !p.active {
			return nil
		}
		switch msg.String() {
		case "esc":
			p.active = false
			p.loading = false
			return nil
		case "up", "ctrl+p":
			if len(p.sessions) > 0 {
				p.index--
				if p.index < 0 {
					p.index = len(p.sessions) - 1
				}
			}
			return nil
		case "down", "ctrl+n", "tab":
			if len(p.sessions) > 0 {
				p.index = (p.index + 1) % len(p.sessions)
			}
			return nil
		case "enter":
			if len(p.sessions) == 0 {
				p.active = false
				return nil
			}
			chosen := p.sessions[p.index]
			p.active = false
			p.loading = false
			return func() tea.Msg {
				if err := runtime.LoadSession(chosen.ID); err != nil {
					return SessionLoadedMsg{Err: err}
				}
				return SessionLoadedMsg{Session: chosen}
			}
		}
	}
	return nil
}

func (p sessionPickerState) View() string {
	if !p.active {
		return ""
	}
	rows := []string{
		styles.PopupTitleStyle.Render("Sesiones"),
		styles.PopupMutedStyle.Render("↑↓ navega  Enter carga  Esc cancela"),
		"",
	}
	if p.loading && len(p.sessions) == 0 {
		rows = append(rows, styles.PopupMutedStyle.Render("Cargando sesiones..."))
		return strings.Join(rows, "\n")
	}
	if len(p.sessions) == 0 {
		rows = append(rows, styles.PopupMutedStyle.Render("No hay sesiones guardadas para este workspace."))
		return strings.Join(rows, "\n")
	}
	for i, s := range p.sessions {
		line := fmt.Sprintf("%s  %s  (%d mensajes)", s.Title, s.UpdatedAt.Format("2006-01-02 15:04"), len(s.History))
		if i == p.index {
			rows = append(rows, styles.PopupSelectionStyle.Render(line))
		} else {
			rows = append(rows, styles.PopupFieldLabelStyle.Render(line))
		}
	}
	return strings.Join(rows, "\n")
}
