package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	settingsModeList = iota
	settingsModeEditIterations
	settingsModePickVerbosity
)

var thinkingVerbosityOptions = []string{"normal", "concise", "caveman"}

type settingsPopupState struct {
	stagedVerbosity string
	status          string
	buffer          string
	index           int
	verbosityIndex  int
	mode            int
	active          bool
}

func (p *settingsPopupState) Open() {
	p.index = 0
	p.mode = settingsModeList
	p.buffer = ""
	p.status = ""
	p.stagedVerbosity = ""
	p.active = true
}

func (p *settingsPopupState) Update(msg tea.Msg, runtime *app.Runtime) tea.Cmd {
	if !p.active {
		return nil
	}
	cfg := runtime.Config()
	if cfg == nil {
		return nil
	}
	if p.stagedVerbosity == "" {
		p.stagedVerbosity = firstNonEmpty(cfg.ThinkingVerbosity, "normal")
	}
	rows := p.rows(cfg)
	if key, ok := msg.(tea.KeyMsg); ok {
		switch p.mode {
		case settingsModeEditIterations:
			return p.updateIterations(key)
		case settingsModePickVerbosity:
			return p.updateVerbosityPicker(key)
		}

		switch key.String() {
		case keyEsc:
			p.active = false
			p.status = ""
			return nil
		case keyUp, keyCtrlP:
			p.index--
			if p.index < 0 {
				p.index = len(rows) - 1
			}
			return nil
		case keyDown, keyCtrlN, keyTab:
			p.index = (p.index + 1) % len(rows)
			return nil
		case keyEnter:
			switch rows[p.index].key {
			case "thinking_verbosity":
				p.mode = settingsModePickVerbosity
				p.verbosityIndex = verbosityIndex(thinkingVerbosityOptions, p.stagedVerbosity)
				p.status = ""
				return nil
			case "max_iterations":
				p.mode = settingsModeEditIterations
				p.buffer = strconv.Itoa(max(1, currentMaxIterations(cfg, p.buffer)))
				p.status = ""
				return nil
			case "save":
				cfg.ThinkingVerbosity = p.stagedVerbosity
				if value, err := strconv.Atoi(strings.TrimSpace(p.buffer)); err == nil && value > 0 {
					cfg.MaxIterations = value
				} else if cfg.MaxIterations <= 0 {
					cfg.MaxIterations = 250
				}
				p.active = false
				return saveRuntimeConfig(runtime, "Settings saved")
			case "cancel":
				p.active = false
				p.status = ""
				return nil
			}
		}
	}
	return nil
}

func (p *settingsPopupState) updateIterations(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case keyEsc:
		p.mode = settingsModeList
		p.status = ""
		return nil
	case keyEnter:
		value, err := strconv.Atoi(strings.TrimSpace(p.buffer))
		if err != nil || value < 1 {
			p.status = "Max iterations must be a positive integer."
			return nil
		}
		p.buffer = strconv.Itoa(value)
		p.status = ""
		p.mode = settingsModeList
		return nil
	case keyBackspace:
		p.buffer = trimLastRune(p.buffer)
		if strings.TrimSpace(p.buffer) == "" {
			p.buffer = ""
		}
		return nil
	default:
		if len(key.Runes) > 0 {
			for _, r := range key.Runes {
				if r < '0' || r > '9' {
					p.status = "Only digits are allowed."
					return nil
				}
			}
			p.buffer += string(key.Runes)
			p.status = ""
		}
		return nil
	}
}

func (p *settingsPopupState) updateVerbosityPicker(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case keyEsc:
		p.mode = settingsModeList
		return nil
	case keyUp, keyCtrlP:
		p.verbosityIndex--
		if p.verbosityIndex < 0 {
			p.verbosityIndex = len(thinkingVerbosityOptions) - 1
		}
		return nil
	case keyDown, keyCtrlN, keyTab:
		p.verbosityIndex = (p.verbosityIndex + 1) % len(thinkingVerbosityOptions)
		return nil
	case keyEnter:
		p.stagedVerbosity = thinkingVerbosityOptions[p.verbosityIndex]
		p.mode = settingsModeList
		return nil
	default:
		return nil
	}
}

func (p settingsPopupState) View(runtime *app.Runtime) string {
	if !p.active {
		return ""
	}
	cfg := runtime.Config()
	if cfg == nil {
		return ""
	}
	rows := p.rows(cfg)
	lines := []string{
		styles.PopupTitleStyle.Render("Settings"),
	}
	switch p.mode {
	case settingsModePickVerbosity:
		lines = append(lines,
			styles.PopupMutedStyle.Render("↑↓ navigate  Enter select  Esc cancel"),
			"",
		)
		for i, option := range thinkingVerbosityOptions {
			line := option + questionDescriptionSuffix(verbosityDescription(option))
			if i == p.verbosityIndex {
				lines = append(lines, styles.PopupSelectionStyle.Render(line))
			} else {
				lines = append(lines, styles.PopupFieldValueStyle.Render(line))
			}
		}
	case settingsModeEditIterations:
		lines = append(lines,
			styles.PopupMutedStyle.Render("Type digits  Enter confirm  Esc cancel"),
			"",
			renderProviderField(0, 0, "Max tool iterations", p.buffer),
		)
	default:
		lines = append(lines,
			styles.PopupMutedStyle.Render("↑↓ navigate  Enter edit/select  Esc cancel"),
			"",
		)
		for i, row := range rows {
			line := fmt.Sprintf("%s: %s", row.label, row.value)
			if i == p.index {
				lines = append(lines, styles.PopupSelectionStyle.Render(line))
			} else {
				lines = append(lines, styles.PopupFieldValueStyle.Render(line))
			}
		}
	}
	if strings.TrimSpace(p.status) != "" {
		lines = append(lines, "", styles.SystemStyle.Render(p.status))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

type settingsRow struct {
	key   string
	label string
	value string
}

func (p settingsPopupState) rows(cfg *config.AppConfig) []settingsRow {
	verbosity := firstNonEmpty(p.stagedVerbosity, cfg.ThinkingVerbosity, "normal")
	iterations := strconv.Itoa(max(1, currentMaxIterations(cfg, p.buffer)))
	return []settingsRow{
		{key: "thinking_verbosity", label: "Thinking verbosity", value: verbosity},
		{key: "max_iterations", label: "Max tool iterations", value: iterations},
		{key: "save", label: "Save", value: "Persist changes"},
		{key: "cancel", label: "Cancel", value: "Discard changes"},
	}
}

func currentMaxIterations(cfg *config.AppConfig, buffer string) int {
	if value, err := strconv.Atoi(strings.TrimSpace(buffer)); err == nil && value > 0 {
		return value
	}
	if cfg != nil && cfg.MaxIterations > 0 {
		return cfg.MaxIterations
	}
	return 250
}

func verbosityIndex(options []string, current string) int {
	for i, option := range options {
		if strings.EqualFold(option, current) {
			return i
		}
	}
	return 0
}

func verbosityDescription(option string) string {
	switch option {
	case "concise":
		return "shorter internal reasoning"
	case "caveman":
		return "aggressively compressed thinking"
	default:
		return "default reasoning behavior"
	}
}

func saveRuntimeConfig(runtime *app.Runtime, message string) tea.Cmd {
	return func() tea.Msg {
		if err := runtime.SaveConfig(); err != nil {
			return ErrorMsg{Err: err}
		}
		return NotificationMsg{Text: message}
	}
}
