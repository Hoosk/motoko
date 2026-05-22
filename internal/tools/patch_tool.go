package tools

import (
	"context"

	patchtool "github.com/Hoosk/motoko/internal/tools/patch"
)

type PatchTool struct {
	engine *patchtool.Tool
}

func NewPatchTool() *PatchTool {
	return &PatchTool{engine: patchtool.New()}
}

func (t *PatchTool) Spec() Spec {
	return Spec{
		Name:    "patch",
		Summary: "Aplica cambios sobre archivos del workspace con AST patch multi-lenguaje, SEARCH/REPLACE o unified diff.",
		Usage:   "patch <ruta> + AST/SEARCH/REPLACE o unified diff con ---/+++",
	}
}

func (t *PatchTool) Run(ctx context.Context, args string) (Result, error) {
	result, err := t.engine.Run(ctx, args)
	if err != nil {
		return Result{}, err
	}
	return Result{Spec: t.Spec(), Summary: result.Summary, Output: result.Output}, nil
}
