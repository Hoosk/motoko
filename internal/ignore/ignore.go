package ignore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var fixedIgnoredDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
}

type Matcher struct {
	ignoredFiles map[string]struct{}
	ignoredDirs  map[string]struct{}
}

func Load(root string) (*Matcher, error) {
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	root = filepath.Clean(root)
	matcher := &Matcher{
		ignoredFiles: make(map[string]struct{}),
		ignoredDirs:  make(map[string]struct{}),
	}
	matcher.loadGitIgnored(root)
	return matcher, nil
}

func (m *Matcher) Ignored(relPath string, isDir bool) bool {
	if m == nil {
		return false
	}
	relPath = normalizeRelativePath(relPath)
	if relPath == "" {
		return false
	}
	if hasFixedIgnoredComponent(relPath) {
		return true
	}
	if _, ok := m.ignoredFiles[relPath]; ok {
		return true
	}
	if isDir {
		if _, ok := m.ignoredDirs[relPath]; ok {
			return true
		}
	}
	for dir := relPath; dir != "" && dir != "."; dir = parentRelativePath(dir) {
		if _, ok := m.ignoredDirs[dir]; ok {
			return true
		}
	}
	return false
}

func (m *Matcher) loadGitIgnored(root string) {
	cmd := exec.Command("git", "ls-files", "--others", "-i", "--exclude-standard", "--directory")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for _, rawLine := range strings.Split(string(out), "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}
		line := normalizeRelativePath(strings.TrimSuffix(rawLine, "/"))
		if line == "" {
			continue
		}
		if strings.HasSuffix(rawLine, "/") {
			m.ignoredDirs[line] = struct{}{}
			continue
		}
		m.ignoredFiles[line] = struct{}{}
	}
}

func hasFixedIgnoredComponent(relPath string) bool {
	for _, part := range strings.Split(relPath, "/") {
		if _, ok := fixedIgnoredDirs[part]; ok {
			return true
		}
	}
	return false
}

func normalizeRelativePath(relPath string) string {
	relPath = strings.TrimSpace(filepath.ToSlash(filepath.Clean(relPath)))
	if relPath == "." {
		return ""
	}
	relPath = strings.TrimPrefix(relPath, "./")
	return relPath
}

func parentRelativePath(relPath string) string {
	if relPath == "" {
		return ""
	}
	parent := filepath.ToSlash(filepath.Dir(relPath))
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}
