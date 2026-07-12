package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	patchtool "github.com/Hoosk/motoko/internal/tools/patch"
)

type WriteTool struct{}

func NewWriteTool() *WriteTool {
	return &WriteTool{}
}

func (t *WriteTool) Spec() Spec {
	return Spec{
		Name:    "write",
		Summary: "Create or fully overwrite a file in the workspace with the given content.",
		Usage:   "write <path>\\n<content>   (or write {\"path\": \"...\", \"content\": \"...\"})",
	}
}

func (t *WriteTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	path, content, err := parseWriteArgs(args)
	if err != nil {
		return Result{}, err
	}
	if path == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}
	if content == "" {
		return Result{}, fmt.Errorf("content is empty; refusing to write an empty file (use bash with truncation if intentional)")
	}

	absPath, relPath, err := patchtool.ResolveWorkspaceWritePath(path)
	if err != nil {
		return Result{}, err
	}

	existed := false
	if info, statErr := os.Stat(absPath); statErr == nil {
		if info.IsDir() {
			return Result{}, fmt.Errorf("path is a directory: %s", relPath)
		}
		existed = true
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o700); err != nil {
		return Result{}, fmt.Errorf("failed to create parent directories: %w", err)
	}

	if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
		return Result{}, fmt.Errorf("failed to write file: %w", err)
	}

	verb := "created"
	if existed {
		verb = "overwrote"
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Successfully %s %s (%d bytes)", verb, relPath, len(content)),
		Output:  fmt.Sprintf("%s file: %s\nabsolute: %s\nbytes: %d", verb, relPath, absPath, len(content)),
	}, nil
}

func parseWriteArgs(args string) (string, string, error) {
	if parsed := parseJSONArgs(args); parsed != nil {
		path := jsonStr(parsed, "path", "file", "file_path", "filePath")
		if path == "" {
			return "", "", fmt.Errorf("usage: write requires {\"path\": \"...\", \"content\": \"...\"}")
		}
		content := jsonRawStr(parsed, "content", "text", "body")
		if content == "" {
			return "", "", fmt.Errorf("usage: write requires non-empty \"content\" field")
		}
		return path, content, nil
	}

	trimmed := strings.TrimLeft(args, " \t\n\r")
	if strings.EqualFold(prefixToken(trimmed), "write") {
		trimmed = strings.TrimSpace(trimmed[len("write"):])
	}

	idx := strings.IndexFunc(trimmed, func(c rune) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
	})
	if idx == -1 {
		return "", "", fmt.Errorf("usage: %s", "write <path>\\n<content>")
	}
	path := strings.TrimSpace(trimmed[:idx])
	content := strings.TrimLeft(trimmed[idx+1:], " \t\n\r")
	if path == "" {
		return "", "", fmt.Errorf("usage: write <path>\\n<content>")
	}
	return path, content, nil
}

func prefixToken(s string) string {
	for i, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return s[:i]
		}
	}
	return s
}
