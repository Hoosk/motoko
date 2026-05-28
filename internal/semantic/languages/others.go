package languages

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/yaml"
	"strings"
)

type RubyExtractor struct{}

func init() {
	e := &RubyExtractor{}
	Register([]string{".rb"}, e)
	Register([]string{"ruby"}, e)
}
func (e *RubyExtractor) SitterLanguage() *sitter.Language                           { return ruby.GetLanguage() }
func (e *RubyExtractor) LanguageID() string                                         { return "ruby" }
func (e *RubyExtractor) IsImportNode(node *sitter.Node) bool                        { return false }
func (e *RubyExtractor) ExtractImportPath(node *sitter.Node, content []byte) string { return "" }
func (e *RubyExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	switch node.Type() {
	case "method":
		return SymbolFromField(node, content, "name", "method")
	case "class":
		return SymbolFromField(node, content, "name", "class")
	case "module":
		return SymbolFromField(node, content, "name", "module")
	}
	return symtypes.Symbol{}, false
}
func (e *RubyExtractor) IsExportNode(node *sitter.Node) bool    { return false }
func (e *RubyExtractor) IsChildOfExport(node *sitter.Node) bool { return false }

type CssExtractor struct{}

func init() {
	e := &CssExtractor{}
	Register([]string{".css"}, e)
	Register([]string{"css"}, e)
}
func (e *CssExtractor) SitterLanguage() *sitter.Language                           { return css.GetLanguage() }
func (e *CssExtractor) LanguageID() string                                         { return "css" }
func (e *CssExtractor) IsImportNode(node *sitter.Node) bool                        { return false }
func (e *CssExtractor) ExtractImportPath(node *sitter.Node, content []byte) string { return "" }
func (e *CssExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	if node.Type() == "rule_set" {
		selectors := node.ChildByFieldName("selectors")
		if selectors != nil {
			return CreateSymbol(node, selectors, strings.TrimSpace(string(selectors.Content(content))), "rule")
		}
	}
	return symtypes.Symbol{}, false
}
func (e *CssExtractor) IsExportNode(node *sitter.Node) bool    { return false }
func (e *CssExtractor) IsChildOfExport(node *sitter.Node) bool { return false }

type HtmlExtractor struct{}

func init() {
	e := &HtmlExtractor{}
	Register([]string{".html", ".htm"}, e)
	Register([]string{"html"}, e)
}
func (e *HtmlExtractor) SitterLanguage() *sitter.Language                           { return html.GetLanguage() }
func (e *HtmlExtractor) LanguageID() string                                         { return "html" }
func (e *HtmlExtractor) IsImportNode(node *sitter.Node) bool                        { return false }
func (e *HtmlExtractor) ExtractImportPath(node *sitter.Node, content []byte) string { return "" }
func (e *HtmlExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	if node.Type() == "element" {
		startTag := node.Child(0)
		if startTag != nil && startTag.Type() == "start_tag" {
			nameNode := startTag.Child(1)
			if nameNode != nil {
				return CreateSymbol(node, nameNode, string(nameNode.Content(content)), "tag")
			}
		}
	}
	return symtypes.Symbol{}, false
}
func (e *HtmlExtractor) IsExportNode(node *sitter.Node) bool    { return false }
func (e *HtmlExtractor) IsChildOfExport(node *sitter.Node) bool { return false }

type YamlExtractor struct{}

func init() {
	e := &YamlExtractor{}
	Register([]string{".yaml", ".yml"}, e)
	Register([]string{"yaml"}, e)
}
func (e *YamlExtractor) SitterLanguage() *sitter.Language                           { return yaml.GetLanguage() }
func (e *YamlExtractor) LanguageID() string                                         { return "yaml" }
func (e *YamlExtractor) IsImportNode(node *sitter.Node) bool                        { return false }
func (e *YamlExtractor) ExtractImportPath(node *sitter.Node, content []byte) string { return "" }
func (e *YamlExtractor) ClassifySymbol(node *sitter.Node, content []byte) (symtypes.Symbol, bool) {
	if node.Type() == "block_mapping_pair" {
		keyNode := node.ChildByFieldName("key")
		if keyNode != nil {
			return CreateSymbol(node, keyNode, strings.TrimSpace(string(keyNode.Content(content))), "key")
		}
	}
	return symtypes.Symbol{}, false
}
func (e *YamlExtractor) IsExportNode(node *sitter.Node) bool    { return false }
func (e *YamlExtractor) IsChildOfExport(node *sitter.Node) bool { return false }
