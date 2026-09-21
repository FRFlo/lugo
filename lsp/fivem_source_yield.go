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

	facts := newFiveMASTFacts(doc)
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
			refs = append(refs, facts.sourceIdentifiers...)
		}
		seen := make(map[ast.NodeID]struct{}, len(refs))
		for _, refID := range refs {
			if _, ok := seen[refID]; ok {
				continue
			}
			seen[refID] = struct{}{}
			if int(refID) >= len(doc.Tree.Nodes) {
				continue
			}
			ref := doc.Tree.Nodes[refID]
			if ref.Kind != ast.KindIdent || ref.Start >= ref.End || ref.End > uint32(len(doc.Source())) || string(doc.Source()[ref.Start:ref.End]) != "source" || facts.nearestFunction[refID] != event.HandlerID {
				continue
			}
			if !fiveMSourceReferenceInHandler(doc, refID, event.HandlerID) {
				continue
			}
			for _, call := range facts.yieldCallsByFunction[event.HandlerID] {
				if call.end > ref.Start {
					continue
				}
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
