package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/agent"
	"github.com/Hoosk/motoko/internal/app/commands"
	"github.com/Hoosk/motoko/internal/app/taskman"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/session"
	"github.com/Hoosk/motoko/internal/skills"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/system"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type PaletteSelectedMsg struct {
	Prompt    string
	SessionID string
	Shortcut  string
	Execute   bool
}

type paletteContext struct {
	Brain             *brain.Brain
	Pending           string
	Providers         []config.ProviderConfig
	Skills            []skills.Skill
	Sessions          []*session.Session
	Tasks             []*taskman.TaskState
	Agents            []agent.AgentDef
	ActiveProvider    config.ProviderConfig
	Info              system.ContextInfo
	QueueLen          int
	Thinking          bool
	HasActiveProvider bool
	ShowSidebar       bool
}

type paletteItem struct {
	category  string
	title     string
	summary   string
	prompt    string
	sessionID string
	shortcut  string
	searchKey string
	execute   bool
}

func (p paletteItem) FilterKey() string {
	return p.searchKey
}

func (p paletteItem) Category() string {
	return p.category
}

func (p paletteItem) Render(active bool) string {
	return p.RenderHighlighted(active, nil)
}

func (p paletteItem) RenderHighlighted(active bool, positions []int) string {
	cursor := "  "
	style := styles.PopupFieldValueStyle
	if active {
		cursor = styles.BoldNeonStyle.Render("> ")
		style = styles.PopupSelectionStyle
	}
	title := highlightText(p.title, positions, style)
	line := cursor + title
	if p.summary != "" {
		line += "\n   " + styles.PopupMutedStyle.Render(p.summary)
	}
	return line
}

type commandPaletteState struct {
	list   *FilterList
	active bool
}

func (p *commandPaletteState) Open(ctx paletteContext) {
	p.list = NewFilterList("Command Palette", "Search commands, shortcuts, sessions, files, and actions...")
	p.list.Active = true
	p.list.SetItems(commandPaletteItems(ctx))
	p.active = true
}

func (p *commandPaletteState) Update(msg tea.Msg) tea.Cmd {
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
		item := chosen.(paletteItem)
		return func() tea.Msg {
			return PaletteSelectedMsg{Prompt: item.prompt, SessionID: item.sessionID, Execute: item.execute, Shortcut: item.shortcut}
		}
	}
	return nil
}

func (p commandPaletteState) View() string {
	if !p.active || p.list == nil {
		return ""
	}
	return p.list.View()
}

func commandPaletteItems(ctx paletteContext) []FilterableItem {
	defs := commands.CommandDefinitions()
	items := make([]FilterableItem, 0, len(defs)+len(ctx.Skills)+len(ctx.Sessions)+len(ctx.Tasks)+len(ctx.Providers)+len(ctx.Info.ModifiedFiles)+16)

	items = append(items, actionPaletteItems(ctx)...)
	items = append(items, navigationPaletteItems(ctx)...)
	items = append(items, workspacePaletteItems(ctx)...)
	items = append(items, commandDefinitionItems(defs)...)
	items = append(items, shortcutPaletteItems()...)
	return items
}

func actionPaletteItems(ctx paletteContext) []FilterableItem {
	var items []FilterableItem
	if ctx.Pending != "" {
		items = append(items,
			newPaletteItem("Actions", "Approve pending command", ctx.Pending, "/approve", true, "", "approve pending "+ctx.Pending),
			newPaletteItem("Actions", "Deny pending command", ctx.Pending, "/deny", true, "", "deny pending "+ctx.Pending),
		)
	}
	if ctx.Thinking {
		items = append(items, newPaletteItem("Actions", "Cancel current request", "Stop the active agent response", "", false, "cancel-request", "cancel stop request thinking"))
	}
	if ctx.QueueLen > 0 {
		items = append(items, newPaletteItem("Actions", fmt.Sprintf("Manage queue (%d prompts)", ctx.QueueLen), "Open queue focus mode", "", false, "ctrl+q", "queue manage queued prompts"))
	}
	items = append(items,
		newPaletteItem("Actions", "Toggle reasoning", "Show or hide reasoning output", "", false, "ctrl+r", "reasoning thinking toggle"),
		newPaletteItem("Actions", "Toggle sidebar", "Show or hide the context sidebar", "", false, "ctrl+s", "sidebar toggle context"),
	)
	return items
}

func navigationPaletteItems(ctx paletteContext) []FilterableItem {
	var items []FilterableItem
	for i, s := range ctx.Sessions {
		if i >= 5 {
			break
		}
		title := s.Title
		if strings.TrimSpace(title) == "" {
			title = s.ID
		}
		items = append(items, paletteItem{
			category:  "Navigate",
			title:     "Session: " + title,
			summary:   fmt.Sprintf("Updated %s ago • %d messages", time.Since(s.UpdatedAt).Round(time.Minute), len(s.History)),
			sessionID: s.ID,
			searchKey: strings.ToLower("session " + title + " " + s.ID),
		})
	}
	for _, model := range ctx.ActiveProvider.Models {
		items = append(items, newPaletteItem("Navigate", "Model: "+model, providerLabel(ctx.ActiveProvider), "/models use "+model, true, "", "model switch "+model))
	}
	for _, agentDef := range ctx.Agents {
		items = append(items, newPaletteItem("Navigate", "Agent: "+agentDef.Name, "Switch active agent mode", "/agent "+agentDef.Name, true, "", "agent mode "+agentDef.Name))
	}
	for _, providerCfg := range ctx.Providers {
		summary := string(providerCfg.Preset)
		if providerCfg.Name == ctx.ActiveProvider.Name {
			summary = "active • " + summary
		}
		items = append(items, newPaletteItem("Navigate", "Provider: "+providerCfg.Name, summary, "/provider use "+providerCfg.Name, true, "", "provider switch "+providerCfg.Name))
	}
	return items
}

