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
	picker      *FilterList
	name        string
	baseURL     string
	apiKey      string
	status      string
	fieldIndex  int
	presetIndex int
	active      bool
	loading     bool
	showPicker  bool
}

type providerFilterItem struct {
	preset config.ProviderPreset
	name   string
}

func (p providerFilterItem) FilterKey() string {
	return string(p.preset) + " " + p.name
}

func (p providerFilterItem) Render(active bool) string {
	if active {
		return styles.PopupSelectionStyle.Render("> " + p.name + " (" + string(p.preset) + ")")
	}
	return styles.PopupFieldValueStyle.Render("  " + p.name + " (" + string(p.preset) + ")")
}

func (f *providerForm) Open(runtime *app.Runtime) {
	f.active = true
	f.status = ""

	presets := runtime.ProviderPresets()
	var items []FilterableItem
	for _, preset := range presets {
		name := ""
		if cp, ok := runtime.LookupCatalogProvider(string(preset)); ok {
			name = cp.Name
		} else {
			name = config.DefaultProviderName(preset)
		}
		items = append(items, providerFilterItem{
			preset: preset,
			name:   name,
		})
	}

	f.picker = NewFilterList("Select Provider", "Search...")
	f.picker.SetItems(items)
	f.picker.Active = true
	f.showPicker = true
	f.presetIndex = 0
	f.fieldIndex = 0
}

func (f *providerForm) isOpenAICompatible(runtime *app.Runtime) bool {
	preset := f.currentProviderPreset(runtime)
	switch preset {
	case config.ProviderPresetOpenAI, config.ProviderPresetAnthropic, config.ProviderPresetGemini:
		return false
	}
	return true
}

func (f *providerForm) fieldCount(runtime *app.Runtime) int {
	if f.isOpenAICompatible(runtime) {
		return 6
	}
	return 4
}

func (f *providerForm) Update(msg tea.Msg, runtime *app.Runtime) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !f.active {
			return nil
		}
		if f.showPicker {
			item, selected, cancelled := f.picker.Update(msg)
			if cancelled {
				f.active = false
				f.showPicker = false
				return nil
			}
			if selected {
				pItem := item.(providerFilterItem)
				presets := runtime.ProviderPresets()
				for i, p := range presets {
					if p == pItem.preset {
						f.presetIndex = i
						break
					}
				}
				f.showPicker = false
				f.syncPreset(runtime)
				f.fieldIndex = 1
			}
			return nil
		}
		switch msg.String() {
		case keyEsc:
			f.active = false
			return nil
		case keyTab, keyDown, keyCtrlN:
			f.fieldIndex = (f.fieldIndex + 1) % f.fieldCount(runtime)
			return nil
		case keyUp, keyCtrlP:
			f.fieldIndex--
			if f.fieldIndex < 0 {
				f.fieldIndex = f.fieldCount(runtime) - 1
			}
			return nil
		case "left":
			if f.fieldIndex == 0 {
				f.showPicker = true
				f.picker.Active = true
				return nil
			} else {
				saveIdx := 2
				cancelIdx := 3
				if f.isOpenAICompatible(runtime) {
					saveIdx = 4
					cancelIdx = 5
				}
				if f.fieldIndex == cancelIdx {
					f.fieldIndex = saveIdx
				}
			}
			return nil
		case "right":
			if f.fieldIndex == 0 {
				f.showPicker = true
				f.picker.Active = true
				return nil
			} else {
				saveIdx := 2
				cancelIdx := 3
				if f.isOpenAICompatible(runtime) {
					saveIdx = 4
					cancelIdx = 5
				}
				if f.fieldIndex == saveIdx {
					f.fieldIndex = cancelIdx
				}
			}
			return nil
		case "backspace":
			if f.isOpenAICompatible(runtime) {
				switch f.fieldIndex {
				case 1:
					f.name = trimLastRune(f.name)
				case 2:
					f.baseURL = trimLastRune(f.baseURL)
				case 3:
					f.apiKey = trimLastRune(f.apiKey)
				}
			} else {
				if f.fieldIndex == 1 {
					f.apiKey = trimLastRune(f.apiKey)
				}
			}
			return nil
		case keyEnter:
			if f.fieldIndex == 0 {
				f.showPicker = true
				f.picker.Active = true
				return nil
			}
			return f.handleEnter(runtime)
		default:
			if len(msg.Runes) == 0 {
				return nil
			}
			if f.isOpenAICompatible(runtime) {
				switch f.fieldIndex {
				case 1:
					f.name += string(msg.Runes)
				case 2:
					f.baseURL += string(msg.Runes)
				case 3:
					f.apiKey += string(msg.Runes)
				}
			} else {
				if f.fieldIndex == 1 {
					f.apiKey += string(msg.Runes)
				}
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
	if f.showPicker {
		return f.picker.View()
	}
	preset := f.currentProviderPreset(runtime)
	presetLine := renderProviderField(0, f.fieldIndex,
		"Provider", string(preset)+"  <Enter to change>")

	var lines []string
	lines = append(lines, styles.PopupTitleStyle.Render("Add Provider"))
	lines = append(lines, styles.PopupMutedStyle.Render("Select a provider and enter details."))
	lines = append(lines, "")
	lines = append(lines, presetLine)

	var saveBtn, cancelBtn string

	if f.isOpenAICompatible(runtime) {
		nameLine := renderProviderField(1, f.fieldIndex, "Name", f.name)
		urlLine := renderProviderField(2, f.fieldIndex, "Base URL", f.baseURL)
		apiKeyLine := renderProviderField(3, f.fieldIndex, "API Key", maskSecret(f.apiKey))
		lines = append(lines, nameLine, urlLine, apiKeyLine)

		saveBtn = renderProviderButton(4, f.fieldIndex, buttonLabel(f.loading, "save"))
		cancelBtn = renderProviderButton(5, f.fieldIndex, "cancel")
	} else {
		apiKeyLine := renderProviderField(1, f.fieldIndex, "API Key", maskSecret(f.apiKey))
		lines = append(lines, apiKeyLine)

		saveBtn = renderProviderButton(2, f.fieldIndex, buttonLabel(f.loading, "save"))
		cancelBtn = renderProviderButton(3, f.fieldIndex, "cancel")
	}

	lines = append(lines, "")
	buttons := lipgloss.JoinHorizontal(lipgloss.Left, saveBtn, "   ", cancelBtn)
	lines = append(lines, buttons)
	lines = append(lines, "")
	if f.status != "" {
		lines = append(lines, styles.SystemStyle.Render(f.status))
	}

	return strings.Join(lines, "\n")
}

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
	if f.isOpenAICompatible(runtime) {
		return config.NormalizeProvider(config.ProviderConfig{
			Name:    strings.TrimSpace(f.name),
			Preset:  preset,
			BaseURL: strings.TrimSpace(f.baseURL),
			APIKey:  strings.TrimSpace(f.apiKey),
		})
	}
	return config.NormalizeProvider(config.ProviderConfig{
		Name:    config.DefaultProviderName(preset),
		Preset:  preset,
		BaseURL: config.DefaultBaseURL(preset, ""),
		APIKey:  strings.TrimSpace(f.apiKey),
	})
}

