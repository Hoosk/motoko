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
	m.providerForm = providerForm{active: true, status: "Paso 1: elige provider. Paso 2: pega API key. Paso 3: Save. Luego usa /models para escoger modelo."}
}

func (m *Model) currentProviderKind() config.ProviderKind {
	kinds := m.runtime.ProviderKinds()
	if len(kinds) == 0 {
		return config.ProviderOpenAI
	}
	if m.providerForm.kindIndex < 0 || m.providerForm.kindIndex >= len(kinds) {
		return kinds[0]
	}
	return kinds[m.providerForm.kindIndex]
}

func (m *Model) providerConfigFromForm() config.ProviderConfig {
	kind := m.currentProviderKind()
	return config.ProviderConfig{Name: string(kind), Kind: kind, BaseURL: config.DefaultBaseURL(kind), APIKey: strings.TrimSpace(m.providerForm.apiKey)}
}

func (m *Model) providerFieldCount() int { return 3 }

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
			kinds := m.runtime.ProviderKinds()
			m.providerForm.kindIndex--
			if m.providerForm.kindIndex < 0 {
				m.providerForm.kindIndex = len(kinds) - 1
			}
		}
		return nil
	case "right":
		if m.providerForm.fieldIndex == 0 {
			kinds := m.runtime.ProviderKinds()
			m.providerForm.kindIndex = (m.providerForm.kindIndex + 1) % len(kinds)
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
		if m.providerForm.fieldIndex == 1 && len(msg.Runes) > 0 {
			m.providerForm.apiKey += string(msg.Runes)
		}
		return nil
	}
}

func (m *Model) handleProviderFormEnter() tea.Cmd {
	if m.providerForm.fieldIndex != 2 {
		m.providerForm.fieldIndex = (m.providerForm.fieldIndex + 1) % m.providerFieldCount()
		return nil
	}
	cfg := m.providerConfigFromForm()
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
		renderProviderField(0, m.providerForm.fieldIndex, "Provider", string(m.currentProviderKind())+"  (left/right)"),
		renderProviderField(1, m.providerForm.fieldIndex, "API Key", maskSecret(m.providerForm.apiKey)),
		renderProviderField(2, m.providerForm.fieldIndex, "Save", buttonLabel(m.providerForm.loading, "guardar y conectar")),
	}
	return strings.Join([]string{styles.PopupTitleStyle.Render("Add Provider"), styles.PopupMutedStyle.Render("Paso 1: elige provider. Paso 2: pega API key. Paso 3: Save. Luego usa /models para escoger modelo."), strings.Join(fields, "\n"), "", styles.SystemStyle.Render(m.providerForm.status)}, "\n")
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
