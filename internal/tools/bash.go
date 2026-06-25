package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const bashTimeout = 20 * time.Second

type BashTool struct{}

func NewBashTool() *BashTool {
	return &BashTool{}
}

func (t *BashTool) Spec() Spec {
	return Spec{
		Name:    "bash",
		Summary: "Ejecuta un comando shell en el workspace actual.",
		Usage:   "bash <comando>",
	}
}

func (t *BashTool) DynamicSpec(ctx ToolContext) Spec {
	spec := t.Spec()
	spec.Summary = fmt.Sprintf("Ejecuta un comando shell en el workspace actual (%s). El output maximo esta limitado a %d bytes.", ctx.Workspace, ctx.MaxOutputSize)
	return spec
}

func (t *BashTool) Run(ctx context.Context, args string) (Result, error) {
	command := strings.TrimSpace(args)
	if command == "" {
		return Result{}, fmt.Errorf("uso: %s", t.Spec().Usage)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	wd, err := os.Getwd()
	if err != nil {
		return Result{}, err
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()
	result := Result{
		Spec:   t.Spec(),
		Output: strings.TrimSpace(string(output)),
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.Summary = fmt.Sprintf("bash timeout para: %s", command)
		return result, nil
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.Summary = fmt.Sprintf("bash termino con salida %d.", exitErr.ExitCode())
			return result, nil
		}
		return Result{}, err
	}

	result.Summary = "bash ejecutado correctamente."
	return result, nil
}
