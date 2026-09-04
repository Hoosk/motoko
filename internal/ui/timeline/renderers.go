package timeline

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

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
		return renderAssistantMarkdown(entry.Text, m.AssistantInnerWidth())
	case app.EntryReasoning:
		wrapped := WrapText(entry.Text, m.AssistantInnerWidth())
		return renderReasoningMessage(wrapped)
	case app.EntrySystem:
		return styles.SystemStyle.Render(entry.Text)
	case app.EntryCommand:
		return styles.CommandStyle.Render(entry.Text)
	case app.EntryOutput:
		return RenderDiffOutput(entry.Text, m.diffRenderWidth())
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

func (m *Model) diffRenderWidth() int {
	return max(20, m.Viewport.Width-2)
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

const (
	DiffWrapMarker = "⏎"
	diffNoNewline  = `\ No newline at end of file`
)

func RenderDiffOutput(text string, width int) string {
	return renderDiffOutput(text, width, true)
}

func RenderFullDiffOutput(text string, width int) string {
	return renderDiffOutput(text, width, false)
}

type diffCounts struct {
	path    string
	adds    int
	removes int
}

func renderDiffOutput(text string, width int, collapseLarge bool) string {
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

	counts := diffScan(lines)

	if collapseLarge && counts.adds+counts.removes > 20 {
		var result []string
		if counts.path != "" {
			result = append(result, diffMetaSummary(counts))
		}
		for _, line := range lines {
			if strings.HasPrefix(line, "@@ ") {
				result = append(result, styles.DiffHeaderStyle.Render(line))
			}
		}
		result = append(result, styles.DiffMetaStyle.Render(fmt.Sprintf("... (%d lines changed, collapsed)", counts.adds+counts.removes)))
		return strings.Join(result, "\n")
	}

	addMarker := styles.DiffAddStyle.Bold(true).Render("+")
	removeMarker := styles.DiffRemoveStyle.Bold(true).Render("-")
	wrapMarker := styles.DiffMetaStyle.Render(DiffWrapMarker + " ")

	var result []string
	if counts.path != "" {
		result = append(result, diffMetaSummary(counts))
	}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
			continue
		case strings.HasPrefix(line, "@@ "):
			result = appendDiffLine(result, line, "", wrapMarker, styles.DiffHeaderStyle, width)
		case strings.HasPrefix(line, diffNoNewline):
			result = append(result, styles.DiffMetaStyle.Render("(no newline at end of file)"))
		case len(line) > 0 && line[0] == '+':
			result = appendDiffLine(result, line[1:], addMarker, wrapMarker, styles.DiffAddStyle, width)
		case len(line) > 0 && line[0] == '-':
			result = appendDiffLine(result, line[1:], removeMarker, wrapMarker, styles.DiffRemoveStyle, width)
		default:
			content := line
			if len(content) > 0 && content[0] == ' ' {
				content = content[1:]
			}
			result = appendDiffLine(result, content, " ", wrapMarker, styles.DiffContextStyle, width)
		}
	}
	return strings.Join(result, "\n")
}

func diffScan(lines []string) diffCounts {
	var counts diffCounts
	oldPath, newPath := "", ""
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "--- "):
			oldPath = diffHeaderPath(line)
		case strings.HasPrefix(line, "+++ "):
			newPath = diffHeaderPath(line)
		case strings.HasPrefix(line, "@@ "):
		default:
			if len(line) > 0 {
				switch line[0] {
				case '+':
					counts.adds++
				case '-':
					counts.removes++
				}
			}
		}
	}
	counts.path = newPath
	if counts.path == "" {
		counts.path = oldPath
	}
	return counts
}

func diffHeaderPath(line string) string {
	path := strings.TrimSpace(line[4:])
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	if path == "/dev/null" {
		return ""
	}
	return path
}

func diffMetaSummary(counts diffCounts) string {
	return styles.DiffMetaStyle.Render(counts.path) + " " +
		styles.DiffAddStyle.Render(fmt.Sprintf("+%d", counts.adds)) + " " +
		styles.DiffRemoveStyle.Render(fmt.Sprintf("-%d", counts.removes))
}

func appendDiffLine(result []string, content, firstPrefix, wrapPrefix string, style lipgloss.Style, width int) []string {
	firstWidth := max(width-lipgloss.Width(firstPrefix), 0)
	wrapWidth := max(width-lipgloss.Width(wrapPrefix), 0)
	segments := wrapDiffContent(content, firstWidth, wrapWidth)
	result = append(result, firstPrefix+style.Render(segments[0]))
	for _, segment := range segments[1:] {
		result = append(result, wrapPrefix+style.Render(segment))
	}
	return result
}

func wrapDiffContent(content string, firstWidth, wrapWidth int) []string {
	if firstWidth <= 0 || wrapWidth <= 0 {
		return []string{content}
	}
	runes := []rune(content)
	segments := make([]string, 0, 1+len(runes)/max(wrapWidth, 1))
	budget, start, col := firstWidth, 0, 0
	for i, r := range runes {
		rw := runewidth.RuneWidth(r)
		if col+rw > budget {
			segments = append(segments, string(runes[start:i]))
			start, col, budget = i, 0, wrapWidth
		}
		col += rw
	}
	return append(segments, string(runes[start:]))
}
