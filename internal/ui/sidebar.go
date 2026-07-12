package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type SidebarModel struct {
	runtime *app.Runtime
	cached  string
	width   int
	height  int
	offset  int
	dirty   bool
}

func NewSidebarModel(runtime *app.Runtime) SidebarModel {
	return SidebarModel{
		runtime: runtime,
		dirty:   true,
	}
}

func (m SidebarModel) Init() tea.Cmd {
	return nil
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	switch msg.(type) {
	case TachikomaStatusMsg, ContextInfoMsg, ContextTokensMsg, ResponseAppliedMsg, AgentResultMsg, AgentStreamBatchMsg, TaskEventMsg, ScheduleEventMsg, SessionLoadedMsg:
		m.dirty = true
	}
	return m, nil
}

func (m *SidebarModel) SetDimensions(width, height int) {
	if m.width == width && m.height == height {
		return
	}
	m.width = width
	m.height = height
	m.dirty = true
}

func (m *SidebarModel) SetOffset(offset int) {
	if m.offset == offset {
		return
	}
	m.offset = offset
	m.dirty = true
}

func renderHeader(title string, style lipgloss.Style, width int) string {
	titleLen := len(title)
	if width <= titleLen+6 {
		return style.Render(title)
	}
	left := 2
	right := max(width-titleLen-left-2, 0)
	return style.Render(fmt.Sprintf("%s %s %s", strings.Repeat("─", left), title, strings.Repeat("─", right)))
}

func (m *SidebarModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if !m.dirty && m.cached != "" {
		return m.cached
	}

	info := m.runtime.GetContextInfo()
	contentWidth := m.width - 3     // Account for left border (1) and padding (2)
	usableWidth := contentWidth - 2 // Usable text width inside padding (1 on each side)

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
		limit := 50
		for i, file := range info.RelevantFiles {
			if i >= limit {
				remaining := len(info.RelevantFiles) - limit
				fileLines = append(fileLines, styles.GrayStyle.Render(fmt.Sprintf("  … and %d more", remaining)))
				break
			}
			parts := strings.Split(file, " | ")
			name := parts[0]
			maxNameLen := usableWidth - 3 // "▫  " is 3 chars
			if maxNameLen > 0 {
				name = contractPath(name, maxNameLen)
			}
			fileLines = append(fileLines, styles.WhiteStyle.Render("▫  ")+name)
		}
		content = append(content, fileLines...)
	}

	if len(info.ModifiedFiles) > 0 {
		modLines := []string{"", renderHeader("MODIFIED", styles.BoldVioletStyle, contentWidth)}
		limit := 50
		for i, file := range info.ModifiedFiles {
			if i >= limit {
				remaining := len(info.ModifiedFiles) - limit
				modLines = append(modLines, styles.GrayStyle.Render(fmt.Sprintf("  … and %d more", remaining)))
				break
			}
			maxNameLen := usableWidth - 3 // "✎  " is 3 chars
			name := file
			if maxNameLen > 0 {
				name = contractPath(name, maxNameLen)
			}
			modLines = append(modLines, styles.PinkStyle.Render("✎  ")+name)
		}
		content = append(content, modLines...)
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

	totalLines := len(content)
	maxOffset := max(totalLines-m.height, 0)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}

	end := min(m.offset+m.height, totalLines)
	content = content[m.offset:end]

	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(styles.Gray).
		Padding(0, 1).
		Width(contentWidth).
		Height(m.height).
		MaxHeight(m.height)

	m.cached = style.Render(strings.Join(content, "\n"))
	m.dirty = false
	return m.cached
}

func contractPath(path string, maxLength int) string {
	path = filepath.ToSlash(path)
	if len(path) <= maxLength {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		if len(path) > maxLength {
			if maxLength > 3 {
				return "..." + path[len(path)-maxLength+3:]
			}
			return path[:maxLength]
		}
		return path
	}

	dirs := make([]string, len(parts)-1)
	for i := 0; i < len(parts)-1; i++ {
		dirs[i] = parts[i]
	}
	filename := parts[len(parts)-1]

	for i := range dirs {
		if len(dirs[i]) <= 1 {
			continue
		}
		runes := []rune(dirs[i])
		if len(runes) > 0 {
			dirs[i] = string(runes[0])
		}

		reconstructed := strings.Join(dirs, "/") + "/" + filename
		if len(reconstructed) <= maxLength {
			return reconstructed
		}
	}

	finalPath := strings.Join(dirs, "/") + "/" + filename
	if len(finalPath) > maxLength {
		if maxLength > 3 {
			return "..." + finalPath[len(finalPath)-maxLength+3:]
		}
		return finalPath[:maxLength]
	}
	return finalPath
}
