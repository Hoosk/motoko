package timeline

import (
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/styles"
)

const (
	TimelineMouseOffsetX = 1
	TimelineMouseOffsetY = 1
	AssistantContentX    = 2
	ReasoningContentX    = 2
	UserContentX         = 4
)

var ThinkingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var LogoArt = "███╗   ███╗ ██████╗ ████████╗ ██████╗ ██╗  ██╗ ██████╗\n" +
	"████╗ ████║██╔═══██╗╚══██╔══╝██╔═══██╗██║ ██╔╝██╔═══██╗\n" +
	"██╔████╔██║██║   ██║   ██║   ██║   ██║█████╔╝ ██║   ██║\n" +
	"██║╚██╔╝██║██║   ██║   ██║   ██║   ██║██╔═██╗ ██║   ██║\n" +
	"██║ ╚═╝ ██║╚██████╔╝   ██║   ╚██████╔╝██║  ██╗╚██████╔╝\n" +
	"╚═╝     ╚═╝ ╚═════╝    ╚═╝    ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ "

func (m *Model) RenderEntry(entry app.Entry) string {
	switch entry.Kind {
	case app.EntryUser:
		return RenderUserMessage(entry.Text, max(20, m.Viewport.Width))
	case app.EntryAssistant:
		wrapped := WrapText(entry.Text, m.AssistantInnerWidth())
		return renderAssistantMessage(wrapped)
	case app.EntryReasoning:
		wrapped := WrapText(entry.Text, m.AssistantInnerWidth())
		return renderReasoningMessage(wrapped)
	case app.EntrySystem:
		return styles.SystemStyle.Render(entry.Text)
	case app.EntryCommand:
		return styles.CommandStyle.Render(entry.Text)
	case app.EntryOutput:
		return RenderDiffOutput(entry.Text)
	case app.EntryError:
		return styles.ErrorStyle.Render(entry.Text)
	case app.EntryHelp:
		return RenderHelpEntry(entry.Text)
	default:
		return entry.Text
	}
}

func (m *Model) AssistantOuterWidth() int {
	return max(40, m.Viewport.Width)
}

func (m *Model) AssistantInnerWidth() int {
	return max(37, m.AssistantOuterWidth()-3)
}

func RenderUserMessage(text string, width int) string {
	w := max(20, width)

	wrapped := WrapText(text, w-5)
	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = " " + styles.UserPromptStyle.Render(">") + "  " + styles.WhiteStyle.Render(line)
		} else {
			lines[i] = "    " + styles.WhiteStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

func renderAssistantMessage(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = styles.NeonStyle.Render("▎") + " " + styles.AssistantBlockStyle.Render(line)
	}
	return strings.Join(lines, "\n")
}

func renderReasoningMessage(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "  " + styles.ReasoningBlockStyle.Render(line)
	}
	return strings.Join(lines, "\n")
}

func RenderHelpEntry(text string) string {
	lines := strings.Split(text, "\n")
	titleStyle := styles.BoldNeonStyle
	cmdStyle := styles.BoldBlueStyle
	descStyle := styles.GrayStyle

	var rendered []string
	for i, line := range lines {
		if i == 0 {
			rendered = append(rendered, titleStyle.Render(line))
			continue
		}
		if line == "" {
			rendered = append(rendered, "")
			continue
		}
		// Split command name from description at first double-space run.
		idx := strings.Index(line, "  ")
		if idx <= 0 {
			rendered = append(rendered, descStyle.Render(line))
			continue
		}
		cmd := line[:idx]
		desc := strings.TrimSpace(line[idx:])
		rendered = append(rendered, cmdStyle.Render(cmd)+"  "+descStyle.Render(desc))
	}
	return strings.Join(rendered, "\n")
}

func RenderDiffOutput(text string) string {
	lines := strings.Split(text, "\n")
	isDiff := false
	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "@@ ") {
			isDiff = true
			break
		}
	}
	if !isDiff {
		return styles.OutputStyle.Render(text)
	}

	changedCount := 0
	for _, line := range lines {
		if len(line) > 0 {
			if line[0] == '+' && !strings.HasPrefix(line, "+++ ") {
				changedCount++
			} else if line[0] == '-' && !strings.HasPrefix(line, "--- ") {
				changedCount++
			}
		}
	}

	if changedCount > 20 {
		var result []string
		for _, line := range lines {
			if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
				result = append(result, styles.DiffMetaStyle.Render(line))
			} else if strings.HasPrefix(line, "@@ ") {
				result = append(result, styles.DiffHeaderStyle.Render(line))
			}
		}
		result = append(result, styles.DiffMetaStyle.Render(fmt.Sprintf("... (%d lines changed, collapsed)", changedCount)))
		return strings.Join(result, "\n")
	}

	var result []string
	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			result = append(result, styles.DiffMetaStyle.Render(line))
		} else if strings.HasPrefix(line, "@@ ") {
			result = append(result, styles.DiffHeaderStyle.Render(line))
		} else if len(line) > 0 && line[0] == '+' {
			result = append(result, styles.DiffAddStyle.Render(line))
		} else if len(line) > 0 && line[0] == '-' {
			result = append(result, styles.DiffRemoveStyle.Render(line))
		} else {
			result = append(result, styles.DiffContextStyle.Render(line))
		}
	}
	return strings.Join(result, "\n")
}
