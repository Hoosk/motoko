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
		return request{}, fmt.Errorf("usage: first line with the path followed by the SEARCH/REPLACE block or a unified diff")
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
		astPatches, astErr := parseASTPatchInput(path, body)
		if astErr != nil {
			return request{}, astErr
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
	before, after, ok := strings.Cut(input, "\n")
	if !ok {
		return "", "", fmt.Errorf("missing patch block")
	}
	path := strings.TrimSpace(before)
	body := strings.TrimSpace(after)
	if path == "" || body == "" {
		return "", "", fmt.Errorf("invalid format; missing path or patch block")
	}
	return path, body, nil
}

func parsePatchInput(input string) (string, string, string, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", "", fmt.Errorf("usage: first line with the path followed by the SEARCH/REPLACE block")
	}

	before, after, ok := strings.Cut(input, "\n")
	if !ok {
		return "", "", "", fmt.Errorf("missing SEARCH/REPLACE block")
	}

	path := strings.TrimSpace(before)
	body := strings.TrimSpace(after)

	searchIndex := strings.Index(body, searchMarker)
	dividerIndex := strings.Index(body, dividerMarker)
	replaceIndex := strings.Index(body, replaceMarker)
	if searchIndex != 0 || dividerIndex == -1 || replaceIndex == -1 || dividerIndex > replaceIndex {
		return "", "", "", fmt.Errorf("invalid format; use SEARCH/REPLACE with this scheme:\n%s\n<old>\n%s\n<new>\n%s", searchMarker, dividerMarker, replaceMarker)
	}

	search := strings.TrimPrefix(body[:dividerIndex], searchMarker)
	search = strings.TrimPrefix(search, "\n")

	replace := body[dividerIndex+len(dividerMarker) : replaceIndex]
	replace = strings.TrimPrefix(replace, "\n")

	trailer := strings.TrimSpace(body[replaceIndex+len(replaceMarker):])
	if trailer != "" {
		return "", "", "", fmt.Errorf("unexpected content after REPLACE block")
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
			return nil, fmt.Errorf("invalid AST format; expected %s (start an AST block with <<<<<<< AST, <<<<<<< SEARCH, or use unified diff with ---/+++) and found: %s", astMarker, lines[i])
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
			return nil, fmt.Errorf("invalid AST format; missing marker %s (must be EXACTLY '%s' with no trailing spaces)", dividerMarker, dividerMarker)
		}
		for j := dividerIndex + 1; j < len(lines); j++ {
			if lines[j] == replaceMarker {
				replaceIndex = j
				break
			}
		}
		if replaceIndex == -1 {
			return nil, fmt.Errorf("invalid AST format; missing marker %s", replaceMarker)
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
		return nil, fmt.Errorf("invalid AST format; contains no mutations")
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
			return astSelector{}, fmt.Errorf("invalid AST line: %s (expected key: value format; valid keys: type, name, query, capture, action, contains, index)", rawLine)
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
				return astSelector{}, fmt.Errorf("unsupported AST action: %s", value)
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
				return astSelector{}, fmt.Errorf("invalid AST index: %s", value)
			}
			selector.Index = index
		default:
			return astSelector{}, fmt.Errorf("unsupported AST key: %s (valid keys: type, name, query, capture, action, contains, index)", key)
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
		return astSelector{}, fmt.Errorf("AST block requires 'query:' or 'type: <node_type>'")
	}
	return selector, nil
}

func astActionFromSelectorBlock(block string) string {
	for rawLine := range strings.SplitSeq(strings.TrimSpace(block), "\n") {
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
	return actionReplace
}

func normalizeASTAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", actionReplace:
		return actionReplace
	case actionDelete:
		return actionDelete
	case actionInsertBefore:
		return actionInsertBefore
	case actionInsertAfter:
		return actionInsertAfter
	default:
		return ""
	}
}
