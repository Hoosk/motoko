package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type SidebarModel struct {
	runtime *app.Runtime
	width   int
	height  int
}

func NewSidebarModel(runtime *app.Runtime) SidebarModel {
	return SidebarModel{
		runtime: runtime,
	}
}

func (m SidebarModel) Init() tea.Cmd {
	return nil
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	return m, nil
}

func (m SidebarModel) View() string {
	if m.width <= 0 {
		return ""
	}

	info := m.runtime.GetContextInfo()

	grayStyle := lipgloss.NewStyle().Foreground(styles.Gray)
	blueStyle := lipgloss.NewStyle().Foreground(styles.AccentBlue)

	// Section 1: Relevant Files (Top Priority)
	fileLines := []string{
		lipgloss.NewStyle().Foreground(styles.AccentBlue).Bold(true).Render("RELEVANT FILES"),
	}
	if len(info.RelevantFiles) > 0 {
		for _, file := range info.RelevantFiles {
			// Extract just the filename or a shortened path
			parts := strings.Split(file, " | ")
			name := parts[0]
			if len(name) > m.width-4 {
				name = "..." + name[len(name)-(m.width-7):]
			}
			fileLines = append(fileLines, "• "+name)
		}
	} else {
		fileLines = append(fileLines, grayStyle.Render("none detected"))
	}

	// Section 2: Git Status
	gitLines := []string{
		"",
		lipgloss.NewStyle().Foreground(styles.AccentViolet).Bold(true).Render("GIT STATUS"),
	}
	if info.HasGit {
		gitLines = append(gitLines, "Branch: "+info.GitBranch)
		if info.GitDirty {
			gitLines = append(gitLines, styles.DiffAddStyle.Render(fmt.Sprintf("+ %d staged", info.Staged)))
			gitLines = append(gitLines, styles.DiffRemoveStyle.Render(fmt.Sprintf("- %d unstaged", info.Unstaged)))
			gitLines = append(gitLines, grayStyle.Render(fmt.Sprintf("? %d untracked", info.Untracked)))
		} else {
			gitLines = append(gitLines, grayStyle.Render("clean"))
		}
	} else {
		gitLines = append(gitLines, grayStyle.Render("no repository"))
	}

	// Section 3: Tachikomas (Bottom)
	tachikomaLines := []string{
		"",
		lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render("TACHIKOMAS"),
	}
	
	// Deterministic sorting for Tachikomas to prevent "blinking"
	var names []string
	if info.Signals != nil {
		for name := range info.Signals {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			tachikomaLines = append(tachikomaLines, fmt.Sprintf("● %-14s %s", name, grayStyle.Render(info.Signals[name])))
		}
	}

	var onDemandNames []string
	if info.OnDemandSignals != nil {
		for name := range info.OnDemandSignals {
			onDemandNames = append(onDemandNames, name)
		}
		sort.Strings(onDemandNames)
		for _, name := range onDemandNames {
			tachikomaLines = append(tachikomaLines, fmt.Sprintf("○ %-14s %s", name, blueStyle.Render("on-demand")))
		}
	}

	content := append(fileLines, gitLines...)
	content = append(content, tachikomaLines...)

	if m.height > 0 && len(content) > m.height {
		content = content[:m.height]
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(styles.Gray).
		Padding(0, 1).
		Width(m.width).
		Height(m.height)

	return style.Render(strings.Join(content, "\n"))
}
