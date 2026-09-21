package lsp

import (
	"fmt"
	"strings"

	"github.com/coalaura/lugo/ast"
)

// buildFiveMPerformanceDiagnostics reports only syntactic, high-signal hotspots.
// These are intentionally low-confidence warnings: runtime load and native
// behavior cannot be established from Lua source alone.
func (s *Server) buildFiveMPerformanceDiagnostics(doc *Document) []Diagnostic {
	if doc == nil || doc.Tree == nil {
		return nil
	}
	var out []Diagnostic
	registers := 0
	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		n := doc.Tree.Nodes[id]
		if n.Kind == ast.KindCallExpr || n.Kind == ast.KindMethodCall {
			name := performanceNodeCallName(doc, id)
			if name == "RegisterNetEvent" {
				registers++
			}
			if performanceSQLCall(name) {
				out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-performance-sync-sql", Message: "Low-confidence performance warning: synchronous SQL-like work may block the FiveM thread."})
			}
		}
		if n.Kind != ast.KindWhile && n.Kind != ast.KindRepeat && n.Kind != ast.KindForNum && n.Kind != ast.KindForIn {
			continue
		}
		waitZero, yield := false, false
		lookup := false
		for child := ast.NodeID(1); int(child) < len(doc.Tree.Nodes); child++ {
			if child == id || !performanceInside(doc.Tree, child, id) {
				continue
			}
			cn := doc.Tree.Nodes[child]
			if cn.Kind != ast.KindCallExpr && cn.Kind != ast.KindMethodCall {
				continue
			}
			name := performanceNodeCallName(doc, child)
			switch {
			case name == "Wait" || strings.HasSuffix(name, ".Wait"):
				arg := callArgument(doc, cn, 0)
				if arg != ast.InvalidNode && doc.Tree.Nodes[arg].Kind == ast.KindNumber && string(doc.Source()[doc.Tree.Nodes[arg].Start:doc.Tree.Nodes[arg].End]) == "0" {
					waitZero = true
				}
				yield = true
			case name == "coroutine.yield" || strings.HasSuffix(name, ".yield"):
				yield = true
			case performanceLookupCall(name):
				lookup = true
			}
		}
		if waitZero {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-performance-wait-zero", Message: "Low-confidence performance warning: this tight loop calls Wait(0); verify that per-frame work is necessary."})
		} else if !yield && performanceUnboundedLoop(doc, id) {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-performance-no-yield", Message: "Low-confidence performance warning: this loop has no obvious yield and may block the FiveM thread."})
		}
		if lookup {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-performance-repeated-lookup", Message: "Low-confidence performance warning: repeated native/entity lookup inside a loop may be expensive; consider caching or reducing its frequency."})
		}
	}
	if registers > 10 {
		out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, firstPerformanceCall(doc, "RegisterNetEvent")), Severity: SeverityWarning, Code: "fivem-performance-many-handlers", Message: fmt.Sprintf("Low-confidence performance warning: %d RegisterNetEvent handlers are declared in this file; review handler count and event work.", registers)})
	}
	return out
}

func performanceUnboundedLoop(doc *Document, id ast.NodeID) bool {
	n := doc.Tree.Nodes[id]
	if n.Kind == ast.KindRepeat {
		return true
	}
	if n.Kind != ast.KindWhile {
		return false
	}
	src := strings.ToLower(string(doc.Source()[n.Start:n.End]))
	return strings.Contains(src, "while true") || strings.Contains(src, "while(true)")
}

func performanceInside(tree *ast.Tree, child, parent ast.NodeID) bool {
	for child != ast.InvalidNode {
		child = tree.Nodes[child].Parent
		if child == parent {
			return true
		}
	}
	return false
}

func performanceNodeCallName(doc *Document, id ast.NodeID) string {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ""
	}
	n := doc.Tree.Nodes[id]
	name := performanceCallName(doc, n.Left)
	if n.Kind == ast.KindMethodCall && n.Right != ast.InvalidNode {
		right := doc.Tree.Nodes[n.Right]
		name += ":" + string(doc.Source()[right.Start:right.End])
	}
	return name
}

func performanceCallName(doc *Document, id ast.NodeID) string {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ""
	}
	n := doc.Tree.Nodes[id]
	if n.Kind != ast.KindIdent && n.Kind != ast.KindMemberExpr {
		return ""
	}
	return string(doc.Source()[n.Start:n.End])
}

func performanceLookupCall(name string) bool {
	for _, part := range []string{"GetEntity", "GetClosest", "GetGamePool", "FindFirstPed", "FindFirstObject", "NetworkGetEntity", "GetActivePlayers"} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}

func performanceSQLCall(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "sync") && (strings.Contains(lower, "mysql") || strings.Contains(lower, "sql") || strings.Contains(lower, "query") || strings.Contains(lower, "execute"))
}

func firstPerformanceCall(doc *Document, wanted string) ast.NodeID {
	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		n := doc.Tree.Nodes[id]
		if (n.Kind == ast.KindCallExpr || n.Kind == ast.KindMethodCall) && performanceNodeCallName(doc, id) == wanted {
			return id
		}
	}
	return ast.InvalidNode
}
