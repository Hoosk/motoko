package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	workspaceignore "github.com/Hoosk/motoko/internal/ignore"
)

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
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
