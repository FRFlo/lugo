package lsp

import (
	"slices"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/token"
)

// fiveMStaticEventName evaluates only immutable local string expressions.
// Runtime-dependent names stay unindexed instead of inventing an event link.
func (doc *Document) fiveMStaticEventName(id ast.NodeID, depth int) (string, bool) {
	if doc == nil || doc.Tree == nil || doc.Resolver == nil || id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) || depth > 20 {
		return "", false
	}
	node := doc.Tree.Nodes[id]
	switch node.Kind {
	case ast.KindString:
		return decodeLuaString(string(doc.nodeSource(id)))
	case ast.KindParenExpr:
		return doc.fiveMStaticEventName(node.Left, depth+1)
	case ast.KindBinaryExpr:
		if token.Kind(node.Extra) != token.Concat {
			return "", false
		}
		left, leftOK := doc.fiveMStaticEventName(node.Left, depth+1)
		right, rightOK := doc.fiveMStaticEventName(node.Right, depth+1)
		if leftOK && rightOK {
			return left + right, true
		}
	case ast.KindIdent:
		def := doc.referenceAt(id)
		if def == ast.InvalidNode || !slices.Contains(doc.Resolver.LocalDefs, def) {
			return "", false
		}
		for _, assignment := range doc.Resolver.Reassignments {
			if assignment.DefID == def {
				return "", false
			}
		}
		value := doc.getAssignedValue(def)
		if value != ast.InvalidNode && value != id {
			return doc.fiveMStaticEventName(value, depth+1)
		}
	}
	return "", false
}
