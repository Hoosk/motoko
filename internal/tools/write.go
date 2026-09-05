package tools

import (
	"context"
	"fmt"
	"strings"

	patchtool "github.com/Hoosk/motoko/internal/tools/patch"
	"github.com/Hoosk/motoko/internal/tools/pathpolicy"
)

type WriteTool struct{}

func NewWriteTool() *WriteTool {
	return &WriteTool{}
}

func (t *WriteTool) Spec() Spec {
	return Spec{
		Name:        "write",
		Summary:     "Create or fully overwrite a file in the workspace with the given content.",
		Usage:       "write <path>\\n<content>   (or write {\"path\": \"...\", \"content\": \"...\"})",
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

	resolved, err := pathpolicy.Resolve(path)
	if err != nil {
		return Result{}, err
	}
	if err := pathpolicy.ValidateWrite(resolved); err != nil {
		return Result{}, err
	}
	if err := approveExternalAccess(ctx, "modify", resolved); err != nil {
		return Result{}, err
	}
	absPath, relPath := resolved.Path, resolved.Relative
	existed := resolved.Existing()
	var previous string
	if existed {
		data, readErr := pathpolicy.ReadFile(resolved)
		if readErr != nil {
			return Result{}, fmt.Errorf("failed to read existing file: %w", readErr)
		}
		previous = string(data)
	}
	diff := patchtool.UnifiedDiff(relPath, previous, content)
	if err := requestFileChange(ctx, relPath, diff); err != nil {
		return Result{}, err
	}

	if err := pathpolicy.WriteFile(ctx, resolved, []byte(previous), []byte(content), 0o600, 0o700); err != nil {
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
