package languages

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"strings"
)

type GoExtractor struct{}

func init() {
	Register([]string{".go"}, &GoExtractor{})
}

func (e *GoExtractor) SitterLanguage() *sitter.Language {
	return golang.GetLanguage()
}

func (e *GoExtractor) LanguageID() string {
	return "go"
}

func (e *GoExtractor) IsImportNode(node *sitter.Node) bool {
	return node.Type() == "import_spec"
}

func (e *GoExtractor) ExtractImportPath(node *sitter.Node, content []byte) string {
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
	return ""
}

func (e *GoExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	switch node.Type() {
	case "function_declaration":
		return SymbolFromField(node, content, "name", "func")
	case "method_declaration":
		return SymbolFromField(node, content, "name", "method")
	case "type_spec":
		return SymbolFromField(node, content, "name", "type")
	case "var_spec":
		return SymbolFromFirstNamedChild(node, content, "var")
	case "const_spec":
		return SymbolFromFirstNamedChild(node, content, "const")
	default:
		return symtypes.Symbol{}, false
	}
}

func (e *GoExtractor) IsExportNode(node *sitter.Node) bool {
	return false
}

func (e *GoExtractor) IsChildOfExport(node *sitter.Node) bool {
	return false
}
