package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type providerForm struct {
	active      bool
	fieldIndex  int
	presetIndex int
	name        string
	baseURL     string
	apiKey      string
	loading     bool
	status      string
}

func (f *providerForm) Open(runtime *app.Runtime) {
	preset := f.currentProviderPreset(runtime)
	f.active = true
	f.name = config.DefaultProviderName(preset)
	f.baseURL = config.DefaultBaseURL(preset, "")
	f.presetIndex = 0
	f.fieldIndex = 0
}

func (f *providerForm) Update(msg tea.Msg, runtime *app.Runtime) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !f.active {
			return nil
		}
		switch msg.String() {
		case "esc":
			f.active = false
			return nil
		case "tab", "down", "ctrl+n":
			f.fieldIndex = (f.fieldIndex + 1) % f.fieldCount()
			return nil
		case "up", "ctrl+p":
			f.fieldIndex--
			if f.fieldIndex < 0 {
				f.fieldIndex = f.fieldCount() - 1
			}
			return nil
		case "left":
			if f.fieldIndex == 0 {
				presets := runtime.ProviderPresets()
				f.presetIndex--
				if f.presetIndex < 0 {
					f.presetIndex = len(presets) - 1
				}
				f.syncPreset(runtime)
			}
			return nil
		case "right":
			if f.fieldIndex == 0 {
				presets := runtime.ProviderPresets()
				f.presetIndex = (f.presetIndex + 1) % len(presets)
				f.syncPreset(runtime)
			}
			return nil
		case "backspace":
			if f.fieldIndex == 1 {
				f.apiKey = trimLastRune(f.apiKey)
			}
			return nil
		case "enter":
			return f.handleEnter(runtime)
		default:
			if len(msg.Runes) == 0 {
				return nil
			}
			if f.fieldIndex == 1 {
				f.apiKey += string(msg.Runes)
			}
			return nil
		}
	}
	return nil
}

func (f *providerForm) View(runtime *app.Runtime) string {
	if !f.active {
		return ""
	}
	preset := f.currentProviderPreset(runtime)
	presetLine := renderProviderField(0, f.fieldIndex,
		"Provider", string(preset)+"  ◀ ▶")
	apiKeyLine := renderProviderField(1, f.fieldIndex,
		"API Key", maskSecret(f.apiKey))

	saveBtn := renderProviderButton(2, f.fieldIndex, buttonLabel(f.loading, "save"))
	cancelBtn := renderProviderButton(3, f.fieldIndex, "cancel")
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
		styles.SystemStyle.Render(f.status),
	}, "\n")
}

func (f *providerForm) fieldCount() int { return 4 }

func (f *providerForm) currentProviderPreset(runtime *app.Runtime) config.ProviderPreset {
	presets := runtime.ProviderPresets()
	if len(presets) == 0 {
		return config.ProviderPresetOpenAI
	}
	if f.presetIndex < 0 || f.presetIndex >= len(presets) {
		return presets[0]
	}
	return presets[f.presetIndex]
}

func (f *providerForm) configFromForm(runtime *app.Runtime) config.ProviderConfig {
	preset := f.currentProviderPreset(runtime)
	return config.NormalizeProvider(config.ProviderConfig{
		Name:    config.DefaultProviderName(preset),
		Preset:  preset,
		BaseURL: config.DefaultBaseURL(preset, ""),
		APIKey:  strings.TrimSpace(f.apiKey),
	})
}

func (f *providerForm) handleEnter(runtime *app.Runtime) tea.Cmd {
	switch f.fieldIndex {
	case 3: // Cancel
		f.active = false
		return nil
	case 2: // Save
		cfg := f.configFromForm(runtime)
		if strings.TrimSpace(cfg.APIKey) == "" {
			f.status = "API Key is required."
			return nil
		}
		if err := runtime.SaveProvider(cfg, true); err != nil {
			f.status = err.Error()
			return nil
		}
		f.active = false
		return tea.Batch(
			func() tea.Msg { return NotificationMsg{Text: "Provider saved and activated: " + cfg.Name} },
			loadProviderModels(runtime, cfg),
		)
	default:
		f.fieldIndex = (f.fieldIndex + 1) % f.fieldCount()
		return nil
	}
}

func (f *providerForm) syncPreset(runtime *app.Runtime) {
	preset := f.currentProviderPreset(runtime)
	f.name = config.DefaultProviderName(preset)
	f.baseURL = config.DefaultBaseURL(preset, "")
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
