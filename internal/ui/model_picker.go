package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type modelPickerState struct {
	models []provider.ModelInfo
	index  int
	active bool
}

func (p *modelPickerState) Open(models []provider.ModelInfo) {
	p.models = models
	p.active = true
	p.index = 0
}

func (p *modelPickerState) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !p.active {
			return nil
		}
		switch msg.String() {
		case keyEsc:
			p.active = false
			return nil

		case keyUp, keyCtrlP:
			if len(p.models) > 0 {
				p.index--
				if p.index < 0 {
					p.index = len(p.models) - 1
				}
			}
			return nil

		case keyDown, keyCtrlN, keyTab:
			if len(p.models) > 0 {
				p.index = (p.index + 1) % len(p.models)
			}
			return nil

		case keyEnter:
			if len(p.models) == 0 {
				p.active = false
				return nil
			}
			chosen := p.models[p.index]
			p.active = false
			return func() tea.Msg {
				return ModelSelectedMsg{Model: chosen}
			}
		}
	}
	return nil
}

func (p modelPickerState) View() string {
	if !p.active {
		return ""
	}

	titleStyle := styles.BoldNeonStyle
	hintStyle := styles.GrayStyle

	rows := []string{
		titleStyle.Render("Select Model"),
		hintStyle.Render("↑↓ navigate  Enter select  Esc cancel"),
		"",
	}

	maxItems := 10
	start := 0
	end := len(p.models)

	if end > maxItems {
		start = p.index - maxItems/2
		if start < 0 {
			start = 0
		}
		end = start + maxItems
		if end > len(p.models) {
			end = len(p.models)
			start = end - maxItems
		}
	}

	if start > 0 {
		rows = append(rows, hintStyle.Render("   ▲ ... more models above ..."))
	}

	for i := start; i < end; i++ {
		mod := p.models[i]
		cursor := "  "
		style := styles.PopupFieldValueStyle
		if i == p.index {
			cursor = styles.BoldNeonStyle.Render("> ")
			style = styles.PopupSelectionStyle
		}
		indicator := " (0)"
		if mod.SupportsThinking {
			indicator = " (1)"
		}
		rows = append(rows, cursor+style.Render(mod.ID+indicator))
	}

	if end < len(p.models) {
		rows = append(rows, hintStyle.Render("   ▼ ... more models below ..."))
	}

	return strings.Join(rows, "\n")
}

type thinkingPickerState struct {
	thinkingBudgets []int
	model           provider.ModelInfo
	thinkingIndex   int
	active          bool
}

func (p *thinkingPickerState) Open(model provider.ModelInfo) {
	p.model = model
	p.thinkingBudgets = app.ThinkingBudgetLevels
	p.thinkingIndex = 0
	p.active = true
}

func (p *thinkingPickerState) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !p.active {
			return nil
		}
		switch msg.String() {
		case "esc":
			p.active = false
			return nil

		case "up", "ctrl+p":
			if len(p.thinkingBudgets) > 0 {
				p.thinkingIndex--
				if p.thinkingIndex < 0 {
					p.thinkingIndex = len(p.thinkingBudgets) - 1
				}
			}
			return nil

		case "down", "ctrl+n", "tab":
			if len(p.thinkingBudgets) > 0 {
				p.thinkingIndex = (p.thinkingIndex + 1) % len(p.thinkingBudgets)
			}
			return nil

		case "enter":
			budget := p.thinkingBudgets[p.thinkingIndex]
			p.active = false
			return func() tea.Msg {
				return ThinkingBudgetSelectedMsg{Model: p.model, Budget: budget}
			}
		}
	}
	return nil
}

func (p thinkingPickerState) View() string {
	if !p.active {
		return ""
	}

	titleStyle := styles.BoldNeonStyle
	hintStyle := styles.GrayStyle
	accentStyle := styles.BlueStyle

	rows := []string{
		titleStyle.Render("Thinking Budget"),
		accentStyle.Render("Model: " + p.model.ID),
		hintStyle.Render("↑↓ navigate  Enter select  Esc cancel"),
		"",
	}

	labels := provider.GetThinkingLabels(p.model.ID)
	for i := range p.thinkingBudgets {
		cursor := "  "
		style := styles.PopupFieldValueStyle
		if i == p.thinkingIndex {
			cursor = styles.BoldNeonStyle.Render("> ")
			style = styles.PopupSelectionStyle
		}
		var label string
		if i < len(labels) {
			label = labels[i]
		} else {
			label = "unknown"
		}
		rows = append(rows, cursor+style.Render(label))
	}

	return strings.Join(rows, "\n")
}

