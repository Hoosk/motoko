package patch

import (
	"strings"

	udiff "github.com/aymanbagabas/go-udiff"
)

func UnifiedDiff(path, old, updated string) string {
	if old == updated {
		return ""
	}
	path = strings.TrimSpace(path)
	return udiff.Unified("a/"+path, "b/"+path, old, updated)
}
