package lsp

import (
	"strings"

	"github.com/coalaura/lugo/ast"
)

// buildFiveMSourceAfterYieldDiagnostics warns when an event handler reads the
// ephemeral server event source after a call that can suspend the coroutine.
// The analysis deliberately recognizes only well-known yield spellings; Lua
// cannot provide enough static information to safely infer arbitrary calls.
func (s *Server) buildFiveMSourceAfterYieldDiagnostics(doc *Document) []Diagnostic {
	if doc == nil || s.getDocumentFiveMProfile(doc).Env() != EnvServer {
		return nil
	}

	var out []Diagnostic
	for _, event := range doc.FiveMEvents {
		if event.Kind != FiveMEventAddHandler && event.Kind != FiveMEventRegisterNet {
			continue
		}
		if event.HandlerID == ast.InvalidNode || int(event.HandlerID) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[event.HandlerID].Kind != ast.KindFunctionExpr {
			continue
		}

		refs := append([]ast.NodeID(nil), doc.Resolver.GlobalRefs...)
		if fiveMHandlerHasSourceParameter(doc, event.HandlerID) {
			for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
				node := doc.Tree.Nodes[id]
				if node.Kind == ast.KindIdent && node.Start < node.End && node.End <= uint32(len(doc.Source())) && string(doc.Source()[node.Start:node.End]) == "source" {
					refs = append(refs, id)
				}
			}
		}
		seen := make(map[ast.NodeID]struct{}, len(refs))
		for _, refID := range refs {
			if _, ok := seen[refID]; ok {
				continue
			}
			seen[refID] = struct{}{}
			ref := doc.Tree.Nodes[refID]
			if !isDescendantOfNode(doc.Tree, refID, event.HandlerID) || (ref.Kind != ast.KindIdent || string(doc.Source()[ref.Start:ref.End]) != "source") {
				continue
			}
			if !fiveMSourceReferenceInHandler(doc, refID, event.HandlerID) {
				continue
			}
			if nearestFunction(doc.Tree, refID) != event.HandlerID {
				continue
			}

			for nodeID := ast.NodeID(1); int(nodeID) < len(doc.Tree.Nodes); nodeID++ {
				node := doc.Tree.Nodes[nodeID]
				if node.Start >= ref.Start || node.End > ref.Start || !isDescendantOfNode(doc.Tree, nodeID, event.HandlerID) || nearestFunction(doc.Tree, nodeID) != event.HandlerID {
					continue
				}
				if isFiveMYieldCall(doc, nodeID) {
					out = append(out, Diagnostic{
						Range:    getNodeRange(doc.Tree, refID),
						Severity: SeverityWarning,
						Code:     "fivem-source-after-yield",
						Message:  "Capture 'source' in a local before Wait/await; the server event source is not reliable after yielding.",
					})
					break
				}
			}
		}
	}
	return out
}

func fiveMSourceReferenceInHandler(doc *Document, id, handler ast.NodeID) bool {
	return doc.Server.isFiveMServerEventSourceReference(doc, id) || fiveMHandlerHasSourceParameter(doc, handler)
}

func fiveMHandlerHasSourceParameter(doc *Document, handler ast.NodeID) bool {
	node := doc.Tree.Nodes[handler]
	for i := uint32(0); i < node.Count && node.Extra+i < uint32(len(doc.Tree.ExtraList)); i++ {
		paramID := doc.Tree.ExtraList[node.Extra+i]
		if int(paramID) >= len(doc.Tree.Nodes) {
			continue
		}
		param := doc.Tree.Nodes[paramID]
		if param.Start < param.End && param.End <= uint32(len(doc.Source())) && string(doc.Source()[param.Start:param.End]) == "source" {
			return true
		}
	}
	return false
}

func isDescendantOfNode(tree *ast.Tree, id, ancestor ast.NodeID) bool {
	for id != ast.InvalidNode && int(id) < len(tree.Nodes) {
		if id == ancestor {
			return true
		}
		id = tree.Nodes[id].Parent
	}
	return false
}

func nearestFunction(tree *ast.Tree, id ast.NodeID) ast.NodeID {
	for id != ast.InvalidNode && int(id) < len(tree.Nodes) {
		node := tree.Nodes[id]
		if node.Kind == ast.KindFunctionExpr || node.Kind == ast.KindFunctionStmt {
			return id
		}
		id = node.Parent
	}
	return ast.InvalidNode
}

func isFiveMYieldCall(doc *Document, id ast.NodeID) bool {
	node := doc.Tree.Nodes[id]
	if node.Kind != ast.KindCallExpr && node.Kind != ast.KindMethodCall {
		return false
	}
	if node.Left == ast.InvalidNode || int(node.Left) >= len(doc.Tree.Nodes) {
		return false
	}
	source := doc.Source()
	left := doc.Tree.Nodes[node.Left]
	if left.Start >= left.End || left.End > uint32(len(source)) {
		return false
	}
	name := string(source[left.Start:left.End])
	if node.Kind == ast.KindCallExpr {
		return name == "Wait" || name == "Await" || name == "PerformHttpRequestAwait" ||
			name == "Citizen.Wait" || name == "Citizen.Await" || name == "coroutine.yield"
	}
	if node.Right == ast.InvalidNode || int(node.Right) >= len(doc.Tree.Nodes) {
		return false
	}
	right := doc.Tree.Nodes[node.Right]
	if right.Start >= right.End || right.End > uint32(len(source)) {
		return false
	}
	method := string(source[right.Start:right.End])
	return (name == "Citizen" && (method == "Wait" || method == "Await")) ||
		(name == "coroutine" && method == "yield") ||
		strings.EqualFold(method, "await")
}
