package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	modelPickerStepModel    = 0
	modelPickerStepThinking = 1
)

type modelPickerState struct {
	active          bool
	step            int
	models          []provider.ModelInfo
	index           int
	thinkingIndex   int
	thinkingBudgets []int
}

func (p *modelPickerState) Open(models []provider.ModelInfo) {
	p.models = models
	p.active = true
	p.step = modelPickerStepModel
	p.index = 0
	p.thinkingBudgets = app.ThinkingBudgetLevels
	p.thinkingIndex = 0
}

func (p *modelPickerState) Update(msg tea.Msg, runtime *app.Runtime) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if p.step == modelPickerStepThinking {
				p.step = modelPickerStepModel
				return nil
			}
			p.active = false
			return nil

		case "up":
			if p.step == modelPickerStepThinking {
				if len(p.thinkingBudgets) > 0 {
					p.thinkingIndex = (p.thinkingIndex - 1 + len(p.thinkingBudgets)) % len(p.thinkingBudgets)
				}
			} else {
				if len(p.models) > 0 {
					p.index = (p.index - 1 + len(p.models)) % len(p.models)
				}
			}

		case "down":
			if p.step == modelPickerStepThinking {
				if len(p.thinkingBudgets) > 0 {
					p.thinkingIndex = (p.thinkingIndex + 1) % len(p.thinkingBudgets)
				}
			} else {
				if len(p.models) > 0 {
					p.index = (p.index + 1) % len(p.models)
				}
			}

		case "enter":
			if p.step == modelPickerStepModel {
				if len(p.models) > 0 {
					p.step = modelPickerStepThinking
				}
				return nil
			}

			if len(p.models) > 0 {
				model := p.models[p.index]
				budget := app.ThinkingBudgetLevels[p.thinkingIndex]
				p.active = false
				return selectModelAndBudget(runtime, model, budget)
			}
			p.active = false
		}
	}
	return nil
}

func (p modelPickerState) View() string {
	if !p.active {
		return ""
	}

	switch p.step {
	case modelPickerStepThinking:
		return p.renderThinkingPicker()
	default:
		return p.renderModelPickerStep()
	}
}

func (p modelPickerState) renderModelPickerStep() string {
	titleStyle := styles.BoldNeonStyle
	hintStyle := styles.GrayStyle

	rows := []string{
		titleStyle.Render("Select Model"),
		hintStyle.Render("↑↓ navigate  Enter select  Esc cancel"),
		"",
	}

	for i, mod := range p.models {
		cursor := "  "
		style := styles.PopupFieldValueStyle
		if i == p.index {
			cursor = styles.BoldNeonStyle.Render("> ")
			style = styles.PopupSelectionStyle
		}
		rows = append(rows, cursor+style.Render(mod.ID))
	}

	return strings.Join(rows, "\n")
}

func (p modelPickerState) renderThinkingPicker() string {
	titleStyle := styles.BoldNeonStyle
	hintStyle := styles.GrayStyle
	accentStyle := styles.BlueStyle

	chosen := ""
	if len(p.models) > 0 {
		chosen = p.models[p.index].ID
	}

	rows := []string{
		titleStyle.Render("Thinking Budget"),
		accentStyle.Render("Model: " + chosen),
		hintStyle.Render("↑↓ navigate  Enter select  Esc back"),
		"",
	}

	for i, budget := range p.thinkingBudgets {
		cursor := "  "
		style := styles.PopupFieldValueStyle
		if i == p.thinkingIndex {
			cursor = styles.BoldNeonStyle.Render("> ")
			style = styles.PopupSelectionStyle
		}
		label := app.ThinkingBudgetLabels[i]
		if budget > 0 {
			rows = append(rows, fmt.Sprintf("%s%s (%d tokens)", cursor, style.Render(label), budget))
		} else {
			rows = append(rows, cursor+style.Render(label))
		}
	}

	return strings.Join(rows, "\n")
}

func selectModelAndBudget(runtime *app.Runtime, model provider.ModelInfo, budget int) tea.Cmd {
	return func() tea.Msg {
		if err := runtime.SetActiveModelInfo(model); err != nil {
			return ErrorMsg{Err: err}
		}
		if err := runtime.SetThinkingBudget(budget); err != nil {
			return ErrorMsg{Err: err}
		}
		return NotificationMsg{Text: "Model updated: " + model.ID}
	}
}
