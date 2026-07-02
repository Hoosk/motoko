package ui

import (
	"sort"
	"strings"

	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type FilterableItem interface {
	FilterKey() string
	Render(active bool) string
}

type CategorizedItem interface {
	FilterableItem
	Category() string
}

type HighlightableItem interface {
	FilterableItem
	RenderHighlighted(active bool, positions []int) string
}

type FilterList struct {
	Title         string
	Placeholder   string
	SearchQuery   string
	Items         []FilterableItem
	Filtered      []FilterableItem
	positions     [][]int
	SelectedIndex int
	Active        bool
}

type filteredItem struct {
	Item     FilterableItem
	Match    fuzzyMatch
	Original int
}

type filterListRow struct {
	Item      FilterableItem
	Positions []int
	Header    string
	Index     int
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
	fl.positions = nil
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		fl.Filtered = append(fl.Filtered, fl.Items...)
		fl.positions = make([][]int, len(fl.Filtered))
	} else {
		matches := make([]filteredItem, 0, len(fl.Items))
		for i, item := range fl.Items {
			match := scoreFuzzy(queryLower, item.FilterKey())
			if match.Score == noFuzzyMatch {
				continue
			}
			matches = append(matches, filteredItem{Item: item, Match: match, Original: i})
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].Match.Score == matches[j].Match.Score {
				return matches[i].Original < matches[j].Original
			}
			return matches[i].Match.Score > matches[j].Match.Score
		})
		for _, match := range matches {
			fl.Filtered = append(fl.Filtered, match.Item)
			fl.positions = append(fl.positions, match.Match.Positions)
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

	displayRows, selectedRow := fl.displayRows()
	maxItems := 12
	start := 0
	end := len(displayRows)

	if end > maxItems {
		start = selectedRow - maxItems/2
		if start < 0 {
			start = 0
		}
		end = start + maxItems
		if end > len(displayRows) {
			end = len(displayRows)
			start = end - maxItems
		}
	}

	if start > 0 {
		rows = append(rows, hintStyle.Render("   ▲ ... more items above ..."))
	}

	for i := start; i < end; i++ {
		row := displayRows[i]
		if row.Header != "" {
			rows = append(rows, styles.PopupMutedStyle.Render("── "+row.Header+" ──"))
			continue
		}
		isActive := row.Index == fl.SelectedIndex
		if item, ok := row.Item.(HighlightableItem); ok {
			rows = append(rows, item.RenderHighlighted(isActive, row.Positions))
			continue
		}
		rows = append(rows, row.Item.Render(isActive))
	}

	if end < len(displayRows) {
		rows = append(rows, hintStyle.Render("   ▼ ... more items below ..."))
	}

	return strings.Join(rows, "\n")
}

func (fl *FilterList) displayRows() ([]filterListRow, int) {
	rows := make([]filterListRow, 0, len(fl.Filtered)+4)
	selectedRow := 0
	lastCategory := ""
	for i, item := range fl.Filtered {
		if categoryItem, ok := item.(CategorizedItem); ok {
			category := categoryItem.Category()
			if category != "" && category != lastCategory {
				rows = append(rows, filterListRow{Header: category})
				lastCategory = category
			}
		}
		if i == fl.SelectedIndex {
			selectedRow = len(rows)
		}
		var positions []int
		if i < len(fl.positions) {
			positions = fl.positions[i]
		}
		rows = append(rows, filterListRow{Item: item, Positions: positions, Index: i})
	}
	return rows, selectedRow
}
