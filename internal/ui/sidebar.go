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

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

func (m SidebarModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	info := m.runtime.GetContextInfo()
	contentWidth := m.width - 3 // Account for left border (1) and padding (2)

	// Section 1: Relevant Files (Top Priority)
	fileLines := []string{
		styles.BoldBlueStyle.Render("RELEVANT FILES"),
	}
	if len(info.RelevantFiles) > 0 {
		for _, file := range info.RelevantFiles {
			parts := strings.Split(file, " | ")
			name := parts[0]
			maxNameLen := contentWidth - 2 // "• " is 2 chars
			if maxNameLen > 0 && len(name) > maxNameLen {
				name = "…" + name[len(name)-(maxNameLen-1):]
			}
			fileLines = append(fileLines, "• "+name)
		}
	} else {
		fileLines = append(fileLines, styles.GrayStyle.Render(truncate("none detected", contentWidth)))
	}

	// Section 2: Git Status
	gitLines := []string{
		"",
		styles.BoldVioletStyle.Render("GIT STATUS"),
	}
	if info.HasGit {
		gitLines = append(gitLines, truncate("Branch: "+info.GitBranch, contentWidth))
		if info.GitDirty {
			gitLines = append(gitLines, styles.DiffAddStyle.Render(truncate(fmt.Sprintf("+ %d staged", info.Staged), contentWidth)))
			gitLines = append(gitLines, styles.DiffRemoveStyle.Render(truncate(fmt.Sprintf("- %d unstaged", info.Unstaged), contentWidth)))
			gitLines = append(gitLines, styles.GrayStyle.Render(truncate(fmt.Sprintf("? %d untracked", info.Untracked), contentWidth)))
		} else {
			gitLines = append(gitLines, styles.GrayStyle.Render(truncate("clean", contentWidth)))
		}
	} else {
		gitLines = append(gitLines, styles.GrayStyle.Render(truncate("no repository", contentWidth)))
	}

	// Section 3: Tachikomas (Bottom)
	tachikomaLines := []string{
		"",
		styles.BoldNeonStyle.Render("TACHIKOMAS"),
	}

	var names []string
	if info.Signals != nil {
		for name := range info.Signals {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			status := info.Signals[name]
			// "● " (2) + name (up to 14) + " " (1) = 17 chars approx before status
			maxStatusLen := contentWidth - 17
			if maxStatusLen > 0 {
				status = truncate(status, maxStatusLen)
			} else {
				status = ""
			}
			tachikomaLines = append(tachikomaLines, fmt.Sprintf("● %-14s %s", name, styles.GrayStyle.Render(status)))
		}
	}

	var onDemandNames []string
	if info.OnDemandSignals != nil {
		for name := range info.OnDemandSignals {
			onDemandNames = append(onDemandNames, name)
		}
		sort.Strings(onDemandNames)
		for _, name := range onDemandNames {
			status := truncate("on-demand", contentWidth-17)
			tachikomaLines = append(tachikomaLines, fmt.Sprintf("○ %-14s %s", name, styles.BlueStyle.Render(status)))
		}
	}

	content := append(fileLines, gitLines...)
	content = append(content, tachikomaLines...)

	if len(content) > m.height {
		content = content[:m.height]
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(styles.BorderColor).
		Padding(0, 1).
		Width(contentWidth).
		Height(m.height).
		MaxHeight(m.height)

	return style.Render(strings.Join(content, "\n"))
}
