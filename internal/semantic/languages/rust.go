package languages

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"
	"strings"
)

type RustExtractor struct{}

func init() {
	Register([]string{".rs"}, &RustExtractor{})
}

func (e *RustExtractor) SitterLanguage() *sitter.Language {
	return rust.GetLanguage()
}

func (e *RustExtractor) LanguageID() string {
	return "rust"
}

func (e *RustExtractor) IsImportNode(node *sitter.Node) bool {
	return node.Type() == "use_declaration"
}

func (e *RustExtractor) ExtractImportPath(node *sitter.Node, content []byte) string {
	pathNode := node.ChildByFieldName("argument")
	if pathNode != nil {
		return strings.TrimSpace(string(pathNode.Content(content)))
	}
	return ""
}

func (e *RustExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	switch node.Type() {
	case "function_item":
		return SymbolFromField(node, content, "name", "func")
	case "struct_item":
		return SymbolFromField(node, content, "name", "struct")
	case "enum_item":
		return SymbolFromField(node, content, "name", "enum")
	case "trait_item":
		return SymbolFromField(node, content, "name", "trait")
	case "mod_item":
		return SymbolFromField(node, content, "name", "mod")
	default:
		return symtypes.Symbol{}, false
	}
}

func (e *RustExtractor) IsExportNode(node *sitter.Node) bool {
	return false
}

func (e *RustExtractor) IsChildOfExport(node *sitter.Node) bool {
	return false
}
