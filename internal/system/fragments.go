package system

import (
	"embed"
	"path/filepath"
)

//go:embed fragments/*.md
var fragmentsFS embed.FS

// LoadFragment returns the contents of a fragment markdown file.
// Fragments are injected as extra messages into the chat array rather than system prompt.
func LoadFragment(name string) string {
	data, err := fragmentsFS.ReadFile(filepath.Join("fragments", name+".md"))
	if err != nil {
		return ""
	}
	return string(data)
}
