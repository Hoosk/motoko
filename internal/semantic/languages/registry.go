package languages

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	sitter "github.com/smacker/go-tree-sitter"
)

type Extractor interface {
	SitterLanguage() *sitter.Language
	LanguageID() string
	IsImportNode(node *sitter.Node) bool
	ExtractImportPath(node *sitter.Node, content []byte) string
	ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool)
	IsExportNode(node *sitter.Node) bool
	IsChildOfExport(node *sitter.Node) bool
}

var registry = make(map[string]Extractor)

func Register(exts []string, extractor Extractor) {
	for _, ext := range exts {
		registry[ext] = extractor
	}
}

func GetExtractor(ext string) (Extractor, bool) {
	e, ok := registry[ext]
	return e, ok
}
