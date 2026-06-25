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

func renderHeader(title string, style lipgloss.Style, width int) string {
	titleLen := len(title)
	if width <= titleLen+6 {
		return style.Render(title)
	}
	left := 2
	right := width - titleLen - left - 2
	if right < 0 {
		right = 0
	}
	return style.Render(fmt.Sprintf("%s %s %s", strings.Repeat("─", left), title, strings.Repeat("─", right)))
}

func (m SidebarModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	info := m.runtime.GetContextInfo()
	contentWidth := m.width - 3 // Account for left border (1) and padding (2)

	// Top Title Card
	sidebarHeader := []string{
		styles.BoldNeonStyle.Render("❖  MONITOR PROFILE"),
		styles.GrayStyle.Render(strings.Repeat("━", contentWidth)),
		"",
	}

	// Section 1: Relevant Files
	fileLines := []string{
		renderHeader("RELEVANT FILES", styles.BoldBlueStyle, contentWidth),
	}
	if len(info.RelevantFiles) > 0 {
		limit := 5
		for i, file := range info.RelevantFiles {
			if i >= limit {
				remaining := len(info.RelevantFiles) - limit
				fileLines = append(fileLines, styles.GrayStyle.Render(fmt.Sprintf("  … and %d more", remaining)))
				break
			}
			parts := strings.Split(file, " | ")
			name := parts[0]
			maxNameLen := contentWidth - 3 // "▫ " is 2 chars + space
			if maxNameLen > 0 && len(name) > maxNameLen {
				name = "…" + name[len(name)-(maxNameLen-1):]
			}
			fileLines = append(fileLines, styles.WhiteStyle.Render("▫  ")+name)
		}
	} else {
		fileLines = append(fileLines, styles.GrayStyle.Render(truncate("  none detected", contentWidth)))
	}

	// Section 2: Git Status
	gitLines := []string{
		"",
		renderHeader("GIT STATUS", styles.BoldVioletStyle, contentWidth),
	}
	if info.HasGit {
		gitLines = append(gitLines, fmt.Sprintf("⎇  %s", styles.VioletStyle.Render(info.GitBranch)))
		if info.GitDirty {
			var statusParts []string
			if info.Staged > 0 {
				statusParts = append(statusParts, styles.DiffAddStyle.Render(fmt.Sprintf("+%d staged", info.Staged)))
			}
			if info.Unstaged > 0 {
				statusParts = append(statusParts, styles.DiffRemoveStyle.Render(fmt.Sprintf("-%d unstaged", info.Unstaged)))
			}
			if info.Untracked > 0 {
				statusParts = append(statusParts, styles.GrayStyle.Render(fmt.Sprintf("?%d untracked", info.Untracked)))
			}
			if len(statusParts) > 0 {
				gitLines = append(gitLines, "  "+strings.Join(statusParts, "  "))
			}
		} else {
			gitLines = append(gitLines, "  "+styles.GrayStyle.Render("✔ clean"))
		}
	} else {
		gitLines = append(gitLines, styles.GrayStyle.Render("  no repository"))
	}

	// Section 3: Subagents
	subagentLines := []string{
		"",
		renderHeader("SUBAGENTS", styles.BoldBlueStyle, contentWidth),
	}
	activeSubagents := m.runtime.ActiveSubagents()
	if len(activeSubagents) > 0 {
		for _, name := range activeSubagents {
			subagentLines = append(subagentLines, fmt.Sprintf("%s %s", styles.BlueStyle.Render("✦"), styles.WhiteStyle.Render(name)))
		}
	} else {
		subagentLines = append(subagentLines, styles.GrayStyle.Render(truncate("  none active", contentWidth)))
	}

	// Section 4: Tachikomas
	tachikomaLines := []string{
		"",
		renderHeader("TACHIKOMAS", styles.BoldNeonStyle, contentWidth),
	}

	var hasTachikomas bool
	var names []string
	if info.Signals != nil {
		for name := range info.Signals {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			status := info.Signals[name]
			maxStatusLen := contentWidth - 17
			if maxStatusLen > 0 {
				status = truncate(status, maxStatusLen)
			} else {
				status = ""
			}
			statusStyle := styles.GrayStyle
			lowerStatus := strings.ToLower(status)
			if strings.Contains(lowerStatus, "run") || strings.Contains(lowerStatus, "ok") || strings.Contains(lowerStatus, "activ") {
				statusStyle = styles.NeonStyle
			} else if strings.Contains(lowerStatus, "fail") || strings.Contains(lowerStatus, "err") {
				statusStyle = styles.PinkStyle
			}
			tachikomaLines = append(tachikomaLines, fmt.Sprintf("%s %-12s %s", styles.NeonStyle.Render("⬢"), name, statusStyle.Render(status)))
			hasTachikomas = true
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
			tachikomaLines = append(tachikomaLines, fmt.Sprintf("%s %-12s %s", styles.BlueStyle.Render("⬡"), name, styles.GrayStyle.Render(status)))
			hasTachikomas = true
		}
	}

	if !hasTachikomas {
		tachikomaLines = append(tachikomaLines, styles.GrayStyle.Render("  none active"))
	}

	content := append(sidebarHeader, fileLines...)
	content = append(content, gitLines...)
	content = append(content, subagentLines...)
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
