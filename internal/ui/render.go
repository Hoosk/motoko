package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderComposer() string {
	prompt := m.renderInputPrompt()
	rows := max(3, m.textarea.Height())
	promptLines := make([]string, rows)
	for i := range promptLines {
		if i == 0 {
			promptLines[i] = prompt
		} else {
			promptLines[i] = " "
		}
	}
	
	promptBlock := lipgloss.NewStyle().Width(3).Render(strings.Join(promptLines, "\n"))
	
	// Add some visual separation for the suggestions line
	suggestions := m.renderSuggestionsLine()
	suggestionsBlock := lipgloss.NewStyle().MarginTop(1).Render(suggestions)

	// Combine prompt and textarea
	body := lipgloss.JoinHorizontal(lipgloss.Top, promptBlock, styles.InputStyle.Render(m.textarea.View()))
	
	return styles.InputChromeStyle.Width(m.width - 4).Render(
		lipgloss.JoinVertical(lipgloss.Left, body, suggestionsBlock),
	)
}

func (m Model) renderInputPrompt() string {
	if m.runtime.InputMode() == app.InputModeShell {
		return lipgloss.NewStyle().Foreground(styles.WarmGold).Bold(true).Render("$")
	}
	return lipgloss.NewStyle().Foreground(styles.MainNeon).Bold(true).Render(">")
}

func (m Model) renderSuggestionsLine() string {
	if m.thinking {
		return styles.InputHintStyle.Render("Esperando respuesta del agente...")
	}
	if len(m.suggestions) == 0 {
		if m.runtime.InputMode() == app.InputModeShell {
			return styles.InputHintStyle.Render("Shell directo activo. Enter ejecuta. /chat sale. Ctrl+T abre la paleta.")
		}
		return styles.InputHintStyle.Render("Tab completa. /provider add abre el formulario. /models tiene autocompletado con modelos cacheados.")
	}
	limit := min(3, len(m.suggestions))
	items := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		if i == m.selectedSuggestion {
			items = append(items, styles.SelectionStyle.Render(m.suggestions[i]))
			continue
		}
		items = append(items, styles.SuggestionStyle.Render(m.suggestions[i]))
	}
	return strings.Join(items, "   ")
}

func (m Model) renderFooter() string {
	ws := styles.WorkspaceStyle.Render("workspace: " + m.sysInfo.Workspace)
	git := styles.GitStyle.Render("git: " + m.sysInfo.GitSummary())
	pending := styles.SystemStyle.Render("pending: " + pendingLabel(m.runtime.PendingApproval()))
	provider := styles.SystemStyle.Render("provider: " + m.runtime.ProviderSummary())

	// Give the footer more structure and spacing
	parts := []string{ws, git, pending, provider}
	return styles.FooterStyle.Width(m.width - 4).Render(strings.Join(parts, "  │  "))
}

func (m Model) renderToolPalette() string {
	sections := []string{styles.PopupTitleStyle.Render("Tools"), styles.PopupMutedStyle.Render("Ctrl+T cierra esta paleta. Usa /tool <nombre> <args> para ejecutarlas."), renderToolList(m.runtime.ToolSpecs())}
	if m.showTachikomas {
		sections = append(sections, "", styles.PopupTitleStyle.Render("Tachikomas"), renderTachikomaList(m.tachikomaInfo))
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

func pendingLabel(pending string) string {
	if pending == "" {
		return "none"
	}
	return pending
}
