package languages

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"
	"strings"
)

type CppExtractor struct{}

func init() {
	Register([]string{".cpp", ".cc", ".cxx", ".hpp", ".h"}, &CppExtractor{})
}

func (e *CppExtractor) SitterLanguage() *sitter.Language {
	return cpp.GetLanguage()
}

func (e *CppExtractor) LanguageID() string {
	return "cpp"
}

func (e *CppExtractor) IsImportNode(node *sitter.Node) bool {
	return node.Type() == "preproc_include"
}

func (e *CppExtractor) ExtractImportPath(node *sitter.Node, content []byte) string {
	pathNode := node.ChildByFieldName("path")
	if pathNode != nil {
		return strings.Trim(string(pathNode.Content(content)), "<>\"")
	}
	return ""
}

func (e *CppExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	switch node.Type() {
	case "function_definition":
		declarator := node.ChildByFieldName("declarator")
		if declarator != nil {
			return SymbolFromFirstNamedChild(declarator, content, "func")
		}
	case "class_specifier":
		return SymbolFromField(node, content, "name", "class")
	case "struct_specifier":
		return SymbolFromField(node, content, "name", "struct")
	}
	return symtypes.Symbol{}, false
}

func (e *CppExtractor) IsExportNode(node *sitter.Node) bool {
	return false
}

func (e *CppExtractor) IsChildOfExport(node *sitter.Node) bool {
	return false
}
