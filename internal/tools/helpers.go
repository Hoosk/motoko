package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	workspaceignore "github.com/Hoosk/motoko/internal/ignore"
)

func parseJSONArgs(args string) map[string]any {
	args = strings.TrimSpace(args)
	if !strings.HasPrefix(args, "{") {
		return nil
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return nil
	}
	return parsed
}

func jsonStr(m map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func jsonRawStr(m map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok {
			return text
		}
	}
	return ""
}

func jsonInt(m map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		switch n := value.(type) {
		case float64:
			return int(n), true
		case string:
			i, err := strconv.Atoi(strings.TrimSpace(n))
			if err == nil {
				return i, true
			}
		}
	}
	return 0, false
}

func jsonHas(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func resolveWorkspacePath(target string) (string, string, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	if target == "" {
		return workspace, ".", nil
	}

	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	path = filepath.Clean(path)

	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path outside workspace: %s", target)
	}
	if rel == "." {
		return path, rel, nil
	}

	return path, filepath.ToSlash(rel), nil
}

func walkWorkspace(ctx context.Context, fn func(relPath, absPath string, entry fs.DirEntry) error) error {
	workspace, _, err := resolveWorkspacePath("")
	if err != nil {
		return err
	}
	matcher, err := workspaceignore.Load(workspace)
	if err != nil {
		return err
	}

	cfg := GetConfig(ctx)
	var excludePatterns []string
	if cfg != nil {
		excludePatterns = cfg.Search.ExcludePatterns
	}

	return filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		rel = filepath.ToSlash(rel)
		if matcher.Ignored(rel, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && (rel == ".git" || strings.HasPrefix(rel, ".git/")) {
			return filepath.SkipDir
		}

		// Apply custom exclude patterns
		for _, pat := range excludePatterns {
			if matched, _ := filepath.Match(pat, rel); matched {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		return fn(rel, path, entry)
	})
}

func compileGlob(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" {
		return nil, fmt.Errorf("empty pattern")
	}

	var out strings.Builder
	out.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				out.WriteString(".*")
				i++
			} else {
				out.WriteString("[^/]*")
			}
		case '?':
			out.WriteString("[^/]")
		case '.':
			out.WriteString("\\.")
		case '/', '\\':
			out.WriteString("/")
		default:
			if strings.ContainsRune("+()|[]{}^$", rune(pattern[i])) {
				out.WriteByte('\\')
			}
			out.WriteByte(pattern[i])
		}
	}
	out.WriteString("$")

	return regexp.Compile(out.String())
}

func isTextFile(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()

	buffer := make([]byte, 8192)
	n, err := file.Read(buffer)
	if err != nil && err.Error() != "EOF" {
		return false
	}
	chunk := buffer[:n]
	if bytesContainsZero(chunk) {
		return false
	}
	return utf8.Valid(chunk)
}

func bytesContainsZero(data []byte) bool {
	return slices.Contains(data, 0)
}
