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
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var thinkingFrames = []string{"thinking.", "thinking..", "thinking..."}

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

func renderToolPalette(specs []tools.Spec, showTachikomas bool, tachikomaInfo map[string]string) string {
	sections := []string{styles.PopupTitleStyle.Render("Tools"), styles.PopupMutedStyle.Render("Ctrl+T cierra esta paleta. Usa /tool <nombre> <args> para ejecutarlas."), renderToolList(specs)}
	if showTachikomas {
		sections = append(sections, "", styles.PopupTitleStyle.Render("Tachikomas"), renderTachikomaList(tachikomaInfo))
	}
	return strings.Join(sections, "\n")
}

func renderToolList(specs []tools.Spec) string {
	lines := make([]string, 0, len(specs))
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("%s\n  %s\n  %s", styles.SelectionStyle.Render(spec.Name), styles.PopupMutedStyle.Render(spec.Summary), styles.SystemStyle.Render(spec.Usage)))
	}
	return strings.Join(lines, "\n\n")
}

func renderTachikomaList(statuses map[string]string) string {
	if len(statuses) == 0 {
		return styles.SystemStyle.Render("Sin datos.")
	}
	names := make([]string, 0, len(statuses))
	for name := range statuses {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, styles.SelectionStyle.Render(name)+"\n"+styles.SystemStyle.Render(statuses[name]))
	}
	return strings.Join(lines, "\n\n")
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
