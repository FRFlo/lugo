package lsp

import (
	"fmt"

	"github.com/coalaura/lugo/ast"
)

// buildFiveMEntityLifecycleDiagnostics performs deliberately conservative checks
// for entities obtained from a literal network id. Dynamic ids and control-flow
// that cannot be established from source order are ignored.
func (s *Server) buildFiveMEntityLifecycleDiagnostics(doc *Document) []Diagnostic {
	if doc == nil || doc.Tree == nil {
		return nil
	}
	type entityState struct {
		name     string
		acquire  ast.NodeID
		exists   bool
		cleaned  bool
		firstUse ast.NodeID
	}
	states := make(map[string]*entityState)
	var diags []Diagnostic

	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		n := doc.Tree.Nodes[id]
		if n.Kind != ast.KindCallExpr || n.Left == ast.InvalidNode {
			continue
		}
		name := lifecycleCallName(doc, n.Left)
		if name == "" {
			continue
		}
		if name == "NetworkGetEntityFromNetworkId" {
			arg := callArgument(doc, n, 0)
			if arg == ast.InvalidNode || doc.Tree.Nodes[arg].Kind != ast.KindNumber {
				continue
			}
			if variable := lifecycleAssignedName(doc, id); variable != "" {
				states[variable] = &entityState{name: variable, acquire: id}
			}
			continue
		}
		if name == "DoesEntityExist" {
			if argName := lifecycleEntityArgument(doc, n); argName != "" {
				if state := states[argName]; state != nil {
					state.exists = true
				}
			}
			continue
		}
		if name != "NetworkRequestControlOfEntity" && name != "DeleteEntity" {
			continue
		}
		argName := lifecycleEntityArgument(doc, n)
		state := states[argName]
		if state == nil {
			continue
		}
		if name == "DeleteEntity" {
			state.cleaned = true
			continue
		}
		if !state.exists {
			if state.firstUse == ast.InvalidNode {
				state.firstUse = id
				diags = append(diags, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-entity-use-before-existence-check", Message: fmt.Sprintf("Entity '%s' is used before checking DoesEntityExist.", state.name)})
			}
		}
	}
	for _, state := range states {
		if !state.cleaned {
			diags = append(diags, Diagnostic{Range: getNodeRange(doc.Tree, state.acquire), Severity: SeverityWarning, Code: "fivem-entity-missing-cleanup", Message: fmt.Sprintf("Entity '%s' acquired from a network id is not cleaned up with DeleteEntity.", state.name)})
		}
	}
	return diags
}

func lifecycleCallName(doc *Document, id ast.NodeID) string {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ""
	}
	n := doc.Tree.Nodes[id]
	if n.Kind != ast.KindIdent {
		return ""
	}
	return string(doc.Source()[n.Start:n.End])
}

func lifecycleEntityArgument(doc *Document, n ast.Node) string {
	arg := callArgument(doc, n, 0)
	if arg == ast.InvalidNode || int(arg) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[arg].Kind != ast.KindIdent {
		return ""
	}
	node := doc.Tree.Nodes[arg]
	return string(doc.Source()[node.Start:node.End])
}

func lifecycleAssignedName(doc *Document, id ast.NodeID) string {
	for id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes) {
		n := doc.Tree.Nodes[id]
		if n.Kind == ast.KindAssign || n.Kind == ast.KindLocalAssign {
			return lifecycleFirstIdent(doc, n.Left)
		}
		id = n.Parent
	}
	return ""
}

func lifecycleFirstIdent(doc *Document, id ast.NodeID) string {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ""
	}
	n := doc.Tree.Nodes[id]
	if n.Kind == ast.KindIdent {
		return string(doc.Source()[n.Start:n.End])
	}
	for i := ast.NodeID(1); int(i) < len(doc.Tree.Nodes); i++ {
		candidate := doc.Tree.Nodes[i]
		if candidate.Parent == id && candidate.Kind == ast.KindIdent {
			return string(doc.Source()[candidate.Start:candidate.End])
		}
	}
	return ""
}
