package timeline

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
)

var (
	mdMu       sync.Mutex
	mdRenderer *glamour.TermRenderer
	mdWidth    int
)

func markdownRenderer(width int) *glamour.TermRenderer {
	mdMu.Lock()
	defer mdMu.Unlock()
	if mdRenderer != nil && mdWidth == width {
		return mdRenderer
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return nil
	}
	mdRenderer, mdWidth = renderer, width
	return mdRenderer
}

func renderAssistantMarkdown(text string, width int) string {
	renderer := markdownRenderer(max(20, width-2))
	if renderer == nil {
		return renderAssistantMessage(text)
	}
	mdMu.Lock()
	rendered, err := renderer.Render(text)
	mdMu.Unlock()
	if err != nil {
		return renderAssistantMessage(text)
	}
	rendered = strings.Trim(rendered, "\n")
	lines := strings.Split(rendered, "\n")
	for i := range lines {
		// Glamour adds a document margin; the timeline already owns the gutter.
		lines[i] = trimMarkdownMargin(lines[i])
	}
	return renderAssistantMessage(strings.Join(lines, "\n"))
}

func trimMarkdownMargin(line string) string {
	for removed := 0; removed < 2; removed++ {
		if !strings.HasPrefix(StripANSI(line), " ") {
			return line
		}
		prefixEnd := 0
		for prefixEnd < len(line) {
			match := ansiPattern.FindStringIndex(line[prefixEnd:])
			if match == nil || match[0] != 0 {
				break
			}
			prefixEnd += match[1]
		}
		if prefixEnd < len(line) && line[prefixEnd] == ' ' {
			line = line[:prefixEnd] + line[prefixEnd+1:]
			continue
		}
		return line
	}
	return line
}
