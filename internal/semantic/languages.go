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

func languageForPath(path string) (languages.Extractor, string) {
	ext := filepath.Ext(path)
	if extractor, ok := languages.GetExtractor(ext); ok {
		return extractor, extractor.LanguageID()
	}
	return nil, ""
}

func extractSymbolsAndDeps(content []byte, extractor languages.Extractor, language string) ([]Symbol, []string, []string) {
	if extractor == nil {
		return nil, nil, nil
	}
	lang := extractor.SitterLanguage()

	// Parse with an explicit parser so the C-allocated tree is released as soon
	// as extraction finishes instead of waiting for finalizers; the whole
	// workspace is re-parsed on every refresh.
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(lang)
	tree, _ := parser.ParseCtx(context.Background(), nil, content)
	if tree == nil {
		return nil, nil, nil
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil {
		return nil, nil, nil
	}
	var symbols []Symbol
	var imports []string
	var exports []string

	seenSymbols := make(map[string]struct{})
	seenImports := make(map[string]struct{})

	languages.WalkNamed(root, func(node *sitter.Node) {
		if extractor.IsImportNode(node) {
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

		symbol, ok := extractor.ClassifySymbol(node, content)
		if !ok || symbol.Name == "" {
			return
		}
		key := symbol.Kind + ":" + symbol.Name
		if _, exists := seenSymbols[key]; exists {
			return
		}
		seenSymbols[key] = struct{}{}
		symbols = append(symbols, symbol)

		if language == "go" && len(symbol.Name) > 0 && unicode.IsUpper(rune(symbol.Name[0])) {
			exports = append(exports, symbol.Name)
		}
		if extractor.IsChildOfExport(node) {
			exports = append(exports, symbol.Name)
		}
	})
	return symbols, imports, exports
}
