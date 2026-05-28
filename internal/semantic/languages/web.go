package languages

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"strings"
)

type WebExtractor struct {
	lang   *sitter.Language
	langID string
}

func init() {
	exts := []string{".js", ".jsx"}
	extractor := &WebExtractor{lang: javascript.GetLanguage(), langID: "javascript"}
	Register(exts, extractor)
	Register([]string{"javascript", "js", "jsx"}, extractor)

	tsExts := []string{".ts"}
	tsExtractor := &WebExtractor{lang: typescript.GetLanguage(), langID: "typescript"}
	Register(tsExts, tsExtractor)
	Register([]string{"typescript", "ts"}, tsExtractor)

	tsxExts := []string{".tsx"}
	tsxExtractor := &WebExtractor{lang: tsx.GetLanguage(), langID: "tsx"}
	Register(tsxExts, tsxExtractor)
	Register([]string{"tsx"}, tsxExtractor)
}

func (e *WebExtractor) SitterLanguage() *sitter.Language {
	return e.lang
}

func (e *WebExtractor) LanguageID() string {
	return e.langID
}

func (e *WebExtractor) IsImportNode(node *sitter.Node) bool {
	return node.Type() == "import_statement" || node.Type() == "import_declaration"
}

func (e *WebExtractor) ExtractImportPath(node *sitter.Node, content []byte) string {
	sourceNode := node.ChildByFieldName("source")
	if sourceNode != nil {
		return strings.Trim(string(sourceNode.Content(content)), "'\"")
	}
	return ""
}

func (e *WebExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	switch node.Type() {
	case "function_declaration":
		return SymbolFromField(node, content, "name", "func")
	case "class_declaration":
		return SymbolFromField(node, content, "name", "class")
	case "method_definition":
		return SymbolFromField(node, content, "name", "method")
	case "interface_declaration":
		return SymbolFromField(node, content, "name", "interface")
	case "type_alias_declaration":
		return SymbolFromField(node, content, "name", "type")
	case "variable_declarator":
		return SymbolFromField(node, content, "name", "var")
	default:
		return symtypes.Symbol{}, false
	}
}

func (e *WebExtractor) IsExportNode(node *sitter.Node) bool {
	t := node.Type()
	return t == "export_statement" || t == "export_declaration"
}

func (e *WebExtractor) IsChildOfExport(node *sitter.Node) bool {
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
