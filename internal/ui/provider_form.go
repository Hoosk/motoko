package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openProviderForm() {
	preset := m.currentProviderPreset()
	m.providerForm = providerForm{
		active:  true,
		name:    config.DefaultProviderName(preset),
		baseURL: config.DefaultBaseURL(preset, ""),
		status:  "Configura nombre, preset, base URL y API key. Luego guarda y usa /models para elegir modelo.",
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
		Name:    strings.TrimSpace(m.providerForm.name),
		Preset:  preset,
		BaseURL: strings.TrimSpace(m.providerForm.baseURL),
		APIKey:  strings.TrimSpace(m.providerForm.apiKey),
	})
}

func (m *Model) providerFieldCount() int { return 5 }

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
		switch m.providerForm.fieldIndex {
		case 1:
			m.providerForm.name = trimLastRune(m.providerForm.name)
		case 2:
			m.providerForm.baseURL = trimLastRune(m.providerForm.baseURL)
		case 3:
			m.providerForm.apiKey = trimLastRune(m.providerForm.apiKey)
		}
		return nil
	case "enter":
		return m.handleProviderFormEnter()
	default:
		if len(msg.Runes) == 0 {
			return nil
		}
		switch m.providerForm.fieldIndex {
		case 1:
			m.providerForm.name += string(msg.Runes)
		case 2:
			m.providerForm.baseURL += string(msg.Runes)
		case 3:
			m.providerForm.apiKey += string(msg.Runes)
		}
		return nil
	}
}

func (m *Model) handleProviderFormEnter() tea.Cmd {
	if m.providerForm.fieldIndex != m.providerFieldCount()-1 {
		m.providerForm.fieldIndex = (m.providerForm.fieldIndex + 1) % m.providerFieldCount()
		return nil
	}
	cfg := m.providerConfigFromForm()
	if strings.TrimSpace(cfg.Name) == "" {
		m.providerForm.status = "El nombre del provider es obligatorio."
		return nil
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		m.providerForm.status = "La base URL es obligatoria."
		return nil
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		m.providerForm.status = "La API key es obligatoria."
		return nil
	}
	if err := m.runtime.SaveProvider(cfg, true); err != nil {
		m.providerForm.status = err.Error()
		return nil
	}
	m.providerForm = providerForm{}
	m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: fmt.Sprintf("Provider guardado y activado: %s", cfg.Name)})
	m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Cargando modelos del provider activo en background... luego usa /models para listarlos o /models <modelo> para elegir uno."})
	m.timeline.renderMessages()
	return loadProviderModels(m.runtime, cfg)
}

func (m Model) renderProviderForm() string {
	fields := []string{
		renderProviderField(0, m.providerForm.fieldIndex, "Preset", string(m.currentProviderPreset())+"  (left/right)"),
		renderProviderField(1, m.providerForm.fieldIndex, "Name", m.providerForm.name),
		renderProviderField(2, m.providerForm.fieldIndex, "Base URL", m.providerForm.baseURL),
		renderProviderField(3, m.providerForm.fieldIndex, "API Key", maskSecret(m.providerForm.apiKey)),
		renderProviderField(4, m.providerForm.fieldIndex, "Save", buttonLabel(m.providerForm.loading, "guardar y conectar")),
	}
	return strings.Join([]string{styles.PopupTitleStyle.Render("Add Provider"), styles.PopupMutedStyle.Render("El preset define la familia de API y el base URL inicial. Puedes editar nombre y URL antes de guardar."), strings.Join(fields, "\n"), "", styles.SystemStyle.Render(m.providerForm.status)}, "\n")
}

func (m *Model) syncProviderFormPreset() {
	preset := m.currentProviderPreset()
	if strings.TrimSpace(m.providerForm.name) == "" || m.providerForm.name == config.DefaultProviderName(config.ProviderPresetOpenAI) || m.providerForm.name == config.DefaultProviderName(config.ProviderPresetOpenRouter) || m.providerForm.name == config.DefaultProviderName(config.ProviderPresetAnthropic) || m.providerForm.name == config.DefaultProviderName(config.ProviderPresetGemini) {
		m.providerForm.name = config.DefaultProviderName(preset)
	}
	m.providerForm.baseURL = config.DefaultBaseURL(preset, "")
}

func renderProviderField(index, active int, label, value string) string {
	line := styles.PopupFieldLabelStyle.Render(label+": ") + styles.PopupFieldValueStyle.Render(value)
	if index == active {
		return styles.PopupSelectionStyle.Render(line)
	}
	return line
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
		return "cargando..."
	}
	return text
}
