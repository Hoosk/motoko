package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Fields: 0=Preset  1=APIKey  2=Save  3=Cancel
func (m *Model) providerFieldCount() int { return 4 }

func (m *Model) openProviderForm() {
	preset := m.currentProviderPreset()
	m.providerForm = providerForm{
		active:  true,
		name:    config.DefaultProviderName(preset),
		baseURL: config.DefaultBaseURL(preset, ""),
	}
}

func (m *Model) currentProviderPreset() config.ProviderPreset {
	presets := m.runtime.ProviderPresets()
	if len(presets) == 0 {
		return config.ProviderPresetOpenAI
	}
	if m.providerForm.presetIndex < 0 || m.providerForm.presetIndex >= len(presets) {
		return presets[0]
	}
	return presets[m.providerForm.presetIndex]
}

func (m *Model) providerConfigFromForm() config.ProviderConfig {
	preset := m.currentProviderPreset()
	return config.NormalizeProvider(config.ProviderConfig{
		Name:    config.DefaultProviderName(preset),
		Preset:  preset,
		BaseURL: config.DefaultBaseURL(preset, ""),
		APIKey:  strings.TrimSpace(m.providerForm.apiKey),
	})
}

func (m *Model) handleProviderFormKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.providerForm = providerForm{}
		return nil
	case "tab", "down", "ctrl+n":
		m.providerForm.fieldIndex = (m.providerForm.fieldIndex + 1) % m.providerFieldCount()
		return nil
	case "up", "ctrl+p":
		m.providerForm.fieldIndex--
		if m.providerForm.fieldIndex < 0 {
			m.providerForm.fieldIndex = m.providerFieldCount() - 1
		}
		return nil
	case "left":
		if m.providerForm.fieldIndex == 0 {
			presets := m.runtime.ProviderPresets()
			m.providerForm.presetIndex--
			if m.providerForm.presetIndex < 0 {
				m.providerForm.presetIndex = len(presets) - 1
			}
			m.syncProviderFormPreset()
		}
		return nil
	case "right":
		if m.providerForm.fieldIndex == 0 {
			presets := m.runtime.ProviderPresets()
			m.providerForm.presetIndex = (m.providerForm.presetIndex + 1) % len(presets)
			m.syncProviderFormPreset()
		}
		return nil
	case "backspace":
		if m.providerForm.fieldIndex == 1 {
			m.providerForm.apiKey = trimLastRune(m.providerForm.apiKey)
		}
		return nil
	case "enter":
		return m.handleProviderFormEnter()
	default:
		if len(msg.Runes) == 0 {
			return nil
		}
		if m.providerForm.fieldIndex == 1 {
			m.providerForm.apiKey += string(msg.Runes)
		}
		return nil
	}
}

func (m *Model) handleProviderFormEnter() tea.Cmd {
	switch m.providerForm.fieldIndex {
	case 3: // Cancel
		m.providerForm = providerForm{}
		return nil
	case 2: // Save
		cfg := m.providerConfigFromForm()
		if strings.TrimSpace(cfg.APIKey) == "" {
			m.providerForm.status = "API Key is required."
			return nil
		}
		if err := m.runtime.SaveProvider(cfg, true); err != nil {
			m.providerForm.status = err.Error()
			return nil
		}
		m.providerForm = providerForm{}
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: fmt.Sprintf("Provider saved and activated: %s", cfg.Name)})
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Loading models in background... then use /models to list or select them."})
		m.timeline.renderMessages()
		return loadProviderModels(m.runtime, cfg)
	default:
		m.providerForm.fieldIndex = (m.providerForm.fieldIndex + 1) % m.providerFieldCount()
		return nil
	}
}

func (m Model) renderProviderForm() string {
	preset := m.currentProviderPreset()
	presetLine := renderProviderField(0, m.providerForm.fieldIndex,
		"Provider", string(preset)+"  ◀ ▶")
	apiKeyLine := renderProviderField(1, m.providerForm.fieldIndex,
		"API Key", maskSecret(m.providerForm.apiKey))

	saveBtn := renderProviderButton(2, m.providerForm.fieldIndex, buttonLabel(m.providerForm.loading, "save"))
	cancelBtn := renderProviderButton(3, m.providerForm.fieldIndex, "cancel")
	buttons := lipgloss.JoinHorizontal(lipgloss.Left, saveBtn, "   ", cancelBtn)

	return strings.Join([]string{
		styles.PopupTitleStyle.Render("Add Provider"),
		styles.PopupMutedStyle.Render("Select a provider and enter your API key."),
		"",
		presetLine,
		apiKeyLine,
		"",
		buttons,
		"",
		styles.SystemStyle.Render(m.providerForm.status),
	}, "\n")
}

func (m *Model) syncProviderFormPreset() {
	preset := m.currentProviderPreset()
	m.providerForm.name = config.DefaultProviderName(preset)
	m.providerForm.baseURL = config.DefaultBaseURL(preset, "")
}

func renderProviderField(index, active int, label, value string) string {
	line := styles.PopupFieldLabelStyle.Render(label+": ") + styles.PopupFieldValueStyle.Render(value)
	if index == active {
		return styles.PopupSelectionStyle.Render(line)
	}
	return line
}

func renderProviderButton(index, active int, label string) string {
	text := "[ " + label + " ]"
	if index == active {
		return styles.PopupSelectionStyle.Render(text)
	}
	return styles.PopupFieldLabelStyle.Render(text)
}

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(value)-4) + value[len(value)-4:]
}

func buttonLabel(loading bool, text string) string {
	if loading {
		return "loading..."
	}
	return text
}