func (f *providerForm) handleEnter(runtime *app.Runtime) tea.Cmd {
	saveIdx := 2
	cancelIdx := 3
	if f.isOpenAICompatible(runtime) {
		saveIdx = 4
		cancelIdx = 5
	}

	switch f.fieldIndex {
	case cancelIdx: // Cancel
		f.active = false
		return nil
	case saveIdx: // Save
		cfg := f.configFromForm(runtime)
		if f.isOpenAICompatible(runtime) {
			if strings.TrimSpace(cfg.Name) == "" {
				f.status = "Name is required."
				return nil
			}
			if strings.TrimSpace(cfg.BaseURL) == "" {
				f.status = "Base URL is required."
				return nil
			}
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
		f.fieldIndex = (f.fieldIndex + 1) % f.fieldCount(runtime)
		return nil
	}
}

func (f *providerForm) syncPreset(runtime *app.Runtime) {
	preset := f.currentProviderPreset(runtime)
	switch preset {
	case config.ProviderPresetOpenAICompatible:
		f.name = ""
		f.baseURL = "http://localhost:11434/v1"
		f.apiKey = ""
	case config.ProviderPresetLMStudio:
		f.name = ""
		f.baseURL = "http://127.0.0.1:1234/v1"
		f.apiKey = "lm-studio"
	default:
		if cp, ok := runtime.LookupCatalogProvider(string(preset)); ok {
			f.name = cp.Name
			f.baseURL = cp.API
			f.apiKey = ""
		} else {
			f.name = config.DefaultProviderName(preset)
			f.baseURL = config.DefaultBaseURL(preset, "")
			f.apiKey = ""
		}
	}
}

func renderProviderField(index, active int, label, value string) string {
	if index == active {
		val := value
		if index > 0 {
			val += "█"
		}
		return styles.PopupSelectionStyle.Render(label + ": " + val)
	}
	return styles.PopupFieldLabelStyle.Render(label+": ") + styles.PopupFieldValueStyle.Render(value)
}

var (
	activeButtonStyle = lipgloss.NewStyle().
				Background(styles.MainNeon).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)
	inactiveButtonStyle = lipgloss.NewStyle().
				Background(styles.BorderColor).
				Foreground(styles.Gray).
				Bold(true)
)

func renderProviderButton(index, active int, label string) string {
	text := " " + strings.ToUpper(label) + " "
	if index == active {
		return activeButtonStyle.Render(text)
	}
	return inactiveButtonStyle.Render(text)
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
