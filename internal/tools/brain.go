package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Hoosk/motoko/internal/brain"
)

type BrainProvider interface {
	GetBrain() *brain.Brain
}

// BrainWriteTool writes or updates files in the brain.
type BrainWriteTool struct {
	provider BrainProvider
}

func NewBrainWriteTool(p BrainProvider) *BrainWriteTool {
	return &BrainWriteTool{provider: p}
}

func (t *BrainWriteTool) Spec() Spec {
	return Spec{
		Name:    "brain_write",
		Summary: "Write or update a file in the session brain",
		Usage:   "brain_write <filename> <content>",
	}
}

func (t *BrainWriteTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	if parsed := parseJSONArgs(args); parsed != nil {
		filename := jsonStr(parsed, "filename", "file", "name")
		content := jsonRawStr(parsed, "content", "text", "body")
		if filename == "" || content == "" {
			return Result{}, fmt.Errorf("usage: brain_write <filename> <content>")
		}

		br := GetBrain(ctx)
		if br == nil {
			br = t.provider.GetBrain()
		}
		if br == nil {
			return Result{}, fmt.Errorf("session brain not initialized")
		}

		err := br.Write(filename, content)
		if err != nil {
			return Result{}, err
		}

		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Successfully wrote to brain file %s", filename),
			Output:  fmt.Sprintf("Wrote %d bytes to %s", len(content), filename),
		}, nil
	}

	args = strings.TrimLeft(args, " \t\n\r")
	idx := strings.IndexFunc(args, func(c rune) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
	})
	if idx == -1 {
		return Result{}, fmt.Errorf("usage: brain_write <filename> <content>")
	}
	filename := strings.TrimSpace(args[:idx])
	content := args[idx+1:]
	if strings.EqualFold(filename, "brain_write") {
		content = strings.TrimLeft(content, " \t\n\r")
		idx2 := strings.IndexFunc(content, func(c rune) bool {
			return c == ' ' || c == '\t' || c == '\n' || c == '\r'
		})
		if idx2 == -1 {
			return Result{}, fmt.Errorf("usage: brain_write <filename> <content>")
		}
		filename = strings.TrimSpace(content[:idx2])
		content = content[idx2+1:]
	}
	if filename == "" || content == "" {
		return Result{}, fmt.Errorf("usage: brain_write <filename> <content>")
	}

	br := GetBrain(ctx)
	if br == nil {
		br = t.provider.GetBrain()
	}
	if br == nil {
		return Result{}, fmt.Errorf("session brain not initialized")
	}

	err := br.Write(filename, content)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Successfully wrote to brain file %s", filename),
		Output:  fmt.Sprintf("Wrote %d bytes to %s", len(content), filename),
	}, nil
}

// BrainReadTool reads files from the brain.
type BrainReadTool struct {
	provider BrainProvider
}

func NewBrainReadTool(p BrainProvider) *BrainReadTool {
	return &BrainReadTool{provider: p}
}

func (t *BrainReadTool) Spec() Spec {
	return Spec{
		Name:    "brain_read",
		Summary: "Read a file from the session brain, optionally with pagination",
		Usage:   "brain_read <filename> [offset] [limit]",
	}
}

func (t *BrainReadTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	if parsed := parseJSONArgs(args); parsed != nil {
		filename := jsonStr(parsed, "filename", "file", "name")
		if filename == "" {
			return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
		}

		br := GetBrain(ctx)
		if br == nil {
			br = t.provider.GetBrain()
		}
		if br == nil {
			return Result{}, fmt.Errorf("session brain not initialized")
		}

		content, err := br.Read(filename)
		if err != nil {
			return Result{}, err
		}

		offset := 1
		limit := 200
		if value, ok := jsonInt(parsed, "offset", "line", "start"); ok {
			if value < 1 {
				return Result{}, fmt.Errorf("invalid offset: %d", value)
			}
			offset = value
		}
		if value, ok := jsonInt(parsed, "limit", "lines", "count"); ok {
			if value < 1 {
				return Result{}, fmt.Errorf("invalid limit: %d", value)
			}
			limit = value
		}

		if offset == 1 && limit == 200 && !jsonHas(parsed, "offset", "line", "start") && !jsonHas(parsed, "limit", "lines", "count") {
			return Result{
				Spec:    t.Spec(),
				Summary: fmt.Sprintf("Successfully read brain file %s", filename),
				Output:  content,
			}, nil
		}

		lines := strings.Split(content, "\n")
		var paginatedLines []string
		for i := offset - 1; i < len(lines) && len(paginatedLines) < limit; i++ {
			paginatedLines = append(paginatedLines, fmt.Sprintf("%d: %s", i+1, lines[i]))
		}

		if len(paginatedLines) == 0 {
			return Result{
				Spec:    t.Spec(),
				Summary: fmt.Sprintf("No visible content in %s from line %d.", filename, offset),
				Output:  "",
			}, nil
		}

		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Successfully read brain file %s from line %d", filename, offset),
			Output:  strings.Join(paginatedLines, "\n"),
		}, nil
	}

	parts := strings.Fields(args)
	if len(parts) == 0 {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}
	filename := parts[0]
	if strings.EqualFold(filename, "brain_read") {
		if len(parts) == 1 {
			return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
		}
		parts = parts[1:]
		filename = parts[0]
	}

	br := GetBrain(ctx)
	if br == nil {
		br = t.provider.GetBrain()
	}
	if br == nil {
		return Result{}, fmt.Errorf("session brain not initialized")
	}

	content, err := br.Read(filename)
	if err != nil {
		return Result{}, err
	}

	if len(parts) == 1 {
		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Successfully read brain file %s", filename),
			Output:  content,
		}, nil
	}

	offset := 1
	limit := 200
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

	lines := strings.Split(content, "\n")
	var paginatedLines []string
	for i := offset - 1; i < len(lines) && len(paginatedLines) < limit; i++ {
		paginatedLines = append(paginatedLines, fmt.Sprintf("%d: %s", i+1, lines[i]))
	}

	if len(paginatedLines) == 0 {
		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("No visible content in %s from line %d.", filename, offset),
			Output:  "",
		}, nil
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Brain file %s read from line %d (%d lines).", filename, offset, len(paginatedLines)),
		Output:  strings.Join(paginatedLines, "\n"),
	}, nil
}

// BrainListTool lists files in the brain.
type BrainListTool struct {
	provider BrainProvider
}

func NewBrainListTool(p BrainProvider) *BrainListTool {
	return &BrainListTool{provider: p}
}

func (t *BrainListTool) Spec() Spec {
	return Spec{
		Name:    "brain_list",
		Summary: "List all files in the session brain",
		Usage:   "brain_list",
	}
}

func (t *BrainListTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	br := GetBrain(ctx)
	if br == nil {
		br = t.provider.GetBrain()
	}
	if br == nil {
		return Result{}, fmt.Errorf("session brain not initialized")
	}

	files, err := br.List()
	if err != nil {
		return Result{}, err
	}

	if len(files) == 0 {
		return Result{
			Spec:    t.Spec(),
			Summary: "No brain files in the current session.",
			Output:  "No brain files found.",
		}, nil
	}

	var lines []string
	for _, f := range files {
		lines = append(lines, fmt.Sprintf("- %s (%d bytes)", f.Name, f.SizeBytes))
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Found %d brain files in session.", len(files)),
		Output:  strings.Join(lines, "\n"),
	}, nil
}
