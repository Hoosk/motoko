package patch

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Hoosk/motoko/internal/tools/pathpolicy"
)

const (
	searchMarker  = "<<<<<<< SEARCH"
	astMarker     = "<<<<<<< AST"
	dividerMarker = "======="
	replaceMarker = ">>>>>>> REPLACE"
)

type Tool struct {
	approveExternal ExternalApprover
}

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

func New(approvers ...ExternalApprover) *Tool {
	tool := &Tool{}
	if len(approvers) > 0 {
		tool.approveExternal = approvers[0]
	}
	return tool
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

	resolved, err := resolveWorkspaceWritePath(ctx, request.Path, t.approveExternal)
	if err != nil {
		return Result{}, err
	}

	var content []byte
	if resolved.Existing() {
		content, err = pathpolicy.ReadFile(resolved)
		if err != nil {
			return Result{}, err
		}
	}

	current := string(content)
	updated := ""

	if !resolved.Existing() {
		if request.Search != "" {
			return Result{}, fmt.Errorf("file %s does not exist and the SEARCH block is not empty", resolved.Relative)
		}
		updated = request.Replace
	} else {
		updated, err = fuzzyReplace(current, request.Search, request.Replace)
		if err != nil {
			return Result{}, err
		}
	}

	if err := pathpolicy.WriteFile(resolved, []byte(updated), 0o644, 0o755); err != nil {
		return Result{}, err
	}

	return Result{
		Summary: fmt.Sprintf("Patch applied to %s.", resolved.Relative),
		Output:  diffPreview(request.Search, request.Replace),
	}, nil
}

func (t *Tool) runASTPatch(ctx context.Context, requests []*astPatch) (Result, error) {
	if len(requests) == 0 {
		return Result{}, fmt.Errorf("no AST mutations provided")
	}
	resolved, err := resolveWorkspaceWritePath(ctx, requests[0].Path, t.approveExternal)
	if err != nil {
		return Result{}, err
	}
	content, err := pathpolicy.ReadFile(resolved)
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
		updated, err = applyASTPatch([]byte(updated), resolved.Relative, request)
		if err != nil {
			return Result{}, err
		}
	}
	if err := pathpolicy.WriteFile(resolved, []byte(updated), 0o644, 0o755); err != nil {
		return Result{}, err
	}
	rendered := make([]string, 0, len(requests))
	for _, request := range requests {
		if request == nil {
			continue
		}
		rendered = append(rendered, request.Render())
	}
	summary := fmt.Sprintf("%d AST mutations applied to %s.", len(rendered), resolved.Relative)
	if len(rendered) == 1 {
		summary = fmt.Sprintf("AST patch applied to %s.", resolved.Relative)
	}
	return Result{Summary: summary, Output: strings.Join(rendered, "\n\n")}, nil
}

func (t *Tool) runUnifiedPatch(ctx context.Context, patch *unifiedPatch) (Result, error) {
	path, err := patch.targetPath()
	if err != nil {
		return Result{}, err
	}
	resolved, err := resolveWorkspaceWritePath(ctx, path, t.approveExternal)
	if err != nil {
		return Result{}, err
	}
	var content []byte
	if resolved.Existing() {
		content, err = pathpolicy.ReadFile(resolved)
		if err != nil {
			return Result{}, err
		}
	}
	if !resolved.Existing() && patch.OldPath != devNull {
		return Result{}, fmt.Errorf("file %s does not exist to apply the unified diff", resolved.Relative)
	}
	updated, err := applyUnifiedPatch(string(content), patch)
	if err != nil {
		return Result{}, err
	}
	if err := pathpolicy.WriteFile(resolved, []byte(updated), 0o644, 0o755); err != nil {
		return Result{}, err
	}
	return Result{
		Summary: fmt.Sprintf("Unified diff applied to %s.", resolved.Relative),
		Output:  patch.Render(),
	}, nil
}
