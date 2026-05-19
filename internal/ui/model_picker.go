package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// openModelPicker initialises and opens the model picker popup.
// If the active provider has cached models, those are used immediately.
// An async fetch is always dispatched so the list stays fresh.
func (m *Model) openModelPicker() {
	active, ok := m.runtime.GetActiveProviderConfig()
	if !ok {
		m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: "No hay provider activo."})
		m.timeline.renderMessages()
		return
	}
	m.modelList = append([]string(nil), active.Models...)
	m.modelListIndex = 0
	// Highlight current model.
	for i, model := range m.modelList {
		if strings.EqualFold(model, active.Model) {
			m.modelListIndex = i
			break
		}
	}
	m.modelPickerLoading = true
	m.modelPickerOpen = true
}

func (m *Model) handleModelPickerKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.modelPickerOpen = false
		m.modelPickerLoading = false
		return nil
	case "up", "ctrl+p":
		if len(m.modelList) > 0 {
			m.modelListIndex--
			if m.modelListIndex < 0 {
				m.modelListIndex = len(m.modelList) - 1
			}
		}
		return nil
	case "down", "ctrl+n", "tab":
		if len(m.modelList) > 0 {
			m.modelListIndex = (m.modelListIndex + 1) % len(m.modelList)
		}
		return nil
	case "enter":
		if len(m.modelList) == 0 {
			m.modelPickerOpen = false
			return nil
		}
		chosen := m.modelList[m.modelListIndex]
		m.modelPickerOpen = false
		m.modelPickerLoading = false
		if err := m.runtime.SetActiveModel(chosen); err != nil {
			return func() tea.Msg {
				return ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryError, Text: err.Error()}}}}
			}
		}
		active, _ := m.runtime.GetActiveProviderConfig()
		return func() tea.Msg {
			return ModelChangedMsg{Provider: active.Name, Model: chosen}
		}
	}
	return nil
}

func (m Model) renderModelPicker() string {
	titleStyle := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(styles.Gray)

	rows := []string{
		titleStyle.Render("Seleccionar modelo"),
		hintStyle.Render("↑↓ navega  Enter selecciona  Esc cancela"),
		"",
	}

	if m.modelPickerLoading && len(m.modelList) == 0 {
		rows = append(rows, hintStyle.Render("Cargando modelos..."))
		return strings.Join(rows, "\n")
	}
	if len(m.modelList) == 0 {
		rows = append(rows, hintStyle.Render("Sin modelos. Usa /models <nombre> para añadir uno."))
		return strings.Join(rows, "\n")
	}

	active, _ := m.runtime.GetActiveProviderConfig()
	for i, model := range m.modelList {
		label := styles.PopupFieldLabelStyle.Render(model)
		current := strings.EqualFold(model, active.Model)
		if current {
			label += "  " + hintStyle.Render("(activo)")
		}
		if i == m.modelListIndex {
			rows = append(rows, styles.PopupSelectionStyle.Render(label))
		} else {
			rows = append(rows, label)
		}
	}

	if m.modelPickerLoading {
		rows = append(rows, "", hintStyle.Render("Actualizando lista…"))
	}
	return strings.Join(rows, "\n")
}
