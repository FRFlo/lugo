package lsp

import (
	"strings"

	"github.com/coalaura/lugo/ast"
)

// buildFiveMTrustBoundaryDiagnostics performs a deliberately conservative check
// at server network event handlers. Handler arguments are external input until
// a recognizable type/shape check has occurred; this is a warning, not a proof
// of safety or authorization.
func (s *Server) buildFiveMTrustBoundaryDiagnostics(doc *Document) {
	if doc == nil || s.getDocumentFiveMProfile(doc).Env() != EnvServer {
		return
	}
	source := string(doc.Source())
	for _, event := range doc.FiveMEvents {
		if event.Kind != FiveMEventRegisterNet && event.Kind != FiveMEventAddHandler {
			continue
		}
		handler := event.HandlerID
		if handler == ast.InvalidNode || int(handler) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[handler].Kind != ast.KindFunctionExpr {
			continue
		}
		params := fiveMHandlerParameterNames(doc, handler)
		if len(params) == 0 {
			continue
		}

		// A handler that has no source/permission guard is worth reporting even
		// when its arguments are not passed to a known sink. Keep both messages
		// low-severity so projects can adopt the check incrementally.
		h := doc.Tree.Nodes[handler]
		handlersrc := source[h.Start:h.End]
		if !fiveMHasSourceCheck(handlersrc) {
			s.diagBuf = append(s.diagBuf, Diagnostic{Range: getNodeRange(doc.Tree, handler), Severity: SeverityWarning, Code: "fivem-event-missing-source-check", Message: "Server event handler does not check source before acting on client input."})
		}
		if !fiveMHasAuthorizationCheck(handlersrc) {
			s.diagBuf = append(s.diagBuf, Diagnostic{Range: getNodeRange(doc.Tree, handler), Severity: SeverityWarning, Code: "fivem-event-missing-authorization", Message: "Server event handler has no recognizable authorization check."})
		}

		for _, param := range params {
			for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
				node := doc.Tree.Nodes[id]
				if node.Kind != ast.KindIdent || node.Start < h.Start || node.End > h.End || node.End <= node.Start || string(doc.Source()[node.Start:node.End]) != param || nearestFunction(doc.Tree, id) != handler {
					continue
				}
				call := enclosingFiveMCall(doc.Tree, id)
				if call == ast.InvalidNode || !fiveMSensitiveEventSink(doc, call) {
					continue
				}
				prefix := source[h.Start:node.Start]
				if !fiveMParameterValidated(prefix, param) {
					s.diagBuf = append(s.diagBuf, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-event-untrusted-parameter", Message: "Untrusted server event parameter is used before validation."})
				}
			}
		}
	}
}

func fiveMHandlerParameterNames(doc *Document, handler ast.NodeID) []string {
	n := doc.Tree.Nodes[handler]
	out := make([]string, 0, n.Count)
	for i := uint32(0); i < n.Count && n.Extra+i < uint32(len(doc.Tree.ExtraList)); i++ {
		id := doc.Tree.ExtraList[n.Extra+i]
		if int(id) >= len(doc.Tree.Nodes) {
			continue
		}
		p := doc.Tree.Nodes[id]
		if p.Kind == ast.KindIdent && p.End <= uint32(len(doc.Source())) {
			out = append(out, string(doc.Source()[p.Start:p.End]))
		}
	}
	return out
}

func enclosingFiveMCall(tree *ast.Tree, id ast.NodeID) ast.NodeID {
	for id != ast.InvalidNode && int(id) < len(tree.Nodes) {
		if tree.Nodes[id].Kind == ast.KindCallExpr || tree.Nodes[id].Kind == ast.KindMethodCall {
			return id
		}
		id = tree.Nodes[id].Parent
	}
	return ast.InvalidNode
}

func fiveMSensitiveEventSink(doc *Document, id ast.NodeID) bool {
	n := doc.Tree.Nodes[id]
	if n.Left == ast.InvalidNode || int(n.Left) >= len(doc.Tree.Nodes) {
		return false
	}
	left := doc.Tree.Nodes[n.Left]
	if left.End > uint32(len(doc.Source())) {
		return false
	}
	name := string(doc.Source()[left.Start:left.End])
	if n.Kind == ast.KindMethodCall && n.Right != ast.InvalidNode && int(n.Right) < len(doc.Tree.Nodes) {
		right := doc.Tree.Nodes[n.Right]
		name += "." + string(doc.Source()[right.Start:right.End])
	}
	for _, sink := range []string{"TriggerClientEvent", "TriggerServerEvent", "ExecuteCommand", "MySQL", "exports", "os.execute", "PerformHttpRequest"} {
		if name == sink || strings.HasPrefix(name, sink+".") || strings.HasPrefix(name, sink+":") {
			return true
		}
	}
	return false
}

func fiveMParameterValidated(prefix, param string) bool {
	for _, marker := range []string{"type(" + param + ")", "assert(" + param, "tonumber(" + param + ")", "tostring(" + param + ")"} {
		if strings.Contains(prefix, marker) {
			return true
		}
	}
	return false
}

func fiveMHasSourceCheck(src string) bool {
	return strings.Contains(src, "source") && (strings.Contains(src, "if ") || strings.Contains(src, "IsPlayer"))
}

func fiveMHasAuthorizationCheck(src string) bool {
	for _, marker := range []string{"IsPlayerAceAllowed", "HasPermission", "hasPermission", "IsPlayerAdmin", "IsPrincipalAceAllowed", "permission"} {
		if strings.Contains(src, marker) {
			return true
		}
	}
	return false
}
