package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
)

const (
	patchSearchMarker  = "<<<<<<< SEARCH"
	patchASTMarker     = "<<<<<<< AST"
	patchDividerMarker = "======="
	patchReplaceMarker = ">>>>>>> REPLACE"
)

type PatchTool struct{}

type patchRequest struct {
	Path    string
	Search  string
	Replace string
	AST     []*astPatch
	Unified *unifiedPatch
}

type astPatch struct {
	Path     string
	Selector astSelector
	Action   string
	Replace  string
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
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []unifiedHunkLine
}

type unifiedHunkLine struct {
	Kind      byte
	Text      string
	NoNewline bool
}

type patchedLine struct {
	Text      string
	NoNewline bool
}

var unifiedHunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func NewPatchTool() *PatchTool {
	return &PatchTool{}
}

func (t *PatchTool) Spec() Spec {
	return Spec{
		Name:    "patch",
		Summary: "Aplica cambios sobre archivos del workspace con AST patch multi-lenguaje, SEARCH/REPLACE o unified diff.",
		Usage:   "patch <ruta> + AST/SEARCH/REPLACE o unified diff con ---/+++",
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
	request, err := parsePatchRequest(args)
	if err != nil {
		return Result{}, err
	}
	if len(request.AST) > 0 {
		return t.runASTPatch(request.AST)
	}
	if request.Unified != nil {
		return t.runUnifiedPatch(request.Unified)
	}

	absPath, relPath, err := resolveWorkspacePath(request.Path)
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
			return Result{}, fmt.Errorf("el archivo %s no existe y el bloque SEARCH no esta vacio", relPath)
		}
		updated = request.Replace
	} else {
		updated, err = fuzzyReplace(current, request.Search, request.Replace)
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
		Output:  diffPreview(request.Search, request.Replace),
	}, nil
}

func (t *PatchTool) runASTPatch(requests []*astPatch) (Result, error) {
	if len(requests) == 0 {
		return Result{}, fmt.Errorf("no se proporcionaron mutaciones AST")
	}
	absPath, relPath, err := resolveWorkspacePath(requests[0].Path)
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
			return Result{}, fmt.Errorf("todas las mutaciones AST deben apuntar al mismo archivo en una request")
		}
		if request.Action == "" {
			request.Action = "replace"
		}
		updated, err = applyASTPatch([]byte(updated), relPath, request)
		if err != nil {
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
	summary := fmt.Sprintf("%d mutaciones AST aplicadas sobre %s.", len(rendered), relPath)
	if len(rendered) == 1 {
		summary = fmt.Sprintf("AST patch aplicado sobre %s.", relPath)
	}
	return Result{
		Spec:    t.Spec(),
		Summary: summary,
		Output:  strings.Join(rendered, "\n\n"),
	}, nil
}

func (t *PatchTool) runUnifiedPatch(patch *unifiedPatch) (Result, error) {
	path, err := patch.targetPath()
	if err != nil {
		return Result{}, err
	}
	absPath, relPath, err := resolveWorkspacePath(path)
	if err != nil {
		return Result{}, err
	}
	content, readErr := os.ReadFile(absPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return Result{}, readErr
	}
	if os.IsNotExist(readErr) && patch.OldPath != "/dev/null" {
		return Result{}, fmt.Errorf("el archivo %s no existe para aplicar el unified diff", relPath)
	}
	updated, err := applyUnifiedPatch(string(content), patch)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return Result{}, err
	}
	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Unified diff aplicado sobre %s.", relPath),
		Output:  patch.Render(),
	}, nil
}

func parsePatchRequest(input string) (patchRequest, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return patchRequest{}, fmt.Errorf("uso: primera linea con la ruta seguida del bloque SEARCH/REPLACE o un unified diff")
	}
	if strings.HasPrefix(trimmed, "--- ") || strings.HasPrefix(trimmed, "diff --git ") {
		patch, err := parseUnifiedPatch(trimmed)
		if err != nil {
			return patchRequest{}, err
		}
		return patchRequest{Unified: patch}, nil
	}
	path, body, err := splitPatchPathAndBody(trimmed)
	if err != nil {
		return patchRequest{}, err
	}
	if strings.HasPrefix(body, patchASTMarker) {
		astPatches, err := parseASTPatchInput(path, body)
		if err != nil {
			return patchRequest{}, err
		}
		return patchRequest{Path: path, AST: astPatches}, nil
	}
	path, search, replace, err := parsePatchInput(trimmed)
	if err != nil {
		return patchRequest{}, err
	}
	return patchRequest{Path: path, Search: search, Replace: replace}, nil
}

