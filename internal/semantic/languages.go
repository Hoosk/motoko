package semantic

import (
	"context"
	"path/filepath"
	"strings"
	"unicode"

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

func isSupported(path string) bool {
	_, language := languageForPath(path)
	return language != ""
}

func languageForPath(path string) (*sitter.Language, string) {
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

func extractSymbolsAndDeps(content []byte, lang *sitter.Language, language string) ([]Symbol, []string, []string) {
	if lang == nil {
		return nil, nil, nil
	}
	root, _ := sitter.ParseCtx(context.Background(), content, lang)
	if root == nil {
		return nil, nil, nil
	}
	var symbols []Symbol
	var imports []string
	var exports []string

	seenSymbols := make(map[string]struct{})
	seenImports := make(map[string]struct{})

	walkNamed(root, func(node *sitter.Node) {
		if isImportNode(node, language) {
			path := extractImportPath(node, content, language)
			if path != "" {
				if _, exists := seenImports[path]; !exists {
					seenImports[path] = struct{}{}
					imports = append(imports, path)
				}
			}
			return
		}

		if len(symbols) >= maxSymbolsPerFile {
			return
		}

		symbol, ok := classifySymbol(node, content, language)
		if ok && symbol.Name != "" {
			key := symbol.Kind + ":" + symbol.Name
			if _, exists := seenSymbols[key]; !exists {
				seenSymbols[key] = struct{}{}
				symbols = append(symbols, symbol)

				if language == "go" && len(symbol.Name) > 0 && unicode.IsUpper(rune(symbol.Name[0])) {
					exports = append(exports, symbol.Name)
				}
				if (language == "javascript" || language == "typescript" || language == "tsx") && isChildOfExport(node) {
					exports = append(exports, symbol.Name)
				}
			}
		}
	})
	return symbols, imports, exports
}

func isImportNode(node *sitter.Node, language string) bool {
	t := node.Type()
	switch language {
	case "go":
		return t == "import_spec"
	case "python":
		return t == "import_from_statement" || t == "import_statement"
	case "rust":
		return t == "use_declaration"
	case "cpp":
		return t == "preproc_include"
	case "javascript", "typescript", "tsx":
		return t == "import_statement" || t == "import_declaration"
	}
	return false
}

func isExportNode(node *sitter.Node, language string) bool {
	t := node.Type()
	switch language {
	case "javascript", "typescript", "tsx":
		return t == "export_statement" || t == "export_declaration"
	}
	return false
}

func isChildOfExport(node *sitter.Node) bool {
	p := node.Parent()
	for p != nil {
		t := p.Type()
		if t == "export_statement" || t == "export_declaration" {
			return true
		}
		p = p.Parent()
	}
	return false
}

func extractImportPath(node *sitter.Node, content []byte, language string) string {
	switch language {
	case "go":
		pathNode := node.ChildByFieldName("path")
		if pathNode == nil {
			for i := 0; i < int(node.NamedChildCount()); i++ {
				c := node.NamedChild(i)
				if c.Type() == "interpreted_string_literal" || c.Type() == "raw_string_literal" {
					pathNode = c
					break
				}
			}
		}
		if pathNode != nil {
			return strings.Trim(string(pathNode.Content(content)), "\"")
		}
	case "python":
		nameNode := node.ChildByFieldName("module_name")
		if nameNode == nil {
			nameNode = node.ChildByFieldName("name")
		}
		if nameNode != nil {
			return strings.TrimSpace(string(nameNode.Content(content)))
		}
	case "rust":
		pathNode := node.ChildByFieldName("argument")
		if pathNode != nil {
			return strings.TrimSpace(string(pathNode.Content(content)))
		}
	case "cpp":
		pathNode := node.ChildByFieldName("path")
		if pathNode != nil {
			return strings.Trim(string(pathNode.Content(content)), "<>\"")
		}
	case "javascript", "typescript", "tsx":
		sourceNode := node.ChildByFieldName("source")
		if sourceNode != nil {
			return strings.Trim(string(sourceNode.Content(content)), "'\"")
		}
	}
	return ""
}

func extractSymbols(content []byte, lang *sitter.Language, language string) []Symbol {
	s, _, _ := extractSymbolsAndDeps(content, lang, language)
	return s
}

func walkNamed(node *sitter.Node, visit func(*sitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkNamed(node.NamedChild(i), visit)
	}
}

func classifySymbol(node *sitter.Node, content []byte, language string) (Symbol, bool) {
	switch language {
	case "go":
		return classifyGoSymbol(node, content)
	case "python":
		return classifyPythonSymbol(node, content)
	case "rust":
		return classifyRustSymbol(node, content)
	case "cpp":
		return classifyCppSymbol(node, content)
	case "ruby":
		return classifyRubySymbol(node, content)
	case "javascript", "typescript", "tsx":
		return classifyJSSymbol(node, content)
	case "css":
		return classifyCssSymbol(node, content)
	case "html":
		return classifyHtmlSymbol(node, content)
	case "yaml":
		return classifyYamlSymbol(node, content)
	default:
		return Symbol{}, false
	}
}

func classifyGoSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_declaration":
		return symbolFromField(node, content, "name", "func")
	case "method_declaration":
		return symbolFromField(node, content, "name", "method")
	case "type_spec":
		return symbolFromField(node, content, "name", "type")
	case "var_spec":
		return symbolFromFirstNamedChild(node, content, "var")
	case "const_spec":
		return symbolFromFirstNamedChild(node, content, "const")
	default:
		return Symbol{}, false
	}
}

func classifyPythonSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_definition":
		return symbolFromField(node, content, "name", "func")
	case "class_definition":
		return symbolFromField(node, content, "name", "class")
	default:
		return Symbol{}, false
	}
}

func classifyRustSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_item":
		return symbolFromField(node, content, "name", "func")
	case "struct_item":
		return symbolFromField(node, content, "name", "struct")
	case "enum_item":
		return symbolFromField(node, content, "name", "enum")
	case "trait_item":
		return symbolFromField(node, content, "name", "trait")
	case "impl_item":
		return Symbol{}, false
	case "mod_item":
		return symbolFromField(node, content, "name", "mod")
	default:
		return Symbol{}, false
	}
}

func classifyCppSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_definition":
		declarator := node.ChildByFieldName("declarator")
		if declarator != nil {
			return symbolFromFirstNamedChild(declarator, content, "func")
		}
	case "class_specifier":
		return symbolFromField(node, content, "name", "class")
	case "struct_specifier":
		return symbolFromField(node, content, "name", "struct")
	}
	return Symbol{}, false
}

func classifyRubySymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "method":
		return symbolFromField(node, content, "name", "method")
	case "class":
		return symbolFromField(node, content, "name", "class")
	case "module":
		return symbolFromField(node, content, "name", "module")
	}
	return Symbol{}, false
}

func classifyJSSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_declaration":
		return symbolFromField(node, content, "name", "func")
	case "class_declaration":
		return symbolFromField(node, content, "name", "class")
	case "method_definition":
		return symbolFromField(node, content, "name", "method")
	case "interface_declaration":
		return symbolFromField(node, content, "name", "interface")
	case "type_alias_declaration":
		return symbolFromField(node, content, "name", "type")
	case "variable_declarator":
		return symbolFromField(node, content, "name", "var")
	default:
		return Symbol{}, false
	}
}

func classifyCssSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	if node.Type() == "rule_set" {
		selectors := node.ChildByFieldName("selectors")
		if selectors != nil {
			return createSymbol(node, selectors, strings.TrimSpace(string(selectors.Content(content))), "rule")
		}
	}
	return Symbol{}, false
}

func classifyHtmlSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	if node.Type() == "element" {
		startTag := node.Child(0)
		if startTag != nil && startTag.Type() == "start_tag" {
			nameNode := startTag.Child(1)
			if nameNode != nil {
				return createSymbol(node, nameNode, string(nameNode.Content(content)), "tag")
			}
		}
	}
	return Symbol{}, false
}

func classifyYamlSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	if node.Type() == "block_mapping_pair" {
		keyNode := node.ChildByFieldName("key")
		if keyNode != nil {
			return createSymbol(node, keyNode, strings.TrimSpace(string(keyNode.Content(content))), "key")
		}
	}
	return Symbol{}, false
}

func createSymbol(node, nameNode *sitter.Node, name, kind string) (Symbol, bool) {
	return Symbol{
		Name: name,
		Kind: kind,
		Line: int(nameNode.StartPoint().Row) + 1,
		Range: LineRange{
			Start: int(node.StartPoint().Row) + 1,
			End:   int(node.EndPoint().Row) + 1,
		},
	}, true
}

func symbolFromField(node *sitter.Node, content []byte, field, kind string) (Symbol, bool) {
	nameNode := node.ChildByFieldName(field)
	if nameNode == nil {
		return Symbol{}, false
	}
	name := strings.TrimSpace(nameNode.Content(content))
	if name == "" {
		return Symbol{}, false
	}
	return createSymbol(node, nameNode, name, kind)
}

func symbolFromFirstNamedChild(node *sitter.Node, content []byte, kind string) (Symbol, bool) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		name := strings.TrimSpace(child.Content(content))
		if name != "" {
			return createSymbol(node, child, name, kind)
		}
	}
	return Symbol{}, false
}
