package languages

import (
	"github.com/Hoosk/motoko/internal/semantic/symtypes"
	sitter "github.com/smacker/go-tree-sitter"
	"strings"
)

func CreateSymbol(node, nameNode *sitter.Node, name, kind string) (symtypes.Symbol, bool) {
	return symtypes.Symbol{
		Name: name,
		Kind: kind,
		Line: int(nameNode.StartPoint().Row) + 1,
		Range: symtypes.LineRange{
			Start: int(node.StartPoint().Row) + 1,
			End:   int(node.EndPoint().Row) + 1,
		},
	}, true
}

func SymbolFromField(node *sitter.Node, content []byte, field, kind string) (symtypes.Symbol, bool) {
	nameNode := node.ChildByFieldName(field)
	if nameNode == nil {
		return symtypes.Symbol{}, false
	}
	name := strings.TrimSpace(nameNode.Content(content))
	if name == "" {
		return symtypes.Symbol{}, false
	}
	return CreateSymbol(node, nameNode, name, kind)
}

func SymbolFromFirstNamedChild(node *sitter.Node, content []byte, kind string) (symtypes.Symbol, bool) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		name := strings.TrimSpace(child.Content(content))
		if name != "" {
			return CreateSymbol(node, child, name, kind)
		}
	}
	return symtypes.Symbol{}, false
}

func WalkNamed(node *sitter.Node, visit func(*sitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		WalkNamed(node.NamedChild(i), visit)
	}
}
