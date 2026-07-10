package tools

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

type GlobTool struct{}

func NewGlobTool() *GlobTool {
	return &GlobTool{}
}

func (t *GlobTool) Spec() Spec {
	return Spec{
		Name:    "glob",
		Summary: "Finds file paths by pattern in the workspace.",
		Usage:   "glob <pattern>",
	}
}

func (t *GlobTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	pattern := strings.TrimSpace(args)
	if parsed := parseJSONArgs(args); parsed != nil {
		pattern = jsonStr(parsed, "pattern", "glob")
	}
	if pattern == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	matcher, err := compileGlob(pattern)
	if err != nil {
		return Result{}, err
	}

	var matches []string
	err = walkWorkspace(ctx, func(relPath, absPath string, entry fs.DirEntry) error {
		_ = absPath
		if matcher.MatchString(relPath) {
			matches = append(matches, relPath)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	sort.Strings(matches)
	if len(matches) == 0 {
		return Result{Spec: t.Spec(), Summary: fmt.Sprintf("No matches for %s.", pattern), Output: ""}, nil
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("%d matches for %s.", len(matches), pattern),
		Output:  strings.Join(matches, "\n"),
	}, nil
}
