package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) enqueuePrompt(prompt string) {
	m.promptQueue = append(m.promptQueue, prompt)
	m.queueSel = clamp(m.queueSel, max(0, len(m.promptQueue)-1))
}

func (m *Model) dequeuePrompt() (string, bool) {
	if len(m.promptQueue) == 0 {
		m.queueSel = 0
		m.queueFocus = false
		return "", false
	}
	prompt := m.promptQueue[0]
	m.promptQueue = append([]string(nil), m.promptQueue[1:]...)
	if len(m.promptQueue) == 0 {
		m.queueSel = 0
		m.queueFocus = false
	} else {
		m.queueSel = clamp(m.queueSel, len(m.promptQueue)-1)
	}
	return prompt, true
}

func (m *Model) removeQueuedAt(index int) {
	if index < 0 || index >= len(m.promptQueue) {
		return
	}
	m.promptQueue = append(m.promptQueue[:index], m.promptQueue[index+1:]...)
	if len(m.promptQueue) == 0 {
		m.queueSel = 0
		m.queueFocus = false
		return
	}
	m.queueSel = clamp(m.queueSel, len(m.promptQueue)-1)
}

func (m *Model) moveQueued(index, delta int) {
	if index < 0 || index >= len(m.promptQueue) {
		return
	}
	target := clamp(index+delta, len(m.promptQueue)-1)
	if target == index {
		return
	}
	m.promptQueue[index], m.promptQueue[target] = m.promptQueue[target], m.promptQueue[index]
	m.queueSel = target
}

func (m Model) queuePanelHeight(width int) int {
	if len(m.promptQueue) == 0 || width <= 0 {
		return 0
	}
	return lipgloss.Height(m.renderQueuePanel(width))
}

func (m Model) renderQueuePanel(width int) string {
	if len(m.promptQueue) == 0 || width <= 0 {
		return ""
	}
	contentWidth := max(width-4, 0)
	header := styles.WarmGoldStyle.Render("Queue") + " " + styles.GrayStyle.Render("("+strconv.Itoa(len(m.promptQueue))+")")
	if m.queueFocus {
		header += "  " + styles.BoldNeonStyle.Render("Ctrl+Up/Down reorder • Backspace delete • Esc close")
	} else {
		header += "  " + styles.GrayStyle.Render("Ctrl+Q manage")
	}
	lines := []string{header}
	for i, prompt := range m.promptQueue {
		line := strconv.Itoa(i+1) + ". " + strings.TrimSpace(prompt)
		if contentWidth > 6 && lipgloss.Width(line) > contentWidth {
			line = truncate(line, contentWidth)
		}
		if m.queueFocus && i == m.queueSel {
			lines = append(lines, styles.PopupSelectionStyle.Render(line))
		} else {
			lines = append(lines, styles.GrayStyle.Render(line))
		}
	}
	body := styles.InputChromeStyle.Width(contentWidth).Render(strings.Join(lines, "\n"))
	separator := styles.GrayStyle.Width(contentWidth).Render(strings.Repeat("─", contentWidth))
	return lipgloss.JoinVertical(lipgloss.Left, separator, body)
}

func (m *Model) nextPromptAfterAgent() (string, bool) {
	if br := m.runtime.GetBrain(); br != nil && br.Exists("goal") {
		pending, completed := br.TaskCounts()
		if !br.Exists("tasks") {
			return "[System: Goal still active. No tasks.md exists yet. Create or refine tasks.md first, then continue.]", true
		}
		if pending > 0 {
			attempts, lastPending := readGoalState(br)
			if lastPending == pending {
				attempts++
			} else {
				attempts = 1
			}
			_ = writeGoalState(br, attempts, pending)
			if attempts > 20 {
				_ = br.Delete("goal")
				_ = br.Delete("goal_state")
				m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: "Goal auto-continue stopped after 20 attempts without progress."})
				m.timeline.renderMessages()
				return m.dequeuePrompt()
			}
			return "[System: Goal still active. Continue with the next unfinished task in tasks.md.]", true
		}
		if pending == 0 && completed == 0 {
			attempts, lastPending := readGoalState(br)
			if lastPending == 0 {
				attempts++
			} else {
				attempts = 1
			}
			_ = writeGoalState(br, attempts, 0)
			if attempts > 20 {
				_ = br.Delete("goal")
				_ = br.Delete("goal_state")
				m.timeline.appendEntry(app.Entry{Kind: app.EntryError, Text: "Goal auto-continue stopped after 20 attempts without a usable tasks.md checklist."})
				m.timeline.renderMessages()
				return m.dequeuePrompt()
			}
			return "[System: Goal still active, but tasks.md has no checkboxes yet. Refine tasks.md first, then continue.]", true
		}
		_ = br.Delete("goal_state")
		_ = br.Delete("goal")
		m.timeline.appendEntry(app.Entry{Kind: app.EntrySystem, Text: "Goal completed."})
		m.timeline.renderMessages()
	}
	return m.dequeuePrompt()
}

func readGoalState(br *brain.Brain) (attempts int, lastPending int) {
	if br == nil {
		return 0, -1
	}
	content, err := br.Read("goal_state")
	if err != nil {
		return 0, -1
	}
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "attempts:") {
			_, _ = fmt.Sscanf(line, "attempts: %d", &attempts)
		}
		if strings.HasPrefix(line, "pending:") {
			_, _ = fmt.Sscanf(line, "pending: %d", &lastPending)
		}
	}
	return attempts, lastPending
}

func writeGoalState(br *brain.Brain, attempts int, pending int) error {
	if br == nil {
		return nil
	}
	return br.Write("goal_state", fmt.Sprintf("attempts: %d\npending: %d\n", attempts, pending))
}

func selectModelAndBudget(runtime *app.Runtime, model provider.ModelInfo, budget int) tea.Cmd {
	return func() tea.Msg {
		if err := runtime.SetActiveModelInfo(model); err != nil {
			return ErrorMsg{Err: err}
		}
		if err := runtime.SetThinkingBudget(budget); err != nil {
			return ErrorMsg{Err: err}
		}
		return NotificationMsg{Text: "Model updated: " + model.ID}
	}
}
