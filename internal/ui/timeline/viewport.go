package timeline

import (
	"strings"
)

func (m *Model) MaxViewportOffset() int {
	if m.Viewport.Height <= 0 || m.ViewportContent == "" {
		return 0
	}
	lineCount := strings.Count(m.ViewportContent, "\n") + 1
	return max(0, lineCount-m.Viewport.Height)
}
