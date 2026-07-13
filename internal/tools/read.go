package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Hoosk/motoko/internal/tools/pathpolicy"
)

const defaultReadLimit = 200

type ReadTool struct {
	injectedInstructions map[string]bool
	mu                   sync.Mutex
}

func NewReadTool() *ReadTool {
	return &ReadTool{
		injectedInstructions: make(map[string]bool),
	}
}

func (t *ReadTool) Spec() Spec {
	return Spec{
		Name:    "read",
		Summary: "Reads a file or lists a directory in the workspace.",
		Usage:   "read <path> [offset] [limit]",
	}
}

func (t *ReadTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)
	offset := 1
	limit := defaultReadLimit

	if parsed := parseJSONArgs(args); parsed != nil {
		path := jsonStr(parsed, "path", "filePath", "file_path", "file")
		if path == "" {
			return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
		}
		if value, ok := jsonInt(parsed, "offset", "line", "start"); ok {
			if value < 1 {
				return Result{}, fmt.Errorf("invalid offset: %d", value)
			}
			offset = value
		}
		if value, ok := jsonInt(parsed, "limit", "lines", "max_lines", "count"); ok {
			if value < 1 {
				return Result{}, fmt.Errorf("invalid limit: %d", value)
			}
			limit = value
		}
		parts = []string{path}
	} else if len(parts) == 0 {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	} else {
		if len(parts) >= 2 {
			value, err := strconv.Atoi(parts[1])
			if err != nil || value < 1 {
				return Result{}, fmt.Errorf("invalid offset: %s", parts[1])
			}
			offset = value
		}
		if len(parts) >= 3 {
			value, err := strconv.Atoi(parts[2])
			if err != nil || value < 1 {
				return Result{}, fmt.Errorf("invalid limit: %s", parts[2])
			}
			limit = value
		}
	}

	resolved, err := pathpolicy.Resolve(parts[0])
	if err != nil {
		return Result{}, err
	}
	if err := approveExternalAccess(ctx, "read", resolved); err != nil {
		return Result{}, err
	}
	absPath, relPath := resolved.Path, resolved.Relative

	file, err := pathpolicy.OpenRead(resolved)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return Result{}, err
	}

	injected := t.getInjectedInstructions(absPath, resolved.External)

	if info.IsDir() {
		entries, readErr := file.ReadDir(-1)
		if readErr != nil {
			return Result{}, readErr
		}

		var lines []string
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			lines = append(lines, name)
		}

		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Directory %s with %d entries.", relPath, len(lines)),
			Output:  strings.Join(lines, "\n") + injected,
		}, nil
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	lineNo := 0
	var lines []string
	for scanner.Scan() {
		lineNo++
		if lineNo < offset {
			continue
		}
		if len(lines) >= limit {
			break
		}
		lines = append(lines, fmt.Sprintf("%d: %s", lineNo, scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return Result{}, err
	}

	if len(lines) == 0 {
		return Result{Spec: t.Spec(), Summary: fmt.Sprintf("No visible content in %s from line %d.", relPath, offset), Output: injected}, nil
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("File %s read from line %d (%d lines).", filepath.ToSlash(relPath), offset, len(lines)),
		Output:  strings.Join(lines, "\n") + injected,
	}, nil
}

func (t *ReadTool) getInjectedInstructions(absPath string, external bool) string {
	if external {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	workspaceRoot, _, err := resolveWorkspacePath("")
	if err != nil {
		return ""
	}

	var injected strings.Builder
	dir := absPath
	info, err := os.Stat(absPath)
	if err == nil && !info.IsDir() {
		dir = filepath.Dir(absPath)
	}

	for {
		for _, name := range []string{"AGENTS.md", ".agents.md"} {
			agentFile := filepath.Join(dir, name)
			if t.injectedInstructions[agentFile] {
				continue
			}
			resolved, err := pathpolicy.Resolve(agentFile)
			if err != nil || resolved.External {
				continue
			}
			file, err := pathpolicy.OpenRead(resolved)
			if err != nil {
				continue
			}
			b, readErr := io.ReadAll(file)
			_ = file.Close()
			if readErr == nil {
				t.injectedInstructions[agentFile] = true
				fmt.Fprintf(&injected, "\n\n<system-reminder>\nFound %s:\n%s\n</system-reminder>", name, string(b))
			}
		}
		if dir == workspaceRoot {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir { // root of filesystem reached
			break
		}
		dir = parent
	}
	return injected.String()
}
