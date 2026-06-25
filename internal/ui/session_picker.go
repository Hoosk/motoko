package ui

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type sessionFilterItem struct {
	session *session.Session
}

func (s sessionFilterItem) FilterKey() string {
	return s.session.Title
}

func (s sessionFilterItem) Render(active bool) string {
	cursor := "  "
	style := styles.PopupFieldLabelStyle
	if active {
		cursor = styles.BoldNeonStyle.Render("> ")
		style = styles.PopupSelectionStyle
	}
	line := fmt.Sprintf("%s  %s  (%d mensajes)", s.session.Title, s.session.UpdatedAt.Format("2006-01-02 15:04"), len(s.session.History))
	return cursor + style.Render(line)
}

type sessionPickerState struct {
	list    *FilterList
	active  bool
	loading bool
}

func (p *sessionPickerState) Open() {
	p.list = nil
	p.loading = true
	p.active = true
}

func (p *sessionPickerState) Update(msg tea.Msg, runtime *app.Runtime) tea.Cmd {
	switch msg := msg.(type) {
	case SessionsMsg:
		if msg.Err != nil {
			return func() tea.Msg { return ErrorMsg{Err: msg.Err} }
		}
		p.list = NewFilterList("Sessions", "Search session...")
		p.list.Active = true
		var items []FilterableItem
		for _, s := range msg.Sessions {
			items = append(items, sessionFilterItem{session: s})
		}
		p.list.SetItems(items)
		p.active = true
		p.loading = false
		return nil

	case tea.KeyMsg:
		if !p.active || p.list == nil {
			return nil
		}
		chosen, selected, cancelled := p.list.Update(msg)
		if cancelled {
			p.active = false
			p.loading = false
			return nil
		}
		if selected {
			p.active = false
			p.loading = false
			sessionItem := chosen.(sessionFilterItem)
			chosenSession := sessionItem.session
			return func() tea.Msg {
				if err := runtime.LoadSession(chosenSession.ID); err != nil {
					return SessionLoadedMsg{Err: err}
				}
				return SessionLoadedMsg{Session: chosenSession}
			}
		}
	}
	return nil
}

func (p sessionPickerState) View() string {
	if !p.active {
		return ""
	}
	if p.loading && (p.list == nil || len(p.list.Items) == 0) {
		rows := []string{
			styles.PopupTitleStyle.Render("Sessions"),
			"",
			styles.PopupMutedStyle.Render("Loading sessions..."),
		}
		return strings.Join(rows, "\n")
	}
	if p.list == nil || len(p.list.Items) == 0 {
		rows := []string{
			styles.PopupTitleStyle.Render("Sessions"),
			"",
			styles.PopupMutedStyle.Render("No sessions saved for this workspace."),
		}
		return strings.Join(rows, "\n")
	}
	return p.list.View()
}
