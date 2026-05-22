package patch

import (
	"fmt"
	"strconv"
	"strings"
)

func parsePatchRequest(input string) (request, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return request{}, fmt.Errorf("uso: primera linea con la ruta seguida del bloque SEARCH/REPLACE o un unified diff")
	}
	if strings.HasPrefix(trimmed, "--- ") || strings.HasPrefix(trimmed, "diff --git ") {
		patch, err := parseUnifiedPatch(trimmed)
		if err != nil {
			return request{}, err
		}
		return request{Unified: patch}, nil
	}
	path, body, err := splitPatchPathAndBody(trimmed)
	if err != nil {
		return request{}, err
	}
	if strings.HasPrefix(body, astMarker) {
		astPatches, err := parseASTPatchInput(path, body)
		if err != nil {
			return request{}, err
		}
		return request{Path: path, AST: astPatches}, nil
	}
	path, search, replace, err := parsePatchInput(trimmed)
	if err != nil {
		return request{}, err
	}
	return request{Path: path, Search: search, Replace: replace}, nil
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

	searchIndex := strings.Index(body, searchMarker)
	dividerIndex := strings.Index(body, dividerMarker)
	replaceIndex := strings.Index(body, replaceMarker)
	if searchIndex != 0 || dividerIndex == -1 || replaceIndex == -1 || dividerIndex > replaceIndex {
		return "", "", "", fmt.Errorf("formato invalido; usa SEARCH/REPLACE con este esquema:\n%s\n<old>\n%s\n<new>\n%s", searchMarker, dividerMarker, replaceMarker)
	}

	search := strings.TrimPrefix(body[:dividerIndex], searchMarker)
	search = strings.TrimPrefix(search, "\n")

	replace := body[dividerIndex+len(dividerMarker) : replaceIndex]
	replace = strings.TrimPrefix(replace, "\n")

	trailer := strings.TrimSpace(body[replaceIndex+len(replaceMarker):])
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
		if lines[i] != astMarker {
			return nil, fmt.Errorf("formato AST invalido; se esperaba %s y se encontro: %s", astMarker, lines[i])
		}
		dividerIndex := -1
		replaceIndex := -1
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == dividerMarker {
				dividerIndex = j
				break
			}
		}
		if dividerIndex == -1 {
			return nil, fmt.Errorf("formato AST invalido; falta marcador %s", dividerMarker)
		}
		for j := dividerIndex + 1; j < len(lines); j++ {
			if lines[j] == replaceMarker {
				replaceIndex = j
				break
			}
		}
		if replaceIndex == -1 {
			return nil, fmt.Errorf("formato AST invalido; falta marcador %s", replaceMarker)
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