func splitPatchPathAndBody(input string) (string, string, error) {
	newline := strings.Index(input, "\n")
	if newline == -1 {
		return "", "", fmt.Errorf("falta el bloque de patch")
	}
	path := strings.TrimSpace(input[:newline])
	body := strings.TrimSpace(input[newline+1:])
	if path == "" || body == "" {
		return "", "", fmt.Errorf("formato invalido; falta ruta o bloque de patch")
	}
	return path, body, nil
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

func parseASTPatchInput(path, body string) ([]*astPatch, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var patches []*astPatch
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}
		if lines[i] != patchASTMarker {
			return nil, fmt.Errorf("formato AST invalido; se esperaba %s y se encontro: %s", patchASTMarker, lines[i])
		}
		dividerIndex := -1
		replaceIndex := -1
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == patchDividerMarker {
				dividerIndex = j
				break
			}
		}
		if dividerIndex == -1 {
			return nil, fmt.Errorf("formato AST invalido; falta marcador %s", patchDividerMarker)
		}
		for j := dividerIndex + 1; j < len(lines); j++ {
			if lines[j] == patchReplaceMarker {
				replaceIndex = j
				break
			}
		}
		if replaceIndex == -1 {
			return nil, fmt.Errorf("formato AST invalido; falta marcador %s", patchReplaceMarker)
		}
		selectorBlock := strings.Join(lines[i+1:dividerIndex], "\n")
		replace := strings.Join(lines[dividerIndex+1:replaceIndex], "\n")
		selector, err := parseASTSelector(selectorBlock)
		if err != nil {
			return nil, err
		}
		action := astActionFromSelectorBlock(selectorBlock)
		patches = append(patches, &astPatch{Path: path, Selector: selector, Action: action, Replace: replace})
		i = replaceIndex + 1
	}
	if len(patches) == 0 {
		return nil, fmt.Errorf("formato AST invalido; no contiene mutaciones")
	}
	return patches, nil
}

func parseASTSelector(block string) (astSelector, error) {
	selector := astSelector{Index: 1}
	lines := strings.Split(strings.TrimSpace(block), "\n")
	queryMode := false
	var queryLines []string
	for _, rawLine := range lines {
		if queryMode {
			queryLines = append(queryLines, rawLine)
			continue
		}
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			if queryMode {
				queryLines = append(queryLines, rawLine)
				continue
			}
			return astSelector{}, fmt.Errorf("linea AST invalida: %s", rawLine)
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch key {
		case "query":
			inlineQuery := strings.TrimSpace(value)
			if inlineQuery != "" {
				selector.Query = inlineQuery
				continue
			}
			queryMode = true
		case "capture":
			selector.Capture = value
		case "action":
			if normalizeASTAction(value) == "" {
				return astSelector{}, fmt.Errorf("accion AST no soportada: %s", value)
			}
		case "type":
			selector.Type = value
		case "name":
			selector.Name = value
		case "contains":
			selector.Contains = value
		case "index":
			index, err := strconv.Atoi(value)
			if err != nil || index < 1 {
				return astSelector{}, fmt.Errorf("index AST invalido: %s", value)
			}
			selector.Index = index
		default:
			return astSelector{}, fmt.Errorf("clave AST no soportada: %s", key)
		}
	}
	if queryMode {
		selector.Query = strings.TrimSpace(strings.Join(queryLines, "\n"))
	}
	if selector.Query != "" {
		if selector.Capture == "" {
			selector.Capture = "target"
		}
		return selector, nil
	}
	if strings.TrimSpace(selector.Type) == "" {
		return astSelector{}, fmt.Errorf("el bloque AST requiere 'query:' o 'type: <node_type>'")
	}
	return selector, nil
}

func astActionFromSelectorBlock(block string) string {
	for _, rawLine := range strings.Split(strings.TrimSpace(block), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), "action") {
			return normalizeASTAction(strings.TrimSpace(parts[1]))
		}
	}
	return "replace"
}

func normalizeASTAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "replace":
		return "replace"
	case "delete":
		return "delete"
	case "insert_before":
		return "insert_before"
	case "insert_after":
		return "insert_after"
	default:
		return ""
	}
}

