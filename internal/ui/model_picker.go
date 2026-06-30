package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type modelFilterItem struct {
	info provider.ModelInfo
}

func (m modelFilterItem) FilterKey() string {
	return m.info.ID
}

func (m modelFilterItem) Render(active bool) string {
	cursor := "  "
	style := styles.PopupFieldValueStyle
	if active {
		cursor = styles.BoldNeonStyle.Render("> ")
		style = styles.PopupSelectionStyle
	}
	indicator := " (0)"
	if m.info.SupportsThinking {
		indicator = " (1)"
	}
	return cursor + style.Render(m.info.ID+indicator)
}

type modelPickerState struct {
	list   *FilterList
	active bool
}

func (p *modelPickerState) Open(models []provider.ModelInfo) {
	p.list = NewFilterList("Select Model", "Search model...")
	p.list.Active = true
	var items []FilterableItem
	for _, m := range models {
		items = append(items, modelFilterItem{info: m})
	}
	p.list.SetItems(items)
	p.active = true
}

func (p *modelPickerState) Update(msg tea.Msg) tea.Cmd {
	if !p.active || p.list == nil {
		return nil
	}
	chosen, selected, cancelled := p.list.Update(msg)
	if cancelled {
		p.active = false
		return nil
	}
	if selected {
		p.active = false
		modelItem := chosen.(modelFilterItem)
		return func() tea.Msg {
			return ModelSelectedMsg{Model: modelItem.info}
		}
	}
	return nil
}

func (p modelPickerState) View() string {
	if !p.active || p.list == nil {
		return ""
	}
	return p.list.View()
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
		case keyEsc:
			p.active = false
			return nil

		case keyUp, keyCtrlP:
			if len(p.thinkingBudgets) > 0 {
				p.thinkingIndex--
				if p.thinkingIndex < 0 {
					p.thinkingIndex = len(p.thinkingBudgets) - 1
				}
			}
			return nil

		case keyDown, keyCtrlN, keyTab:
			if len(p.thinkingBudgets) > 0 {
				p.thinkingIndex = (p.thinkingIndex + 1) % len(p.thinkingBudgets)
			}
			return nil

		case keyEnter:
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

	rows := []string{
		styles.PopupTitleStyle.Render("Thinking Budget"),
		styles.PopupFieldLabelStyle.Render("Model: " + p.model.ID),
		styles.PopupMutedStyle.Render("↑↓ navigate  Enter select  Esc cancel"),
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
