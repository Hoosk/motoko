package patch

import (
	"context"
	"fmt"
	"regexp"

	dialogpkg "github.com/Hoosk/motoko/internal/tools/dialog"
	"github.com/Hoosk/motoko/internal/tools/pathpolicy"
)

const (
	searchMarker  = "<<<<<<< SEARCH"
	astMarker     = "<<<<<<< AST"
	dividerMarker = "======="
	replaceMarker = ">>>>>>> REPLACE"
)

type Tool struct{ approveExternal ExternalApprover }

type Result struct {
	Summary string
	Output  string
}

type request struct {
	Unified *unifiedPatch
	Path    string
	Search  string
	Replace string
	Edits   []jsonPatchEdit
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
	if len(request.Edits) > 0 {
		return t.runJSONPatch(ctx, request.Path, request.Edits)
	}

	resolved, err := resolveWorkspaceWritePath(ctx, request.Path, t.approveExternal)
	if err != nil {
		return Result{}, err
	}

	relPath := resolved.Relative
	content, err := readPatchContent(resolved)
	if err != nil {
		return Result{}, err
	}
	existed := resolved.Existing()
	current := string(content)
	updated := ""

	if !existed {
		if request.Search != "" {
			return Result{}, fmt.Errorf("file %s does not exist and the SEARCH block is not empty", relPath)
		}
		if request.Replace == "" {
			return Result{}, fmt.Errorf("refusing to create an empty file with patch: %s", relPath)
		}
		updated = request.Replace
	} else {
		updated, err = fuzzyReplace(current, request.Search, request.Replace)
		if err != nil {
			return Result{}, err
		}
	}

	diff := UnifiedDiff(relPath, current, updated)
	if err := dialogpkg.GetBroker(ctx).RequestFileChange(ctx, dialogpkg.FileChange{Path: relPath, Diff: diff}); err != nil {
		return Result{}, err
	}
	if err := pathpolicy.WriteFile(ctx, resolved, content, []byte(updated), 0o644, 0o755); err != nil {
		return Result{}, err
	}

	return Result{
		Summary: fmt.Sprintf("Patch applied to %s.", relPath),
		Output:  diff,
	}, nil
}

func (t *Tool) runJSONPatch(ctx context.Context, path string, edits []jsonPatchEdit) (Result, error) {
	resolved, err := resolveWorkspaceWritePath(ctx, path, t.approveExternal)
	if err != nil {
		return Result{}, err
	}

	relPath := resolved.Relative
	content, readErr := readPatchContent(resolved)
	if readErr != nil {
		return Result{}, readErr
	}
	existed := resolved.Existing()
	updated := string(content)
	if !existed {
		if len(edits) != 1 {
			return Result{}, fmt.Errorf("new files require exactly one JSON edit")
		}
		old, replacement := jsonPatchValues(edits[0])
		if old != "" {
			return Result{}, fmt.Errorf("new file JSON edit must have an empty old value")
		}
		if replacement == "" {
			return Result{}, fmt.Errorf("refusing to create an empty file with patch: %s", relPath)
		}
		updated = replacement
	} else {
		for _, edit := range edits {
			old, replacement := jsonPatchValues(edit)
			if old == "" {
				return Result{}, fmt.Errorf("JSON edit requires a non-empty old value")
			}
			updated, err = fuzzyReplace(updated, old, replacement)
			if err != nil {
				return Result{}, err
			}
		}
	}

	diff := UnifiedDiff(relPath, string(content), updated)
	if err := dialogpkg.GetBroker(ctx).RequestFileChange(ctx, dialogpkg.FileChange{Path: relPath, Diff: diff}); err != nil {
		return Result{}, err
	}
	if err := pathpolicy.WriteFile(ctx, resolved, content, []byte(updated), 0o644, 0o755); err != nil {
		return Result{}, err
	}
	return Result{
		Summary: fmt.Sprintf("Patch applied to %s.", relPath),
		Output:  diff,
	}, nil
}

func jsonPatchValues(edit jsonPatchEdit) (old, replacement string) {
	old = edit.Old
	if old == "" {
		old = edit.OldString
	}
	replacement = edit.New
	if replacement == "" {
		replacement = edit.NewString
	}
	return old, replacement
}

func (t *Tool) runASTPatch(ctx context.Context, requests []*astPatch) (Result, error) {
	if len(requests) == 0 {
		return Result{}, fmt.Errorf("no AST mutations provided")
	}
	resolved, err := resolveWorkspaceWritePath(ctx, requests[0].Path, t.approveExternal)
	if err != nil {
		return Result{}, err
	}
	relPath := resolved.Relative
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
		updated, err = applyASTPatch([]byte(updated), relPath, request)
		if err != nil {
			return Result{}, err
		}
	}
	diff := UnifiedDiff(relPath, string(content), updated)
	if err := dialogpkg.GetBroker(ctx).RequestFileChange(ctx, dialogpkg.FileChange{Path: relPath, Diff: diff}); err != nil {
		return Result{}, err
	}
	if err := pathpolicy.WriteFile(ctx, resolved, []byte(content), []byte(updated), 0o644, 0o755); err != nil {
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
	resolved, err := resolveWorkspaceWritePath(ctx, path, t.approveExternal)
	if err != nil {
		return Result{}, err
	}
	relPath := resolved.Relative
	content, readErr := readPatchContent(resolved)
	if readErr != nil {
		return Result{}, readErr
	}
	if !resolved.Existing() && patch.OldPath != devNull {
		return Result{}, fmt.Errorf("file %s does not exist to apply the unified diff", relPath)
	}
	updated, err := applyUnifiedPatch(string(content), patch)
	if err != nil {
		return Result{}, err
	}
	existed := resolved.Existing()
	if !existed && updated == "" {
		return Result{}, fmt.Errorf("refusing to create an empty file with unified patch: %s", relPath)
	}
	diff := UnifiedDiff(relPath, string(content), updated)
	if err := dialogpkg.GetBroker(ctx).RequestFileChange(ctx, dialogpkg.FileChange{Path: relPath, Diff: diff}); err != nil {
		return Result{}, err
	}
	if err := pathpolicy.WriteFile(ctx, resolved, []byte(content), []byte(updated), 0o644, 0o755); err != nil {
		return Result{}, err
	}
	return Result{
		Summary: fmt.Sprintf("Unified diff applied to %s.", relPath),
		Output:  diff,
	}, nil
}

func readPatchContent(resolved pathpolicy.Resolution) ([]byte, error) {
	if !resolved.Existing() {
		return nil, nil
	}
	return pathpolicy.ReadFile(resolved)
}