func parseUnifiedPatch(input string) (*unifiedPatch, error) {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "--- ") {
			start = i
			break
		}
	}
	if start == -1 || start+1 >= len(lines) || !strings.HasPrefix(lines[start+1], "+++ ") {
		return nil, fmt.Errorf("unified diff invalido; faltan cabeceras ---/+++")
	}
	patch := &unifiedPatch{
		OldPath: normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[start], "--- "))),
		NewPath: normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[start+1], "+++ "))),
	}
	for i := start + 2; i < len(lines); {
		line := lines[i]
		if line == "" {
			i++
			continue
		}
		if !strings.HasPrefix(line, "@@ ") {
			return nil, fmt.Errorf("unified diff invalido; hunk esperado y se encontro: %s", line)
		}
		hunk, next, err := parseUnifiedHunk(lines, i)
		if err != nil {
			return nil, err
		}
		patch.Hunks = append(patch.Hunks, hunk)
		i = next
	}
	if len(patch.Hunks) == 0 {
		return nil, fmt.Errorf("unified diff invalido; no contiene hunks")
	}
	return patch, nil
}

func parseUnifiedHunk(lines []string, start int) (unifiedHunk, int, error) {
	match := unifiedHunkHeaderPattern.FindStringSubmatch(lines[start])
	if match == nil {
		return unifiedHunk{}, 0, fmt.Errorf("cabecera de hunk invalida: %s", lines[start])
	}
	hunk := unifiedHunk{
		OldStart: mustParsePatchNumber(match[1]),
		OldCount: parsePatchCount(match[2]),
		NewStart: mustParsePatchNumber(match[3]),
		NewCount: parsePatchCount(match[4]),
	}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "@@ ") {
			return hunk, i, nil
		}
		if strings.HasPrefix(line, "\\ No newline at end of file") {
			if len(hunk.Lines) == 0 {
				return unifiedHunk{}, 0, fmt.Errorf("marcador de newline invalido en hunk")
			}
			hunk.Lines[len(hunk.Lines)-1].NoNewline = true
			continue
		}
		if line == "" {
			hunk.Lines = append(hunk.Lines, unifiedHunkLine{Kind: ' ', Text: ""})
			continue
		}
		kind := line[0]
		if kind != ' ' && kind != '+' && kind != '-' {
			return unifiedHunk{}, 0, fmt.Errorf("linea de hunk invalida: %s", line)
		}
		hunk.Lines = append(hunk.Lines, unifiedHunkLine{Kind: kind, Text: line[1:]})
	}
	return hunk, len(lines), nil
}

func applyUnifiedPatch(current string, patch *unifiedPatch) (string, error) {
	lines := splitPatchedLines(current)
	result := make([]patchedLine, 0, len(lines))
	pos := 0
	for _, hunk := range patch.Hunks {
		target := hunk.targetIndex()
		if target < pos || target > len(lines) {
			return "", fmt.Errorf("no se puede aplicar el hunk en la linea %d", hunk.OldStart)
		}
		result = append(result, lines[pos:target]...)
		pos = target
		for _, line := range hunk.Lines {
			switch line.Kind {
			case ' ':
				if pos >= len(lines) || lines[pos].Text != line.Text {
					return "", fmt.Errorf("el contexto del hunk no coincide en la linea %d", pos+1)
				}
				result = append(result, lines[pos])
				pos++
			case '-':
				if pos >= len(lines) || lines[pos].Text != line.Text {
					return "", fmt.Errorf("la eliminacion del hunk no coincide en la linea %d", pos+1)
				}
				pos++
			case '+':
				result = append(result, patchedLine{Text: line.Text, NoNewline: line.NoNewline})
			}
		}
	}
	result = append(result, lines[pos:]...)
	return joinPatchedLines(result), nil
}

func (p *unifiedPatch) targetPath() (string, error) {
	oldPath := normalizeDiffPath(p.OldPath)
	newPath := normalizeDiffPath(p.NewPath)
	if newPath == "/dev/null" {
		return "", fmt.Errorf("el unified diff no soporta borrado de archivos")
	}
	if oldPath != "" && oldPath != "/dev/null" && newPath != "" && oldPath != newPath {
		return "", fmt.Errorf("el unified diff no soporta renombrados; old=%s new=%s", oldPath, newPath)
	}
	if newPath != "" {
		return newPath, nil
	}
	if oldPath != "" && oldPath != "/dev/null" {
		return oldPath, nil
	}
	return "", fmt.Errorf("no se pudo resolver la ruta objetivo del unified diff")
}

