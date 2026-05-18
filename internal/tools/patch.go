package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	patchSearchMarker  = "<<<<<<< SEARCH"
	patchDividerMarker = "======="
	patchReplaceMarker = ">>>>>>> REPLACE"
)

type PatchTool struct{}

func NewPatchTool() *PatchTool {
	return &PatchTool{}
}

func (t *PatchTool) Spec() Spec {
	return Spec{
		Name:    "patch",
		Summary: "Aplica una sustitucion exacta y controlada sobre un archivo del workspace.",
		Usage:   "patch <ruta> followed by SEARCH/REPLACE blocks",
	}
}

func (t *PatchTool) Run(ctx context.Context, args string) (Result, error) {
	_ = ctx
	path, search, replace, err := parsePatchInput(args)
	if err != nil {
		return Result{}, err
	}

	absPath, relPath, err := resolveWorkspacePath(path)
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
		if search != "" {
			return Result{}, fmt.Errorf("el archivo %s no existe y el bloque SEARCH no esta vacio", relPath)
		}
		updated = replace
	} else {
		if search == "" {
			return Result{}, fmt.Errorf("SEARCH vacio solo se permite para crear archivos nuevos")
		}

		matches := strings.Count(current, search)
		if matches == 0 {
			return Result{}, fmt.Errorf("no se encontro el bloque SEARCH en %s", relPath)
		}
		if matches > 1 {
			return Result{}, fmt.Errorf("el bloque SEARCH aparece %d veces en %s; la sustitucion debe ser unica", matches, relPath)
		}

		updated = strings.Replace(current, search, replace, 1)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return Result{}, err
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Patch aplicado sobre %s.", relPath),
		Output:  diffPreview(search, replace),
	}, nil
}

func parsePatchInput(input string) (string, string, string, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", "", fmt.Errorf("uso: primera linea con la ruta seguida del bloque SEARCH/REPLACE")
	}

	newline := strings.Index(input, "\n")
	if newline == -1 {
		return "", "", "", fmt.Errorf("falta el bloque SEARCH/REPLACE")
	}

	path := strings.TrimSpace(input[:newline])
	body := strings.TrimSpace(input[newline+1:])

	searchIndex := strings.Index(body, patchSearchMarker)
	dividerIndex := strings.Index(body, patchDividerMarker)
	replaceIndex := strings.Index(body, patchReplaceMarker)
	if searchIndex != 0 || dividerIndex == -1 || replaceIndex == -1 || dividerIndex > replaceIndex {
		return "", "", "", fmt.Errorf("formato invalido; usa SEARCH/REPLACE con este esquema:\n%s\n<old>\n%s\n<new>\n%s", patchSearchMarker, patchDividerMarker, patchReplaceMarker)
	}

	search := strings.TrimPrefix(body[:dividerIndex], patchSearchMarker)
	search = strings.TrimPrefix(search, "\n")

	replace := body[dividerIndex+len(patchDividerMarker):replaceIndex]
	replace = strings.TrimPrefix(replace, "\n")

	trailer := strings.TrimSpace(body[replaceIndex+len(patchReplaceMarker):])
	if trailer != "" {
		return "", "", "", fmt.Errorf("contenido inesperado despues del bloque REPLACE")
	}

	return path, search, replace, nil
}

func diffPreview(search, replace string) string {
	return strings.Join([]string{
		patchSearchMarker,
		search,
		patchDividerMarker,
		replace,
		patchReplaceMarker,
	}, "\n")
}
