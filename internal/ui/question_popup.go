package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type questionPopupFocus int

const (
	questionFocusList questionPopupFocus = iota
	questionFocusCustom
)

type questionOptionItem struct {
	index       int
	label       string
	description string
	selected    bool
	multiple    bool
}

func (q questionOptionItem) FilterKey() string {
	return q.label + " " + q.description
}

func (q questionOptionItem) Render(active bool) string {
	prefix := "  "
	if q.multiple {
		if q.selected {
			prefix = "[x] "
		} else {
			prefix = "[ ] "
		}
	}
	if active {
		cursor := styles.BoldNeonStyle.Render("> ")
		return cursor + styles.PopupSelectionStyle.Render(prefix+q.label+questionDescriptionSuffix(q.description))
	}
	return styles.PopupFieldValueStyle.Render(prefix + q.label + questionDescriptionSuffix(q.description))
}

type questionPopupState struct {
	pending *tools.PendingQuestion
	list    *FilterList

	selected    map[int]bool
	custom      string
	focus       questionPopupFocus
	active      bool
	allowCustom bool
	multiple    bool
}

func (p *questionPopupState) Open(pending *tools.PendingQuestion) {
	if pending != nil && len(pending.Question.Options) == 0 && !pending.Question.AllowCustom {
		pending.Resolve(tools.Answer{Cancelled: true})
		p.pending = nil
		p.active = false
		return
	}
	p.pending = pending
	p.selected = make(map[int]bool)
	p.custom = ""
	p.focus = questionFocusList
	p.active = pending != nil
	p.allowCustom = pending != nil && pending.Question.AllowCustom
	p.multiple = pending != nil && pending.Question.Multiple
	if pending == nil {
		p.list = nil
		return
	}
	p.list = NewFilterList(firstNonEmpty(pending.Question.Header, "Question"), "Filter options...")
	p.list.Active = true
	p.refreshItems()
	if len(pending.Question.Options) == 0 && pending.Question.AllowCustom {
		p.focus = questionFocusCustom
	}
}

func (p *questionPopupState) Update(msg tea.Msg) bool {
	if !p.active || p.pending == nil {
		return false
	}
	question := p.pending.Question
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case keyEsc:
			p.pending.Resolve(tools.Answer{Cancelled: true})
			p.active = false
			return true
		case keyTab:
			if p.allowCustom {
				if p.focus == questionFocusList {
					p.focus = questionFocusCustom
				} else {
					p.focus = questionFocusList
				}
				return false
			}
		case "shift+tab":
			if p.allowCustom {
				if p.focus == questionFocusCustom {
					p.focus = questionFocusList
				} else {
					p.focus = questionFocusCustom
				}
				return false
			}
		}
	}

	if p.focus == questionFocusCustom {
		return p.updateCustom(msg)
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case " ":
			if question.Multiple {
				if selected, ok := p.selectedOption(); ok {
					if p.selected[selected.index] {
						delete(p.selected, selected.index)
					} else {
						p.selected[selected.index] = true
					}
					p.refreshItems()
				}
				return false
			}
		case keyEnter:
			if question.Multiple {
				if len(p.selected) == 0 && strings.TrimSpace(p.custom) == "" {
					if p.allowCustom {
						p.focus = questionFocusCustom
					}
					return false
				}
				p.submit()
				return true
			}
			if selected, ok := p.selectedOption(); ok {
				p.pending.Resolve(tools.Answer{Selections: []string{selected.label}})
				p.active = false
				return true
			}
			if p.allowCustom {
				p.focus = questionFocusCustom
			}
			return false
		}
	}

	if p.list != nil {
		_, _, cancelled := p.list.Update(msg)
		if cancelled {
			p.pending.Resolve(tools.Answer{Cancelled: true})
			p.active = false
			return true
		}
	}
	return false
}

func (p *questionPopupState) updateCustom(msg tea.Msg) bool {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	switch key.String() {
	case keyEsc:
		p.pending.Resolve(tools.Answer{Cancelled: true})
		p.active = false
		return true
	case keyEnter:
		if strings.TrimSpace(p.custom) == "" && len(p.selected) == 0 {
			return false
		}
		p.submit()
		return true
	case "backspace":
		p.custom = trimLastRune(p.custom)
		return false
	default:
		if len(key.Runes) > 0 {
			p.custom += string(key.Runes)
		}
		return false
	}
}

func (p *questionPopupState) submit() {
	if p.pending == nil {
		return
	}
	question := p.pending.Question
	selections := make([]string, 0, len(p.selected))
	for i, option := range question.Options {
		if p.selected[i] {
			selections = append(selections, option.Label)
		}
	}
	p.pending.Resolve(tools.Answer{Selections: selections, Custom: strings.TrimSpace(p.custom)})
	p.active = false
}

func (p *questionPopupState) refreshItems() {
	if p.pending == nil || p.list == nil {
		return
	}
	items := make([]FilterableItem, 0, len(p.pending.Question.Options))
	for i, option := range p.pending.Question.Options {
		items = append(items, questionOptionItem{
			index:       i,
			label:       option.Label,
			description: option.Description,
			selected:    p.selected[i],
			multiple:    p.multiple,
		})
	}
	query := ""
	if p.list != nil {
		query = p.list.SearchQuery
	}
	p.list.SetItems(items)
	p.list.Active = true
	p.list.Filter(query)
}

func (p *questionPopupState) selectedOption() (questionOptionItem, bool) {
	if p.list == nil {
		return questionOptionItem{}, false
	}
	item, ok := p.list.Selected()
	if !ok {
		return questionOptionItem{}, false
	}
	selected, ok := item.(questionOptionItem)
	return selected, ok
}

func (p questionPopupState) View() string {
	if !p.active || p.pending == nil {
		return ""
	}
	question := p.pending.Question
	hint := "↑↓ navigate  Enter select  Esc cancel"
	if question.Multiple {
		hint = "↑↓ navigate  Space toggle  Enter submit  Esc cancel"
	}
	if p.allowCustom {
		hint += "  Tab custom"
	}
	rows := []string{
		styles.PopupTitleStyle.Render(firstNonEmpty(question.Header, "Question")),
		styles.PopupMutedStyle.Render(question.Question),
		styles.PopupMutedStyle.Render(hint),
	}
	if p.list != nil && len(question.Options) > 0 {
		rows = append(rows, "", p.list.View())
	}
	if p.allowCustom {
		rows = append(rows, "", styles.PopupFieldLabelStyle.Render("Custom answer:"))
		custom := p.custom
		if p.focus == questionFocusCustom {
			custom += "█"
			rows = append(rows, styles.PopupSelectionStyle.Render(custom))
		} else if strings.TrimSpace(custom) == "" {
			rows = append(rows, styles.PopupMutedStyle.Render("Tab to edit a free-form answer"))
		} else {
			rows = append(rows, styles.PopupFieldValueStyle.Render(custom))
		}
	}
	if question.Multiple {
		rows = append(rows, "", renderProviderButton(0, 0, "submit"))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func questionDescriptionSuffix(description string) string {
	if strings.TrimSpace(description) == "" {
		return ""
	}
	return "  " + description
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