func (p *unifiedPatch) Render() string {
	var lines []string
	lines = append(lines, "--- "+p.OldPath, "+++ "+p.NewPath)
	for _, hunk := range p.Hunks {
		lines = append(lines, hunk.header())
		for _, line := range hunk.Lines {
			lines = append(lines, string(line.Kind)+line.Text)
			if line.NoNewline {
				lines = append(lines, `\ No newline at end of file`)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (h unifiedHunk) targetIndex() int {
	if h.OldStart <= 0 {
		return 0
	}
	return h.OldStart - 1
}

func (h unifiedHunk) header() string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
}

func splitPatchedLines(content string) []patchedLine {
	if content == "" {
		return nil
	}
	trailingNewline := strings.HasSuffix(content, "\n")
	raw := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	lines := make([]patchedLine, 0, len(raw))
	for i, line := range raw {
		patched := patchedLine{Text: line}
		if i == len(raw)-1 && !trailingNewline {
			patched.NoNewline = true
		}
		lines = append(lines, patched)
	}
	return lines
}

func joinPatchedLines(lines []patchedLine) string {
	if len(lines) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, line := range lines {
		builder.WriteString(line.Text)
		if i < len(lines)-1 || !line.NoNewline {
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func normalizeDiffPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return filepath.ToSlash(path)
}

func mustParsePatchNumber(raw string) int {
	value, _ := strconv.Atoi(raw)
	return value
}

func parsePatchCount(raw string) int {
	if raw == "" {
		return 1
	}
	return mustParsePatchNumber(raw)
}

func applyASTPatch(content []byte, relPath string, request *astPatch) (string, error) {
	lang, languageName := treeSitterLanguageForPath(relPath)
	if lang == nil {
		return "", fmt.Errorf("AST patch no soportado para %s", filepath.Ext(relPath))
	}
	root, _ := sitter.ParseCtx(context.Background(), content, lang)
	if root == nil {
		return "", fmt.Errorf("no se pudo parsear el AST de %s (%s)", relPath, languageName)
	}
	matches, err := findASTMatches(root, content, lang, request.Selector)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		if request.Selector.Query != "" {
			return "", fmt.Errorf("no se encontro un nodo AST que coincida con la query proporcionada")
		}
		return "", fmt.Errorf("no se encontro un nodo AST que coincida con type=%s", request.Selector.Type)
	}
	if request.Selector.Index > len(matches) {
		if request.Selector.Query != "" {
			return "", fmt.Errorf("solo hay %d coincidencias AST para la query proporcionada", len(matches))
		}
		return "", fmt.Errorf("solo hay %d coincidencias AST para type=%s", len(matches), request.Selector.Type)
	}
	if request.Selector.Index == 1 && len(matches) > 1 && request.Selector.Name == "" && request.Selector.Contains == "" && request.Selector.Query == "" {
		return "", fmt.Errorf("el selector AST coincide en %d nodos; agrega name, contains o index para desambiguar", len(matches))
	}
	if request.Selector.Index == 1 && len(matches) > 1 && request.Selector.Query != "" {
		return "", fmt.Errorf("la query AST coincide en %d nodos; agrega index para desambiguar", len(matches))
	}
	target := matches[request.Selector.Index-1]
	start := int(target.StartByte())
	end := int(target.EndByte())
	updated, err := applyASTAction(string(content), start, end, request)
	if err != nil {
		return "", err
	}
	return updated, nil
}

func applyASTAction(content string, start, end int, request *astPatch) (string, error) {
	switch normalizeASTAction(request.Action) {
	case "replace":
		return content[:start] + request.Replace + content[end:], nil
	case "delete":
		return content[:start] + content[end:], nil
	case "insert_before":
		text := request.Replace
		if len(text) > 0 && text[len(text)-1] != '\n' {
			text += "\n"
		}
		return content[:start] + text + content[start:], nil
	case "insert_after":
		text := request.Replace
		if len(text) > 0 && text[0] != '\n' {
			text = "\n" + text
		}
		return content[:end] + text + content[end:], nil
	default:
		return "", fmt.Errorf("accion AST no soportada: %s", request.Action)
	}
}

func findASTMatches(root *sitter.Node, content []byte, lang *sitter.Language, selector astSelector) ([]*sitter.Node, error) {
	if selector.Query != "" {
		return findASTQueryMatches(root, content, lang, selector)
	}
	var matches []*sitter.Node
	walkNamedAST(root, func(node *sitter.Node) {
		if node == nil || node.Type() != selector.Type {
			return
		}
		if selector.Name != "" && astNodeName(node, content) != selector.Name {
			return
		}
		if selector.Contains != "" && !strings.Contains(strings.TrimSpace(node.Content(content)), selector.Contains) {
			return
		}
		matches = append(matches, node)
	})
	return matches, nil
}

func findASTQueryMatches(root *sitter.Node, content []byte, lang *sitter.Language, selector astSelector) ([]*sitter.Node, error) {
	query, err := sitter.NewQuery([]byte(selector.Query), lang)
	if err != nil {
		return nil, fmt.Errorf("query AST invalida: %w", err)
	}
	defer query.Close()
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.Exec(query, root)
	var matches []*sitter.Node
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		match = cursor.FilterPredicates(match, content)
		if match == nil {
			continue
		}
		node, err := selectQueryCaptureNode(query, match, selector.Capture)
		if err != nil {
			return nil, err
		}
		if node == nil {
			continue
		}
		matches = append(matches, node)
	}
	return matches, nil
}

func selectQueryCaptureNode(query *sitter.Query, match *sitter.QueryMatch, captureName string) (*sitter.Node, error) {
	if match == nil {
		return nil, nil
	}
	explicitCapture := strings.TrimSpace(captureName) != ""
	if captureName == "" {
		captureName = "target"
	}
	var fallback *sitter.Node
	for _, capture := range match.Captures {
		name := query.CaptureNameForId(capture.Index)
		if name == captureName {
			return capture.Node, nil
		}
		if fallback == nil {
			fallback = capture.Node
		}
	}
	if !explicitCapture && len(match.Captures) == 1 && fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("la query AST no produjo la captura requerida: %s", captureName)
}

func astNodeName(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		return strings.TrimSpace(nameNode.Content(content))
	}
	if parent := node.Parent(); parent != nil {
		if nameNode := parent.ChildByFieldName("name"); nameNode != nil {
			return strings.TrimSpace(nameNode.Content(content))
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "identifier", "type_identifier", "field_identifier", "property_identifier", "shorthand_property_identifier_pattern", "shorthand_property_identifier":
			return strings.TrimSpace(child.Content(content))
		}
	}
	return ""
}

func walkNamedAST(node *sitter.Node, visit func(*sitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkNamedAST(node.NamedChild(i), visit)
	}
}

func treeSitterLanguageForPath(path string) (*sitter.Language, string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return golang.GetLanguage(), "go"
	case ".py":
		return python.GetLanguage(), "python"
	case ".rs":
		return rust.GetLanguage(), "rust"
	case ".cpp", ".cc", ".cxx", ".hpp", ".h":
		return cpp.GetLanguage(), "cpp"
	case ".rb":
		return ruby.GetLanguage(), "ruby"
	case ".js", ".jsx":
		return javascript.GetLanguage(), "javascript"
	case ".ts":
		return typescript.GetLanguage(), "typescript"
	case ".tsx":
		return tsx.GetLanguage(), "tsx"
	case ".css":
		return css.GetLanguage(), "css"
	case ".html", ".htm":
		return html.GetLanguage(), "html"
	case ".yaml", ".yml":
		return yaml.GetLanguage(), "yaml"
	default:
		return nil, ""
	}
}

func (p *astPatch) Render() string {
	var selectorLines []string
	if p.Selector.Query != "" {
		if action := normalizeASTAction(p.Action); action != "" && action != "replace" {
			selectorLines = append(selectorLines, "action: "+action)
		}
		if p.Selector.Capture != "" {
			selectorLines = append(selectorLines, "capture: "+p.Selector.Capture)
		}
		if p.Selector.Index > 1 {
			selectorLines = append(selectorLines, fmt.Sprintf("index: %d", p.Selector.Index))
		}
		selectorLines = append(selectorLines, "query:")
		selectorLines = append(selectorLines, p.Selector.Query)
	} else {
		selectorLines = append(selectorLines, "type: "+p.Selector.Type)
		if action := normalizeASTAction(p.Action); action != "" && action != "replace" {
			selectorLines = append(selectorLines, "action: "+action)
		}
		if p.Selector.Name != "" {
			selectorLines = append(selectorLines, "name: "+p.Selector.Name)
		}
		if p.Selector.Contains != "" {
			selectorLines = append(selectorLines, "contains: "+p.Selector.Contains)
		}
		if p.Selector.Index > 1 {
			selectorLines = append(selectorLines, fmt.Sprintf("index: %d", p.Selector.Index))
		}
	}
	return strings.Join([]string{
		p.Path,
		patchASTMarker,
		strings.Join(selectorLines, "\n"),
		patchDividerMarker,
		p.Replace,
		patchReplaceMarker,
	}, "\n")
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
