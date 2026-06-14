package patch

import (
	"context"
	"fmt"
	"path/filepath"
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
	case actionReplace:
		return content[:start] + request.Replace + content[end:], nil
	case actionDelete:
		return content[:start] + content[end:], nil
	case actionInsertBefore:
		text := request.Replace
		if len(text) > 0 && text[len(text)-1] != '\n' {
			text += "\n"
		}
		return content[:start] + text + content[start:], nil
	case actionInsertAfter:
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
		astMarker,
		strings.Join(selectorLines, "\n"),
		dividerMarker,
		p.Replace,
		replaceMarker,
	}, "\n")
}

func diffPreview(search, replace string) string {
	return strings.Join([]string{
		searchMarker,
		search,
		dividerMarker,
		replace,
		replaceMarker,
	}, "\n")
}
