package tools

import (
	"context"
	"fmt"
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
	args = strings.TrimLeft(args, " \t\n\r")
	idx := strings.IndexFunc(args, func(c rune) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
	})
	if idx == -1 {
		return Result{}, fmt.Errorf("usage: brain_write <filename> <content>")
	}
	filename := strings.TrimSpace(args[:idx])
	content := args[idx+1:]
	if filename == "" || content == "" {
		return Result{}, fmt.Errorf("usage: brain_write <filename> <content>")
	}

	br := t.provider.GetBrain()
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
		Summary: "Read a file from the session brain",
		Usage:   "brain_read <filename>",
	}
}

func (t *BrainReadTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	filename := strings.TrimSpace(args)
	if filename == "" {
		return Result{}, fmt.Errorf("usage: brain_read <filename>")
	}

	br := t.provider.GetBrain()
	if br == nil {
		return Result{}, fmt.Errorf("session brain not initialized")
	}

	content, err := br.Read(filename)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Successfully read brain file %s", filename),
		Output:  content,
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
	br := t.provider.GetBrain()
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
