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

	var content []string
	if pending := strings.TrimSpace(m.runtime.PendingApproval()); pending != "" {
		content = append(content,
			renderHeader("APPROVAL", styles.BoldNeonStyle, contentWidth),
			styles.ErrorStyle.Render(truncate("  "+pending, contentWidth)),
			"",
		)
	}
	if tasks := m.runtime.ActiveTasks(); tasks > 0 {
		content = append(content,
			renderHeader("TASKS", styles.BoldBlueStyle, contentWidth),
			styles.WhiteStyle.Render(fmt.Sprintf("%d active", tasks)),
			"",
		)
	}

	if len(info.RelevantFiles) > 0 {
		fileLines := []string{renderHeader("FILES", styles.BoldBlueStyle, contentWidth)}
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
		content = append(content, fileLines...)
	}

	if info.HasGit {
		gitLines := []string{"", renderHeader("GIT", styles.BoldVioletStyle, contentWidth)}
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
		content = append(content, gitLines...)
	}

	activeSubagents := m.runtime.ActiveSubagents()
	if len(activeSubagents) > 0 {
		subagentLines := []string{"", renderHeader("SUBAGENTS", styles.BoldBlueStyle, contentWidth)}
		for _, name := range activeSubagents {
			subagentLines = append(subagentLines, fmt.Sprintf("%s %s", styles.BlueStyle.Render("✦"), styles.WhiteStyle.Render(name)))
		}
		content = append(content, subagentLines...)
	}

	var hasTachikomas bool
	var tachikomaLines []string
	var names []string
	if info.Signals != nil {
		for name := range info.Signals {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) > 0 {
			tachikomaLines = append(tachikomaLines, "", renderHeader("TACHIKOMAS", styles.BoldNeonStyle, contentWidth))
		}
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
		if len(onDemandNames) > 0 && len(tachikomaLines) == 0 {
			tachikomaLines = append(tachikomaLines, "", renderHeader("TACHIKOMAS", styles.BoldNeonStyle, contentWidth))
		}
		for _, name := range onDemandNames {
			status := truncate("on-demand", contentWidth-17)
			tachikomaLines = append(tachikomaLines, fmt.Sprintf("%s %-12s %s", styles.BlueStyle.Render("⬡"), name, styles.GrayStyle.Render(status)))
			hasTachikomas = true
		}
	}

	if hasTachikomas {
		content = append(content, tachikomaLines...)
	}

	if len(content) == 0 {
		content = append(content,
			styles.GrayStyle.Render("Sidebar idle."),
			styles.GrayStyle.Render("Open files, run tasks,"),
			styles.GrayStyle.Render("or inspect code to see live context."),
		)
	}

	if len(content) > m.height {
		content = content[:m.height]
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(styles.Gray).
		Padding(0, 1).
		Width(contentWidth).
		Height(m.height).
		MaxHeight(m.height)

	return style.Render(strings.Join(content, "\n"))
}
