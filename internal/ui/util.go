package ui

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var thinkingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const logoArt = `
  __  __  ____ _____ ____  _  _____
 |  \/  |/ __ \_   _/ __ \| |/ / _ \
 | \  / | |  | || || |  | | ' / | | |
 | |\/| | |  | || || |  | |  <| | | |
 | |  | | |__| || || |__| | . \ |_| |
 |_|  |_|\____/ |_| \____/|_|\_\___/
`

func writeClipboard(text string) error {
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
	return fmt.Errorf("no se pudo copiar: instala wl-copy, xclip o xsel si el backend actual falla")
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

func renderTachikomaList(statuses map[string]string) string {
	if len(statuses) == 0 {
		return styles.SystemStyle.Render("No background workers active.")
	}
	
	names := make([]string, 0, len(statuses))
	for name := range statuses {
		names = append(names, name)
	}
	sort.Strings(names)
	
	whiteStyle := lipgloss.NewStyle().Foreground(styles.White)
	grayStyle := lipgloss.NewStyle().Foreground(styles.Gray)
	neonStyle := lipgloss.NewStyle().Foreground(styles.MainNeon)
	pinkStyle := lipgloss.NewStyle().Foreground(styles.AlertPink)

	var lines []string
	for _, name := range names {
		status := statuses[name]
		indicator := neonStyle.Render("●")
		if strings.Contains(strings.ToLower(status), "error") || strings.Contains(strings.ToLower(status), "fail") {
			indicator = pinkStyle.Render("●")
		}
		
		line := fmt.Sprintf("%s %-15s %s", indicator, whiteStyle.Render(name), grayStyle.Render(status))
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

func estimateTextareaHeight(value string, width int) int {
	if width <= 1 {
		return 3
	}
	lines := strings.Split(value, "\n")
	count := 0
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth == 0 {
			count++
			continue
		}
		count += (lineWidth-1)/width + 1
	}
	return max(3, count)
}

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

// wrapText wraps text to fit within width visible columns, respecting
// existing \n characters. Word-boundary aware; falls back to character
// breaks for words longer than width. Unicode-aware via go-runewidth.
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, wrapOneLine(line, width))
	}
	return strings.Join(result, "\n")
}

// wrapOneLine wraps a single line at word boundaries to fit within width cols.
func wrapOneLine(line string, width int) string {
	if runewidth.StringWidth(line) <= width {
		return line
	}
	runes := []rune(line)
	var out strings.Builder
	col := 0

	for i := 0; i < len(runes); {
		r := runes[i]
		if r == ' ' || r == '\t' {
			rw := runewidth.RuneWidth(r)
			// Emit space only if we have content on the line and it fits.
			if col > 0 && col+rw <= width {
				out.WriteRune(r)
				col += rw
			}
			i++
			continue
		}
		// Measure the next word.
		j, wordW := i, 0
		for j < len(runes) && runes[j] != ' ' && runes[j] != '\t' {
			wordW += runewidth.RuneWidth(runes[j])
			j++
		}
		word := runes[i:j]
		switch {
		case col == 0:
			// At start of a (possibly new) line: write with force-breaks if needed.
			for _, wr := range word {
				wrW := runewidth.RuneWidth(wr)
				if col+wrW > width && col > 0 {
					out.WriteByte('\n')
					col = 0
				}
				out.WriteRune(wr)
				col += wrW
			}
		case col+wordW <= width:
			out.WriteString(string(word))
			col += wordW
		default:
			// Word doesn't fit: wrap and write (with force-breaks for very long words).
			out.WriteByte('\n')
			col = 0
			for _, wr := range word {
				wrW := runewidth.RuneWidth(wr)
				if col+wrW > width && col > 0 {
					out.WriteByte('\n')
					col = 0
				}
				out.WriteRune(wr)
				col += wrW
			}
		}
		i = j
	}
	return out.String()
}
