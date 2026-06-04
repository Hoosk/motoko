package semantic

import (
	"context"
	"path/filepath"
	"unicode"

	"github.com/Hoosk/motoko/internal/semantic/languages"
	sitter "github.com/smacker/go-tree-sitter"
)

func isSupported(path string) bool {
	_, ok := languages.GetExtractor(filepath.Ext(path))
	return ok
}

func languageForPath(path string) (*sitter.Language, string) {
	ext := filepath.Ext(path)
	if extractor, ok := languages.GetExtractor(ext); ok {
		return extractor.SitterLanguage(), extractor.LanguageID()
	}
	return nil, ""
}

func extractSymbolsAndDeps(content []byte, lang *sitter.Language, language string) ([]Symbol, []string, []string) {
	if lang == nil {
		return nil, nil, nil
	}

	extractor, ok := languages.GetExtractor("." + language)
	if !ok {
		extractor, _ = languages.GetExtractor(language)
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

	languages.WalkNamed(root, func(node *sitter.Node) {
		if extractor != nil && extractor.IsImportNode(node) {
			path := extractor.ExtractImportPath(node, content)
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

		if extractor != nil {
			symbol, ok := extractor.ClassifySymbol(node, content)
			if ok && symbol.Name != "" {
				key := symbol.Kind + ":" + symbol.Name
				if _, exists := seenSymbols[key]; !exists {
					seenSymbols[key] = struct{}{}
					symbols = append(symbols, symbol)

					if language == "go" && len(symbol.Name) > 0 && unicode.IsUpper(rune(symbol.Name[0])) {
						exports = append(exports, symbol.Name)
					}
					if extractor.IsChildOfExport(node) {
						exports = append(exports, symbol.Name)
					}
				}
			}
		}
	})
	return symbols, imports, exports
}

