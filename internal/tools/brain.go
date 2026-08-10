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
		Name:        "brain_write",
		Summary:     "Write or update a file in the session brain",
		Usage:       "brain_write <filename> <content>",
		InputSchema: schemaBrainWrite,
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
		Name:        "brain_read",
		Summary:     "Read a file from the session brain, optionally with pagination",
		Usage:       "brain_read <filename> [offset] [limit]",
		InputSchema: schemaBrainRead,
	}
}

func (t *BrainReadTool) Run(ctx context.Context, args string) (Result, error) {
	if parsed := parseJSONArgs(args); parsed != nil {
		return t.runJSON(ctx, parsed)
	}
	return t.runPlain(ctx, args)
}

// readBrain resolves the session brain from the tool context, falling back
// to the provider when the context carries no brain.
func (t *BrainReadTool) readBrain(ctx context.Context) (*brain.Brain, error) {
	br := GetBrain(ctx)
	if br == nil {
		br = t.provider.GetBrain()
	}
	if br == nil {
		return nil, fmt.Errorf("session brain not initialized")
	}
	return br, nil
}

func (t *BrainReadTool) runJSON(ctx context.Context, parsed map[string]any) (Result, error) {
	filename := jsonStr(parsed, "filename", "file", "name")
	if filename == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	br, err := t.readBrain(ctx)
	if err != nil {
		return Result{}, err
	}
	content, err := br.Read(filename)
	if err != nil {
		return Result{}, err
	}

	offset, limit, err := readRangeFrom(parsed)
	if err != nil {
		return Result{}, err
	}
	if offset == 1 && limit == 200 && !jsonHas(parsed, "offset", "line", "start") && !jsonHas(parsed, "limit", "lines", "count") {
		return Result{
			Spec:    t.Spec(),
			Summary: fmt.Sprintf("Successfully read brain file %s", filename),
			Output:  content,
		}, nil
	}
	return paginatedResult(t.Spec(), filename, content, offset, limit, "Successfully read brain file %s from line %d")
}

// readRangeFrom extracts and validates the offset/limit pair from JSON args.
func readRangeFrom(parsed map[string]any) (offset, limit int, err error) {
	offset, limit = 1, 200
	if value, ok := jsonInt(parsed, "offset", "line", "start"); ok {
		if value < 1 {
			return 0, 0, fmt.Errorf("invalid offset: %d", value)
		}
		offset = value
	}
	if value, ok := jsonInt(parsed, "limit", "lines", "count"); ok {
		if value < 1 {
			return 0, 0, fmt.Errorf("invalid limit: %d", value)
		}
		limit = value
	}
	return offset, limit, nil
}

func (t *BrainReadTool) runPlain(ctx context.Context, args string) (Result, error) {
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

	br, err := t.readBrain(ctx)
	if err != nil {
		return Result{}, err
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

	offset, limit, err := readRangeFromParts(parts)
	if err != nil {
		return Result{}, err
	}
	return paginatedResult(t.Spec(), filename, content, offset, limit, "Brain file %s read from line %d (%d lines)")
}

// readRangeFromParts extracts and validates the offset/limit pair from the
// positional form `filename [offset] [limit]`.
func readRangeFromParts(parts []string) (offset, limit int, err error) {
	offset, limit = 1, 200
	if len(parts) >= 2 {
		value, parseErr := strconv.Atoi(parts[1])
		if parseErr != nil || value < 1 {
			return 0, 0, fmt.Errorf("invalid offset: %s", parts[1])
		}
		offset = value
	}
	if len(parts) >= 3 {
		value, parseErr := strconv.Atoi(parts[2])
		if parseErr != nil || value < 1 {
			return 0, 0, fmt.Errorf("invalid limit: %s", parts[2])
		}
		limit = value
	}
	return offset, limit, nil
}

// paginatedResult renders the line-numbered slice of content. format is a
// printf template receiving the filename, offset and (where included) the
// line count.
func paginatedResult(spec Spec, filename, content string, offset, limit int, format string) (Result, error) {
	lines := strings.Split(content, "\n")
	var paginatedLines []string
	for i := offset - 1; i < len(lines) && len(paginatedLines) < limit; i++ {
		paginatedLines = append(paginatedLines, fmt.Sprintf("%d: %s", i+1, lines[i]))
	}

	if len(paginatedLines) == 0 {
		return Result{
			Spec:    spec,
			Summary: fmt.Sprintf("No visible content in %s from line %d.", filename, offset),
			Output:  "",
		}, nil
	}
	return Result{
		Spec:    spec,
		Summary: fmt.Sprintf(format, filename, offset, len(paginatedLines)),
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
		Name:        "brain_list",
		Summary:     "List all files in the session brain",
		Usage:       "brain_list",
		InputSchema: schemaBrainList,
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
