package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type FilterableItem interface {
	FilterKey() string
	Render(active bool) string
}

type FilterList struct {
	Title         string
	Placeholder   string
	SearchQuery   string
	Items         []FilterableItem
	Filtered      []FilterableItem
	SelectedIndex int
	Active        bool
}

func NewFilterList(title, placeholder string) *FilterList {
	return &FilterList{
		Title:       title,
		Placeholder: placeholder,
	}
}

func (fl *FilterList) SetItems(items []FilterableItem) {
	fl.Items = items
	fl.Filter(fl.SearchQuery)
}

func (fl *FilterList) Filter(query string) {
	fl.SearchQuery = query
	fl.Filtered = nil
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		fl.Filtered = fl.Items
	} else {
		for _, item := range fl.Items {
			if strings.Contains(strings.ToLower(item.FilterKey()), queryLower) {
				fl.Filtered = append(fl.Filtered, item)
			}
		}
	}
	if fl.SelectedIndex >= len(fl.Filtered) {
		fl.SelectedIndex = 0
	}
	if fl.SelectedIndex < 0 {
		fl.SelectedIndex = 0
	}
}

func (fl *FilterList) Selected() (FilterableItem, bool) {
	if len(fl.Filtered) == 0 || fl.SelectedIndex < 0 || fl.SelectedIndex >= len(fl.Filtered) {
		return nil, false
	}
	return fl.Filtered[fl.SelectedIndex], true
}

func (fl *FilterList) Update(msg tea.Msg) (FilterableItem, bool, bool) {
	// Returns: (chosenItem, wasSelected, wasCancelled)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !fl.Active {
			return nil, false, false
		}
		switch msg.String() {
		case keyEsc:
			fl.Active = false
			return nil, false, true

		case keyUp, keyCtrlP:
			if len(fl.Filtered) > 0 {
				fl.SelectedIndex--
				if fl.SelectedIndex < 0 {
					fl.SelectedIndex = len(fl.Filtered) - 1
				}
			}
			return nil, false, false

		case keyDown, keyCtrlN, keyTab:
			if len(fl.Filtered) > 0 {
				fl.SelectedIndex = (fl.SelectedIndex + 1) % len(fl.Filtered)
			}
			return nil, false, false

		case keyEnter:
			if len(fl.Filtered) == 0 {
				fl.Active = false
				return nil, false, true
			}
			chosen := fl.Filtered[fl.SelectedIndex]
			fl.Active = false
			return chosen, true, false

		case "backspace":
			if len(fl.SearchQuery) > 0 {
				fl.Filter(fl.SearchQuery[:len(fl.SearchQuery)-1])
			}
			return nil, false, false

		default:
			// Ignore normal control keys/tab etc for text entry
			if len(msg.Runes) > 0 && msg.Type == tea.KeyRunes {
				fl.Filter(fl.SearchQuery + string(msg.Runes))
			}
			return nil, false, false
		}
	}
	return nil, false, false
}

func (fl *FilterList) View() string {
	if !fl.Active {
		return ""
	}

	titleStyle := styles.BoldNeonStyle
	hintStyle := styles.GrayStyle

	rows := []string{
		titleStyle.Render(fl.Title),
		styles.PopupFieldLabelStyle.Render("Search: ") + styles.PopupSelectionStyle.Render(fl.SearchQuery+"█"),
		hintStyle.Render("↑↓ navigate  letters filter  Enter select  Esc cancel"),
		"",
	}

	if len(fl.Filtered) == 0 {
		rows = append(rows, styles.PopupMutedStyle.Render("  No items match your search."))
		return strings.Join(rows, "\n")
	}

	maxItems := 10
	start := 0
	end := len(fl.Filtered)

	if end > maxItems {
		start = fl.SelectedIndex - maxItems/2
		if start < 0 {
			start = 0
		}
		end = start + maxItems
		if end > len(fl.Filtered) {
			end = len(fl.Filtered)
			start = end - maxItems
		}
	}

	if start > 0 {
		rows = append(rows, hintStyle.Render("   ▲ ... more items above ..."))
	}

	for i := start; i < end; i++ {
		item := fl.Filtered[i]
		isActive := (i == fl.SelectedIndex)
		rows = append(rows, item.Render(isActive))
	}

	if end < len(fl.Filtered) {
		rows = append(rows, hintStyle.Render("   ▼ ... more items below ..."))
	}

	return strings.Join(rows, "\n")
}
