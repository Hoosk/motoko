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
		Summary: "Aplica una sustitucion robusta sobre un archivo del workspace. Tolera errores de indentacion.",
		Usage:   "patch <ruta> followed by SEARCH/REPLACE blocks",
	}
}

func fuzzyReplace(current, search, replace string) (string, error) {
	if search == "" {
		return "", fmt.Errorf("SEARCH vacio solo se permite para crear archivos nuevos")
	}
	if err := validateFuzzySearchBlock(search); err != nil {
		return "", err
	}

	matches := strings.Count(current, search)
	if matches == 1 {
		return strings.Replace(current, search, replace, 1), nil
	}

	if matches > 1 {
		return "", fmt.Errorf("el bloque SEARCH aparece %d veces de forma exacta; la sustitucion debe ser unica. Proporciona mas contexto.", matches)
	}

	// Intento de reemplazo tolerante a espacios/indentación
	type lineInfo struct {
		start int
		end   int
		text  string
	}
	var lines []lineInfo
	for i := 0; i < len(current); {
		nl := strings.IndexByte(current[i:], '\n')
		var end int
		if nl == -1 {
			end = len(current)
		} else {
			end = i + nl + 1
		}
		lines = append(lines, lineInfo{start: i, end: end, text: current[i:end]})
		i = end
	}

	searchLines := strings.Split(strings.TrimSpace(search), "\n")
	if len(searchLines) == 0 {
		return "", fmt.Errorf("no se encontro el bloque SEARCH (vacio tras limpiar espacios)")
	}

	var matchIndices []int
	for i := 0; i <= len(lines)-len(searchLines); i++ {
		match := true
		for j := 0; j < len(searchLines); j++ {
			if strings.TrimSpace(lines[i+j].text) != strings.TrimSpace(searchLines[j]) {
				match = false
				break
			}
		}
		if match {
			matchIndices = append(matchIndices, i)
		}
	}

	if len(matchIndices) == 0 {
		return "", fmt.Errorf("no se encontro el bloque SEARCH ni siquiera ignorando espacios e indentacion. Revisa el contenido actual del archivo.")
	}
	if len(matchIndices) > 1 {
		return "", fmt.Errorf("el bloque SEARCH coincide en %d lugares ignorando espacios; debe ser unico. Proporciona mas lineas de contexto.", len(matchIndices))
	}

	startLine := matchIndices[0]
	endLine := startLine + len(searchLines) - 1

	originalStart := lines[startLine].start
	originalEnd := lines[endLine].end

	updated := current[:originalStart] + replace
	if len(replace) > 0 && replace[len(replace)-1] != '\n' && originalEnd < len(current) && current[originalEnd-1] == '\n' {
		updated += "\n"
	}
	updated += current[originalEnd:]
	
	return updated, nil
}

func validateFuzzySearchBlock(search string) error {
	trimmed := strings.TrimSpace(search)
	if trimmed == "" {
		return fmt.Errorf("el bloque SEARCH no puede quedar vacio tras limpiar espacios")
	}
	lines := strings.Split(trimmed, "\n")
	meaningfulLines := 0
	nonWhitespace := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		meaningfulLines++
		for _, r := range line {
			if !strings.ContainsRune("{}[]()", r) {
				nonWhitespace++
			}
		}
	}
	if meaningfulLines < 2 || nonWhitespace < 3 {
		return fmt.Errorf("el bloque SEARCH es demasiado ambiguo para fuzzy replace; proporciona mas lineas de contexto unicas")
	}
	return nil
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
		updated, err = fuzzyReplace(current, search, replace)
		if err != nil {
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
