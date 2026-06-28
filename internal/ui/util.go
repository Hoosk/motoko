package ui

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const (
	keyTab    = "tab"
	keyCtrlN  = "ctrl+n"
	keyCtrlP  = "ctrl+p"
	keyEnter  = "enter"
	keyEsc    = "esc"
	keyUp     = "up"
	keyDown   = "down"
	keyRight  = "right"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var thinkingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func writeClipboard(text string) error {
	seq := osc52.New(text)
	if os.Getenv("TMUX") != "" {
		seq = seq.Tmux()
	} else if os.Getenv("STY") != "" {
		seq = seq.Screen()
	}
	if _, err := seq.WriteTo(os.Stderr); err == nil {
		return nil
	}
	if err := clipboard.WriteAll(text); err == nil {
		return nil
	}
	commands := [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("could not copy: OSC52, clipboard backend, and wl-copy/xclip/xsel all failed")
}

func copySelection(text string) tea.Cmd {
	return func() tea.Msg {
		return CopySelectionMsg{Err: writeClipboard(text)}
	}
}

func pendingLabel(pending string) string {
	if pending == "" {
		return "none"
	}
	return pending
}

func renderToolPalette(specs []tools.Spec, tachikomaInfo map[string]string) string {
	title := styles.PopupTitleStyle.Render("TOOL CATALOG")
	help := styles.PopupMutedStyle.Render("Press Ctrl+T to close. Use /tool <name> <args> to execute.")

	toolList := renderToolList(specs)

	sections := []string{
		title,
		help,
		"",
		toolList,
	}

	return strings.Join(sections, "\n")
}

func renderToolList(specs []tools.Spec) string {
	if len(specs) == 0 {
		return styles.SystemStyle.Render("No tools registered.")
	}

	var lines []string
	for _, spec := range specs {
		name := styles.SelectionStyle.Width(12).Render(spec.Name)
		usage := styles.CommandStyle.Render(spec.Usage)
		summary := styles.PopupMutedStyle.Render(spec.Summary)

		// Create a clean block for each tool
		toolBlock := fmt.Sprintf("%s  %s\n              %s", name, usage, summary)
		lines = append(lines, toolBlock)
	}
	return strings.Join(lines, "\n\n")
}

func formatShortcut(key, desc string) string {
	return fmt.Sprintf("%s %s", styles.BoldNeonStyle.Render(key), styles.GrayStyle.Render(desc))
}

func helpView() string {
	rows := [][]string{
		{formatShortcut("enter", "send"), formatShortcut("tab", "next suggestion")},
		{formatShortcut("ctrl+p", "providers"), formatShortcut("ctrl+m", "models")},
		{formatShortcut("ctrl+o", "sessions"), formatShortcut("ctrl+s", "sidebar")},
		{formatShortcut("ctrl+a", "modes"), formatShortcut("ctrl+r", "toggle reasoning")},
		{formatShortcut("ctrl+t", "tools"), formatShortcut("ctrl+h", "this help")},
		{formatShortcut("/", "commands"), formatShortcut("@", "mention file")},
	}

	var formatted []string
	for _, row := range rows {
		formatted = append(formatted, strings.Join(row, "  "))
	}

	return styles.GrayStyle.Render(strings.Join(formatted, "\n"))
}

func renderTachikomaList(statuses map[string]string) string {
	if len(statuses) == 0 {
		return styles.SystemStyle.Render("No background workers active.")
	}

	names := make([]string, 0, len(statuses))
	for name := range statuses {
		names = append(names, name)
	}
	sort.Strings(names)

	var lines []string
	for _, name := range names {
		status := statuses[name]
		indicator := styles.NeonStyle.Render("●")
		if strings.Contains(strings.ToLower(status), "error") || strings.Contains(strings.ToLower(status), "fail") {
			indicator = styles.PinkStyle.Render("●")
		}

		line := fmt.Sprintf("%s %-15s %s", indicator, styles.WhiteStyle.Render(name), styles.GrayStyle.Render(status))
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func trimLastRune(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func truncateANSI(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	var out strings.Builder
	col := 0
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			j := i + 2
			for j < len(runes) {
				c := runes[j]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
					j++
					break
				}
				j++
			}
			out.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			rw = 1
		}
		if col+rw > maxCols {
			break
		}
		out.WriteRune(r)
		col += rw
		i++
	}
	out.WriteString("\x1b[0m")
	return out.String()
}

// overlayBase superimposes an overlay string (toast) over a base string.
func overlayBase(base, overlay string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	res := make([]string, len(baseLines))
	for i, baseLine := range baseLines {
		if i < len(overlayLines) {
			oLine := overlayLines[i]
			oWidth := lipgloss.Width(oLine)
			availWidth := width - oWidth
			if availWidth < 0 {
				availWidth = 0
			}
			truncated := truncateANSI(baseLine, availWidth)
			actualWidth := lipgloss.Width(truncated)
			padding := ""
			if actualWidth < availWidth {
				padding = strings.Repeat(" ", availWidth-actualWidth)
			}
			res[i] = truncated + padding + oLine
		} else {
			res[i] = baseLine
		}
	}
	return strings.Join(res, "\n")
}

// dimBackground strips ANSI styling from all lines and renders them in muted
// gray. Used to visually de-emphasise the main UI when a modal popup is open.
func dimBackground(base string) string {
	lines := strings.Split(base, "\n")
	for i, line := range lines {
		plain := ansiPattern.ReplaceAllString(line, "")
		lines[i] = styles.GrayStyle.Render(plain)
	}
	return strings.Join(lines, "\n")
}

// overlayCenter superimposes an overlay string (popup modal) centered vertically and horizontally over a base string.
func overlayCenter(base, overlay string, width, height int) string {
	// Dim background when a modal is shown.
	base = dimBackground(base)

	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}

	popupHeight := len(overlayLines)
	popupWidth := 0
	for _, oLine := range overlayLines {
		w := lipgloss.Width(oLine)
		if w > popupWidth {
			popupWidth = w
		}
	}

	startY := (len(baseLines) - popupHeight) / 2
	if startY < 0 {
		startY = 0
	}

	startX := (width - popupWidth) / 2
	if startX < 0 {
		startX = 0
	}

	res := make([]string, len(baseLines))
	for i, baseLine := range baseLines {
		if i >= startY && i < startY+popupHeight {
			oLine := overlayLines[i-startY]
			oWidth := lipgloss.Width(oLine)

			leftPart := truncateANSI(baseLine, startX)
			leftWidth := lipgloss.Width(leftPart)
			leftPadding := ""
			if leftWidth < startX {
				leftPadding = strings.Repeat(" ", startX-leftWidth)
			}

			rightStart := startX + oWidth
			rightPart := rightPartANSI(baseLine, rightStart)
			rightWidth := lipgloss.Width(rightPart)
			rightPadding := ""
			if rightStart+rightWidth < width {
				rightPadding = strings.Repeat(" ", width-(rightStart+rightWidth))
			}

			res[i] = leftPart + leftPadding + oLine + rightPart + rightPadding
		} else {
			res[i] = baseLine
		}
	}
	return strings.Join(res, "\n")
}

func rightPartANSI(str string, startCol int) string {
	runes := []rune(str)
	var out strings.Builder
	col := 0
	i := 0
	for i < len(runes) {
		r := runes[i]
		if r == '\x1b' {
			j := i + 1
			for j < len(runes) {
				if (runes[j] >= 'a' && runes[j] <= 'z') || (runes[j] >= 'A' && runes[j] <= 'Z') {
					j++
					break
				}
				j++
			}
			out.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			rw = 1
		}
		if col >= startCol {
			out.WriteRune(r)
		}
		col += rw
		i++
	}
	return out.String()
}

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
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
