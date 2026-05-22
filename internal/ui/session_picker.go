package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openSessionPicker() {
	m.sessionList = nil
	m.sessionListIndex = 0
	m.sessionLoading = true
	m.sessionPickerOpen = true
}

func (m *Model) handleSessionPickerKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.sessionPickerOpen = false
		m.sessionLoading = false
		return nil
	case "up", "ctrl+p":
		if len(m.sessionList) > 0 {
			m.sessionListIndex--
			if m.sessionListIndex < 0 {
				m.sessionListIndex = len(m.sessionList) - 1
			}
		}
		return nil
	case "down", "ctrl+n", "tab":
		if len(m.sessionList) > 0 {
			m.sessionListIndex = (m.sessionListIndex + 1) % len(m.sessionList)
		}
		return nil
	case "enter":
		if len(m.sessionList) == 0 {
			m.sessionPickerOpen = false
			return nil
		}
		chosen := m.sessionList[m.sessionListIndex]
		m.sessionPickerOpen = false
		m.sessionLoading = false
		return func() tea.Msg {
			if err := m.runtime.LoadSession(chosen.ID); err != nil {
				return SessionLoadedMsg{Err: err}
			}
			return SessionLoadedMsg{Session: chosen}
		}
	}
	return nil
}

func (m Model) renderSessionPicker() string {
	rows := []string{
		styles.PopupTitleStyle.Render("Sesiones"),
		styles.PopupMutedStyle.Render("↑↓ navega  Enter carga  Esc cancela"),
		"",
	}
	if m.sessionLoading && len(m.sessionList) == 0 {
		rows = append(rows, styles.PopupMutedStyle.Render("Cargando sesiones..."))
		return strings.Join(rows, "\n")
	}
	if len(m.sessionList) == 0 {
		rows = append(rows, styles.PopupMutedStyle.Render("No hay sesiones guardadas para este workspace."))
		return strings.Join(rows, "\n")
	}
	for i, s := range m.sessionList {
		line := fmt.Sprintf("%s  %s  (%d mensajes)", s.Title, s.UpdatedAt.Format("2006-01-02 15:04"), len(s.History))
		if i == m.sessionListIndex {
			rows = append(rows, styles.PopupSelectionStyle.Render(line))
		} else {
			rows = append(rows, styles.PopupFieldLabelStyle.Render(line))
		}
	}
	return strings.Join(rows, "\n")
}
