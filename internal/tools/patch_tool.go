package tools

import (
	"context"

	patchtool "github.com/Hoosk/motoko/internal/tools/patch"
	"github.com/Hoosk/motoko/internal/tools/pathpolicy"
)

type PatchTool struct {
	engine *patchtool.Tool
}

func NewPatchTool() *PatchTool {
	return &PatchTool{engine: patchtool.New(func(ctx context.Context, resolved pathpolicy.Resolution) error {
		return approveExternalAccess(ctx, "modify", resolved)
	})}
}

func (t *PatchTool) Spec() Spec {
	return Spec{
		Name:    "patch",
		Summary: "Applies changes to workspace files with multi-language AST patch, SEARCH/REPLACE, or unified diff.",
		Usage:   "patch <path> + AST/SEARCH/REPLACE or unified diff with ---/+++",
	}
}

func (t *PatchTool) Run(ctx context.Context, args string) (Result, error) {
	result, err := t.engine.Run(ctx, args)
	if err != nil {
		return Result{}, err
	}
	return Result{Spec: t.Spec(), Summary: result.Summary, Output: result.Output}, nil
}