func workspacePaletteItems(ctx paletteContext) []FilterableItem {
	var items []FilterableItem
	for _, skill := range ctx.Skills {
		items = append(items, newPaletteItem("Workspace", "Skill: "+skill.Name, skill.Description, "/tool activate_skill "+skill.Name, true, "", "skill activate "+skill.Name+" "+skill.Description))
	}
	if ctx.Brain != nil {
		brainItems := []struct {
			Name    string
			Prompt  string
			Summary string
		}{
			{Name: modePlan, Prompt: "/brain plan", Summary: "Open current implementation plan"},
			{Name: "tasks", Prompt: "/brain tasks", Summary: "Open active task checklist"},
			{Name: "summary", Prompt: "/brain summary", Summary: "Open session summary"},
		}
		for _, brainItem := range brainItems {
			if ctx.Brain.Exists(brainItem.Name) {
				items = append(items, newPaletteItem("Workspace", "Brain: "+brainItem.Name+".md", brainItem.Summary, brainItem.Prompt, true, "", "brain "+brainItem.Name))
			}
		}
	}
	for _, task := range ctx.Tasks {
		if !task.Running {
			continue
		}
		items = append(items, newPaletteItem("Workspace", "Terminate task: "+task.ID, task.Command, "/task terminate "+task.ID, true, "", "task terminate "+task.ID+" "+task.Command))
	}
	for _, file := range ctx.Info.ModifiedFiles {
		items = append(items, newPaletteItem("Workspace", "Mention file: @"+file, "Prefill the modified file in the composer", "@"+file+" ", false, "", "mention file modified "+file))
	}
	return items
}

func commandDefinitionItems(defs []commands.Definition) []FilterableItem {
	items := make([]FilterableItem, 0, len(defs))
	for _, def := range defs {
		prompt, execute := palettePrompt(def)
		items = append(items, paletteItem{
			category:  "Commands",
			title:     def.Usage,
			summary:   def.Summary,
			prompt:    prompt,
			execute:   execute,
			searchKey: strings.ToLower(def.Name + " " + def.Usage + " " + def.Summary),
		})
	}
	return items
}

func shortcutPaletteItems() []FilterableItem {
	shortcuts := []paletteItem{
		{category: categoryShortcuts, title: "Ctrl+M", summary: "Open model selector", shortcut: keyCtrlM, searchKey: "ctrl+m models model selector"},
		{category: categoryShortcuts, title: "Ctrl+P", summary: "Open provider form", shortcut: keyCtrlP, searchKey: "ctrl+p provider form"},
		{category: categoryShortcuts, title: "Ctrl+O", summary: "Open session picker", shortcut: keyCtrlO, searchKey: "ctrl+o sessions"},
		{category: categoryShortcuts, title: "Ctrl+A", summary: "Open agent mode selector", shortcut: keyCtrlA, searchKey: "ctrl+a agent mode"},
		{category: categoryShortcuts, title: "Ctrl+H", summary: "Open help overlay", shortcut: keyCtrlH, searchKey: "ctrl+h help"},
		{category: categoryShortcuts, title: "Ctrl+T", summary: "Open tool catalog", shortcut: keyCtrlT, searchKey: "ctrl+t tools"},
	}
	items := make([]FilterableItem, 0, len(shortcuts))
	for _, shortcut := range shortcuts {
		items = append(items, shortcut)
	}
	return items
}

func newPaletteItem(category, title, summary, prompt string, execute bool, shortcut, searchKey string) paletteItem {
	return paletteItem{
		category:  category,
		title:     title,
		summary:   summary,
		prompt:    prompt,
		execute:   execute,
		shortcut:  shortcut,
		searchKey: strings.ToLower(searchKey + " " + title + " " + summary),
	}
}

func palettePrompt(def commands.Definition) (string, bool) {
	switch def.Name {
	case "provider":
		return "/provider add", true
	case "models":
		return "/models list", true
	case "agent":
		return "/agent ", false
	case "themes":
		return "/themes ", false
	case "tool":
		return "/tool ", false
	case "task":
		return "/task list", true
	case "brain":
		return "/brain list", true
	}

	prompt := def.Usage
	if idx := strings.IndexAny(prompt, "[<"); idx >= 0 {
		prompt = strings.TrimSpace(prompt[:idx]) + " "
		return prompt, false
	}
	return prompt, true
}

func providerLabel(providerCfg config.ProviderConfig) string {
	parts := []string{providerCfg.Name}
	if providerCfg.Preset != "" {
		parts = append(parts, string(providerCfg.Preset))
	}
	return strings.Join(parts, " • ")
}

func highlightText(text string, positions []int, base lipgloss.Style) string {
	if len(positions) == 0 {
		return base.Render(text)
	}
	matched := make(map[int]struct{}, len(positions))
	for _, pos := range positions {
		matched[pos] = struct{}{}
	}
	runes := []rune(text)
	var out strings.Builder
	for i, r := range runes {
		if _, ok := matched[i]; ok {
			out.WriteString(styles.BoldNeonStyle.Render(string(r)))
			continue
		}
		out.WriteString(base.Render(string(r)))
	}
	return out.String()
}
