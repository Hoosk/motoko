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
	patterns     []ignorePattern
}

// ignorePattern represents a single parsed .gitignore pattern used as
// a fallback when git ls-files is not available.
type ignorePattern struct {
	pattern string
	base    string
	dirOnly bool
	rooted  bool
}

// parseIgnoreLine parses a single line from a .gitignore file.
// base is the relative path (from workspace root) of the directory that
// contains the .gitignore file (empty string for the root).
// Returns (pattern, true) if the line represents a pattern we should act on.
func parseIgnoreLine(line, base string) (ignorePattern, bool) {
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
		return ignorePattern{}, false
	}
	p := ignorePattern{base: base}
	p.dirOnly = strings.HasSuffix(line, "/")
	line = strings.TrimSuffix(line, "/")
	// A leading / means anchored to the .gitignore's directory.
	// An interior / (not leading/trailing) also means anchored.
	stripped := strings.TrimPrefix(line, "/")
	p.rooted = line != stripped || strings.Contains(stripped, "/")
	line = stripped
	p.pattern = line
	return p, p.pattern != ""
}

// matches reports whether the given relative path (from workspace root)
// is matched by this pattern. isDir indicates whether the path is a directory.
func (p ignorePattern) matches(relPath string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}
	// Restrict to paths under the .gitignore's base directory.
	target := relPath
	if p.base != "" {
		prefix := p.base + "/"
		if !strings.HasPrefix(relPath, prefix) {
			return false
		}
		target = relPath[len(prefix):]
	}
	if p.rooted {
		// Anchored: pattern must match the path from the base dir.
		matched, _ := filepath.Match(p.pattern, target)
		return matched
	}
	// Non-anchored: pattern can match the base name or any path suffix.
	parts := strings.Split(target, "/")
	for i := range parts {
		suffix := strings.Join(parts[i:], "/")
		if matched, _ := filepath.Match(p.pattern, suffix); matched {
			return true
		}
	}
	return false
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
	// Pattern-based check (from .gitignore fallback parser).
	for _, p := range m.patterns {
		if p.matches(relPath, isDir) {
			return true
		}
	}
	// Check parent directories against patterns (handles files under ignored dirs).
	for dir := parentRelativePath(relPath); dir != ""; dir = parentRelativePath(dir) {
		for _, p := range m.patterns {
			if p.matches(dir, true) {
				return true
			}
		}
	}
	return false
}

func (m *Matcher) loadGitIgnored(root string) {
	cmd := exec.Command("git", "ls-files", "--others", "-i", "--exclude-standard", "--directory")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		// Git is not available or root is not a git repo: fall back to
		// parsing .gitignore files directly.
		m.loadGitIgnoreFiles(root)
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

// maxGitIgnoreWalkDepth is the maximum directory depth we recurse into
// when searching for nested .gitignore files in the fallback path.
const maxGitIgnoreWalkDepth = 6

// loadGitIgnoreFiles walks the workspace tree and parses every .gitignore
// file found up to maxGitIgnoreWalkDepth deep.
func (m *Matcher) loadGitIgnoreFiles(root string) {
	m.walkForGitIgnore(root, "", 0)
}

func (m *Matcher) walkForGitIgnore(dir, relBase string, depth int) {
	if depth > maxGitIgnoreWalkDepth {
		return
	}
	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		m.loadSingleGitIgnoreFile(gitignorePath, relBase)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, ok := fixedIgnoredDirs[name]; ok {
			continue
		}
		childRel := name
		if relBase != "" {
			childRel = relBase + "/" + name
		}
		m.walkForGitIgnore(filepath.Join(dir, name), childRel, depth+1)
	}
}

// loadSingleGitIgnoreFile parses one .gitignore file and appends the
// resulting patterns to m.patterns.  base is the relative path (from the
// workspace root) of the directory that contains the file.
func (m *Matcher) loadSingleGitIgnoreFile(path, base string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		p, ok := parseIgnoreLine(line, base)
		if !ok {
			continue
		}
		m.patterns = append(m.patterns, p)
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
