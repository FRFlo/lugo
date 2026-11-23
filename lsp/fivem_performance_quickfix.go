package lsp

import (
	"fmt"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMPerformanceQuickFixes only edits an explicit Wait(0) argument. It does
// not add yields to loops whose control flow has not been proven safe.
func fiveMPerformanceQuickFixes(doc *Document, uri string, diag Diagnostic) []CodeAction {
	if doc == nil || doc.Tree == nil || diag.Code != "fivem-performance-wait-zero" {
		return nil
	}
	loopID := ast.InvalidNode
	offset := doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character)
	for id := doc.Tree.NodeAt(offset); id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes); id = doc.Tree.Nodes[id].Parent {
		node := doc.Tree.Nodes[id]
		if getNodeRange(doc.Tree, id) == diag.Range && (node.Kind == ast.KindWhile || node.Kind == ast.KindRepeat || node.Kind == ast.KindForNum || node.Kind == ast.KindForIn) {
			loopID = id
			break
		}
	}
	if loopID == ast.InvalidNode {
		return nil
	}
	loop := doc.Tree.Nodes[loopID]
	facts := newFiveMASTFacts(doc)
	var actions []CodeAction
	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		node := doc.Tree.Nodes[id]
		if (node.Kind != ast.KindCallExpr && node.Kind != ast.KindMethodCall) || node.Start < loop.Start || node.End > loop.End {
			continue
		}
		name := facts.callNames[id]
		if name != "Wait" && !(len(name) > len(".Wait") && strings.HasSuffix(name, ".Wait")) {
			continue
		}
		// A Wait inside a nested function does not yield the containing loop.
		if facts.nearestFunction[id] != facts.nearestFunction[loopID] {
			continue
		}
		arg := callArgument(doc, node, 0)
		if arg == ast.InvalidNode || int(arg) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[arg].Kind != ast.KindNumber {
			continue
		}
		argNode := doc.Tree.Nodes[arg]
		if argNode.Start > argNode.End || uint64(argNode.End) > uint64(len(doc.Source())) || string(doc.Source()[argNode.Start:argNode.End]) != "0" {
			continue
		}
		for _, delay := range []string{"100", "1000"} {
			actions = append(actions, CodeAction{
				Title:       fmt.Sprintf("Set Wait delay to %s ms", delay),
				Kind:        "quickfix",
				Diagnostics: []Diagnostic{diag},
				Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{
					uri: {{Range: getNodeRange(doc.Tree, arg), NewText: delay}},
				}},
			})
		}
	}
	return actions
}
