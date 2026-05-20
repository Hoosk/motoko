package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	modelPickerStepModel    = 0
	modelPickerStepThinking = 1
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
	m.modelList = make([]provider.ModelInfo, 0, len(active.Models))
	for _, modelID := range active.Models {
		m.modelList = append(m.modelList, provider.ModelInfo{ID: modelID})
	}
	m.modelListIndex = 0
	// Highlight current model.
	for i, model := range m.modelList {
		if strings.EqualFold(model.ID, active.Model) {
			m.modelListIndex = i
			break
		}
	}
	// Init thinking level index from current config.
	m.thinkingLevelIndex = budgetToLevelIndex(active.ThinkingBudget)
	m.modelPickerStep = modelPickerStepModel
	m.modelPickerLoading = true
	m.modelPickerOpen = true
}

// budgetToLevelIndex returns the index into ThinkingBudgetLevels for a given budget.
func budgetToLevelIndex(budget int) int {
	for i, v := range app.ThinkingBudgetLevels {
		if v == budget {
			return i
		}
	}
	return 0 // default: off
}

// pickerSupportsThinking reports whether the active provider + model support
// thinking level control.
func pickerSupportsThinking(active config.ProviderConfig) bool {
	switch active.Kind {
	case config.ProviderKindAnthropic:
		return true
	case config.ProviderKindGemini:
		return true
	case config.ProviderKindOpenAICompatible:
		// Legacy o-series and current gpt-5.x reasoning models support reasoning_effort.
		lower := strings.ToLower(active.Model)
		return strings.HasPrefix(lower, "o1") ||
			strings.HasPrefix(lower, "o3") ||
			strings.HasPrefix(lower, "o4") ||
			strings.HasPrefix(lower, "gpt-5")
	}
	return false
}

func (m *Model) handleModelPickerKey(msg tea.KeyMsg) tea.Cmd {
	switch m.modelPickerStep {
	case modelPickerStepModel:
		return m.handleModelPickerStepModel(msg)
	case modelPickerStepThinking:
		return m.handleModelPickerStepThinking(msg)
	}
	return nil
}

func (m *Model) handleModelPickerStepModel(msg tea.KeyMsg) tea.Cmd {
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
		// Apply model selection immediately so pickerSupportsThinking can use it.
		active, _ := m.runtime.GetActiveProviderConfig()
		active.Model = chosen.ID
		if pickerSupportsThinking(active) {
			// Advance to thinking level step.
			m.modelPickerStep = modelPickerStepThinking
			return nil
		}
		// No thinking support: confirm and close.
		return m.confirmModelSelection(chosen)
	}
	return nil
}

func (m *Model) handleModelPickerStepThinking(msg tea.KeyMsg) tea.Cmd {
	levels := app.ThinkingBudgetLevels
	switch msg.String() {
	case "esc":
		// Go back to model selection step.
		m.modelPickerStep = modelPickerStepModel
		return nil
	case "up", "ctrl+p":
		m.thinkingLevelIndex--
		if m.thinkingLevelIndex < 0 {
			m.thinkingLevelIndex = len(levels) - 1
		}
		return nil
	case "down", "ctrl+n", "tab":
		m.thinkingLevelIndex = (m.thinkingLevelIndex + 1) % len(levels)
		return nil
	case "enter":
		chosen := m.modelList[m.modelListIndex]
		budget := levels[m.thinkingLevelIndex]
		// Apply thinking budget first, then model.
		if err := m.runtime.SetThinkingBudget(budget); err != nil {
			m.modelPickerOpen = false
			m.modelPickerLoading = false
			return func() tea.Msg {
				return ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryError, Text: err.Error()}}}}
			}
		}
		return m.confirmModelSelection(chosen)
	}
	return nil
}

// confirmModelSelection applies the chosen model and closes the picker.
func (m *Model) confirmModelSelection(chosen provider.ModelInfo) tea.Cmd {
	m.modelPickerOpen = false
	m.modelPickerLoading = false
	if err := m.runtime.SetActiveModelInfo(chosen); err != nil {
		return func() tea.Msg {
			return ResponseAppliedMsg{Response: app.Response{Entries: []app.Entry{{Kind: app.EntryError, Text: err.Error()}}}}
		}
	}
	active, _ := m.runtime.GetActiveProviderConfig()
	return func() tea.Msg {
		return ModelChangedMsg{Provider: active.Name, Model: chosen.ID}
	}
}

func (m Model) renderModelPicker() string {
	switch m.modelPickerStep {
	case modelPickerStepThinking:
		return m.renderThinkingPicker()
	default:
		return m.renderModelPickerStep()
	}
}

func (m Model) renderModelPickerStep() string {
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
		label := styles.PopupFieldLabelStyle.Render(model.ID)
		current := strings.EqualFold(model.ID, active.Model)
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

func (m Model) renderThinkingPicker() string {
	titleStyle := lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(styles.Gray)
	accentStyle := lipgloss.NewStyle().Foreground(styles.AccentBlue)

	chosen := ""
	if len(m.modelList) > 0 {
		chosen = m.modelList[m.modelListIndex].ID
	}

	rows := []string{
		titleStyle.Render("Nivel de razonamiento"),
		accentStyle.Render(chosen),
		hintStyle.Render("↑↓ navega  Enter confirma  Esc volver"),
		"",
	}

	labels := app.ThinkingBudgetLabels
	for i, label := range labels {
		entry := styles.PopupFieldLabelStyle.Render(label)
		if i == m.thinkingLevelIndex {
			rows = append(rows, styles.PopupSelectionStyle.Render(entry))
		} else {
			rows = append(rows, entry)
		}
	}
	return strings.Join(rows, "\n")
}
