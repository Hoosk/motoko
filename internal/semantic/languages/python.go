package languages

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
	"strings"
)

type PythonExtractor struct{}

func init() {
	Register([]string{".py"}, &PythonExtractor{})
}

func (e *PythonExtractor) SitterLanguage() *sitter.Language {
	return python.GetLanguage()
}

func (e *PythonExtractor) LanguageID() string {
	return "python"
}

func (e *PythonExtractor) IsImportNode(node *sitter.Node) bool {
	return node.Type() == "import_from_statement" || node.Type() == "import_statement"
}

func (e *PythonExtractor) ExtractImportPath(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("module_name")
	if nameNode == nil {
		nameNode = node.ChildByFieldName("name")
	}
	if nameNode != nil {
		return strings.TrimSpace(string(nameNode.Content(content)))
	}
	return ""
}

func (e *PythonExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	switch node.Type() {
	case "function_definition":
		return SymbolFromField(node, content, "name", "func")
	case "class_definition":
		return SymbolFromField(node, content, "name", "class")
	default:
		return symtypes.Symbol{}, false
	}
}

func (e *PythonExtractor) IsExportNode(node *sitter.Node) bool {
	return false
}

func (e *PythonExtractor) IsChildOfExport(node *sitter.Node) bool {
	return false
}
