package patch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	approvalpkg "github.com/Hoosk/motoko/internal/tools/approval"
)

const (
	searchMarker  = "<<<<<<< SEARCH"
	astMarker     = "<<<<<<< AST"
	dividerMarker = "======="
	replaceMarker = ">>>>>>> REPLACE"
)

type Tool struct{}

type Result struct {
	Summary string
	Output  string
}

type request struct {
	Unified *unifiedPatch
	Path    string
	Search  string
	Replace string
	AST     []*astPatch
}

type astPatch struct {
	Path     string
	Action   string
	Replace  string
	Selector astSelector
}

type astSelector struct {
	Query    string
	Capture  string
	Type     string
	Name     string
	Contains string
	Index    int
}

type unifiedPatch struct {
	OldPath string
	NewPath string
	Hunks   []unifiedHunk
}

type unifiedHunk struct {
	Lines    []unifiedHunkLine
	OldStart int
	OldCount int
	NewStart int
	NewCount int
}

type unifiedHunkLine struct {
	Text      string
	Kind      byte
	NoNewline bool
}

type patchedLine struct {
	Text      string
	NoNewline bool
}

var unifiedHunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func New() *Tool {
	return &Tool{}
}

func (t *Tool) Run(ctx context.Context, args string) (Result, error) {
	request, err := parsePatchRequest(args)
	if err != nil {
		return Result{}, err
	}
	if len(request.AST) > 0 {
		return t.runASTPatch(ctx, request.AST)
	}
	if request.Unified != nil {
		return t.runUnifiedPatch(ctx, request.Unified)
	}

	absPath, relPath, err := resolveWorkspaceWritePath(request.Path)
	if err != nil {
		return Result{}, err
	}

	content, err := os.ReadFile(absPath)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}

	current := string(content)
	updated := ""

	if os.IsNotExist(err) {
		if request.Search != "" {
			return Result{}, fmt.Errorf("file %s does not exist and the SEARCH block is not empty", relPath)
		}
		updated = request.Replace
	} else {
		updated, err = fuzzyReplace(current, request.Search, request.Replace)
		if err != nil {
			return Result{}, err
		}
	}

	diff := UnifiedDiff(relPath, current, updated)
	if broker := approvalpkg.GetBroker(ctx); broker != nil {
		if err := broker.Request(ctx, approvalpkg.FileChange{Path: relPath, Diff: diff}); err != nil {
			return Result{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return Result{}, err
	}

	return Result{
		Summary: fmt.Sprintf("Patch applied to %s.", relPath),
		Output:  diff,
	}, nil
}

func (t *Tool) runASTPatch(ctx context.Context, requests []*astPatch) (Result, error) {
	if len(requests) == 0 {
		return Result{}, fmt.Errorf("no AST mutations provided")
	}
	absPath, relPath, err := resolveWorkspaceWritePath(requests[0].Path)
	if err != nil {
		return Result{}, err
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return Result{}, err
	}
	updated := string(content)
	for _, request := range requests {
		if request == nil {
			continue
		}
		if request.Path != requests[0].Path {
			return Result{}, fmt.Errorf("all AST mutations must target the same file in one request")
		}
		if request.Action == "" {
			request.Action = actionReplace
		}
		updated, err = applyASTPatch([]byte(updated), relPath, request)
		if err != nil {
			return Result{}, err
		}
	}
	diff := UnifiedDiff(relPath, string(content), updated)
	if broker := approvalpkg.GetBroker(ctx); broker != nil {
		if err := broker.Request(ctx, approvalpkg.FileChange{Path: relPath, Diff: diff}); err != nil {
			return Result{}, err
		}
	}
	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return Result{}, err
	}
	rendered := make([]string, 0, len(requests))
	for _, request := range requests {
		if request == nil {
			continue
		}
		rendered = append(rendered, request.Render())
	}
	summary := fmt.Sprintf("%d AST mutations applied to %s.", len(rendered), relPath)
	if len(rendered) == 1 {
		summary = fmt.Sprintf("AST patch applied to %s.", relPath)
	}
	return Result{Summary: summary, Output: diff}, nil
}

func (t *Tool) runUnifiedPatch(ctx context.Context, patch *unifiedPatch) (Result, error) {
	path, err := patch.targetPath()
	if err != nil {
		return Result{}, err
	}
	absPath, relPath, err := resolveWorkspaceWritePath(path)
	if err != nil {
		return Result{}, err
	}
	content, readErr := os.ReadFile(absPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return Result{}, readErr
	}
	if os.IsNotExist(readErr) && patch.OldPath != devNull {
		return Result{}, fmt.Errorf("file %s does not exist to apply the unified diff", relPath)
	}
	updated, err := applyUnifiedPatch(string(content), patch)
	if err != nil {
		return Result{}, err
	}
	diff := UnifiedDiff(relPath, string(content), updated)
	if broker := approvalpkg.GetBroker(ctx); broker != nil {
		if err := broker.Request(ctx, approvalpkg.FileChange{Path: relPath, Diff: diff}); err != nil {
			return Result{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return Result{}, err
	}
	return Result{
		Summary: fmt.Sprintf("Unified diff applied to %s.", relPath),
		Output:  diff,
	}, nil
}
