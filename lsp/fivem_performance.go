package lsp

import (
	"fmt"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// buildFiveMPerformanceDiagnostics reports only syntactic, high-signal hotspots.
// These are intentionally low-confidence warnings: runtime load and native
// behavior cannot be established from Lua source alone.
func (s *Server) buildFiveMPerformanceDiagnostics(doc *Document) []Diagnostic {
	if doc == nil || doc.Tree == nil {
		return nil
	}
	facts := newFiveMASTFacts(doc)
	var out []Diagnostic
	registers := 0
	firstRegister := ast.InvalidNode
	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		n := doc.Tree.Nodes[id]
		if n.Kind == ast.KindCallExpr || n.Kind == ast.KindMethodCall {
			name := facts.callNames[id]
			if name == "RegisterNetEvent" {
				registers++
				if firstRegister == ast.InvalidNode {
					firstRegister = id
				}
			}
			if performanceSQLCall(name) {
				out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-performance-sync-sql", Message: "Low-confidence performance warning: synchronous SQL-like work may block the FiveM thread."})
			}
		}
		loop, ok := facts.loops[id]
		if !ok {
			continue
		}
		if loop.waitZero {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-performance-wait-zero", Message: "Low-confidence performance warning: this tight loop calls Wait(0); verify that per-frame work is necessary."})
		} else if !loop.yield && performanceUnboundedLoop(doc, id) {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-performance-no-yield", Message: "Low-confidence performance warning: this loop has no obvious yield and may block the FiveM thread."})
		}
		if loop.lookup {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, id), Severity: SeverityWarning, Code: "fivem-performance-repeated-lookup", Message: "Low-confidence performance warning: repeated native/entity lookup inside a loop may be expensive; consider caching or reducing its frequency."})
		}
	}
	if registers > 10 {
		out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, firstRegister), Severity: SeverityWarning, Code: "fivem-performance-many-handlers", Message: fmt.Sprintf("Low-confidence performance warning: %d RegisterNetEvent handlers are declared in this file; review handler count and event work.", registers)})
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
