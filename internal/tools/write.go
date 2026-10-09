package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	patchtool "github.com/Hoosk/motoko/internal/tools/patch"
)

type WriteTool struct{}

func NewWriteTool() *WriteTool {
	return &WriteTool{}
}

func (t *WriteTool) Spec() Spec {
	return Spec{
		Name:        "write",
		Summary:     "Create or fully overwrite a file in the workspace with the given content.",
		Usage:       `write {"path": "internal/foo.go", "content": "package foo\n"}`,
		InputSchema: schemaWrite,
	}
}

func (t *WriteTool) Run(ctx context.Context, args string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
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
	previous := ""
	if info, statErr := os.Stat(absPath); statErr == nil {
		if info.IsDir() {
			return Result{}, fmt.Errorf("path is a directory: %s", relPath)
		}
		existed = true
		data, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return Result{}, fmt.Errorf("failed to read existing file: %w", readErr)
		}
		previous = string(data)
	}
	diff := patchtool.UnifiedDiff(relPath, previous, content)
	if err := requestFileChange(ctx, relPath, diff); err != nil {
		return Result{}, err
	}

	if err := patchtool.WriteWorkspaceFile(ctx, absPath, []byte(previous), []byte(content), existed, 0o700, 0o600); err != nil {
		return Result{}, fmt.Errorf("failed to write file: %w", err)
	}
	output := diff
	if strings.TrimSpace(output) == "" {
		output = fmt.Sprintf("%s file: %s\nabsolute: %s\nbytes: %d", verbForWrite(existed), relPath, absPath, len(content))
	} else {
		output += fmt.Sprintf("\n\n%s file: %s\nabsolute: %s\nbytes: %d", verbForWrite(existed), relPath, absPath, len(content))
	}

	verb := "created"
	if existed {
		verb = "overwrote"
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Successfully %s %s (%d bytes)", verb, relPath, len(content)),
		Output:  output,
	}, nil
}

func verbForWrite(existed bool) string {
	if existed {
		return "overwrote"
	}
	return "created"
}

func parseWriteArgs(args string) (string, string, error) {
	parsed := parseJSONArgs(args)
	if parsed == nil {
		return "", "", fmt.Errorf(`usage: write {"path": "...", "content": "..."}`)
	}
	path := jsonStr(parsed, "path", "file", "file_path", "filePath")
	if path == "" {
		return "", "", fmt.Errorf(`usage: write requires {"path": "...", "content": "..."}`)
	}
	content := jsonRawStr(parsed, "content", "text", "body")
	if content == "" {
		return "", "", fmt.Errorf(`usage: write requires non-empty "content" field`)
	}
	return path, content, nil
}
