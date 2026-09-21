package lsp

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// buildFiveMStateBagDiagnostics indexes the literal keys used by state bags in
// this document. It deliberately ignores computed keys: guessing those keys
// creates considerably more noise than useful information.
func (s *Server) buildFiveMStateBagDiagnostics(doc *Document) []Diagnostic {
	if doc == nil || doc.Tree == nil {
		return nil
	}

	var writes = make(map[string]ast.NodeID)
	var writeNames = make(map[string]string)
	var reads []stateBagKeyUse
	var handlers []stateBagKeyUse

	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		n := doc.Tree.Nodes[id]
		switch n.Kind {
		case ast.KindMemberExpr:
			if !s.stateBagMember(doc, id) {
				continue
			}
			keyID := n.Right
			key := stateBagNodeText(doc, keyID)
			if key == "" {
				continue
			}
			if isStateBagAssignment(doc, id) {
				canonical := strings.ToLower(key)
				if _, ok := writes[canonical]; !ok {
					writes[canonical] = keyID
					writeNames[canonical] = key
				}
			} else {
				reads = append(reads, stateBagKeyUse{key: key, node: keyID})
			}
		case ast.KindCallExpr, ast.KindMethodCall:
			name, receiver := stateBagCall(doc, id)
			if name == "set" || name == "get" {
				keyID := callArgument(doc, n, 0)
				key := stateBagNodeText(doc, keyID)
				if key == "" {
					continue
				}
				if name == "set" {
					canonical := strings.ToLower(key)
					if _, ok := writes[canonical]; !ok {
						writes[canonical] = keyID
						writeNames[canonical] = key
					}
				} else if receiver {
					reads = append(reads, stateBagKeyUse{key: key, node: keyID})
				}
			}
			if stateBagCallName(doc, id) == "AddStateBagChangeHandler" {
				keyID := callArgument(doc, n, 0)
				if key := stateBagNodeText(doc, keyID); key != "" {
					handlers = append(handlers, stateBagKeyUse{key: key, node: keyID})
				}
			}
		}
	}

	var diags []Diagnostic
	for _, use := range reads {
		canonical := strings.ToLower(use.key)
		if _, ok := writes[canonical]; !ok {
			diags = append(diags, Diagnostic{Range: getNodeRange(doc.Tree, use.node), Severity: SeverityWarning, Code: "fivem-state-bag-missing-key", Message: fmt.Sprintf("State bag key '%s' is read but never written.", use.key)})
		} else if writeNames[canonical] != use.key {
			diags = append(diags, Diagnostic{Range: getNodeRange(doc.Tree, use.node), Severity: SeverityWarning, Code: "fivem-state-bag-inconsistent-key", Message: fmt.Sprintf("State bag key '%s' differs from the indexed key '%s'.", use.key, writeNames[canonical])})
		}
	}
	for _, use := range handlers {
		if _, ok := writes[strings.ToLower(use.key)]; !ok {
			diags = append(diags, Diagnostic{Range: getNodeRange(doc.Tree, use.node), Severity: SeverityWarning, Code: "fivem-state-bag-unknown-key", Message: fmt.Sprintf("State bag change handler references unknown key '%s'.", use.key)})
		}
	}
	return diags
}

type stateBagKeyUse struct {
	key  string
	node ast.NodeID
}

func stateBagNodeText(doc *Document, id ast.NodeID) string {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return ""
	}
	n := doc.Tree.Nodes[id]
	if n.Kind == ast.KindIdent {
		return string(doc.Source()[n.Start:n.End])
	}
	if n.Kind != ast.KindString {
		return ""
	}
	return unquoteLuaString(string(doc.Source()[n.Start:n.End]))
}

func callArgument(doc *Document, n ast.Node, index uint32) ast.NodeID {
	if index >= n.Count || n.Extra+index >= uint32(len(doc.Tree.ExtraList)) {
		return ast.InvalidNode
	}
	return doc.Tree.ExtraList[n.Extra+index]
}

func stateBagCallName(doc *Document, id ast.NodeID) string {
	n := doc.Tree.Nodes[id]
	if n.Kind != ast.KindCallExpr || n.Left == ast.InvalidNode {
		return ""
	}
	left := doc.Tree.Nodes[n.Left]
	if left.Kind != ast.KindIdent {
		return ""
	}
	return string(doc.Source()[left.Start:left.End])
}

func stateBagCall(doc *Document, id ast.NodeID) (string, bool) {
	n := doc.Tree.Nodes[id]
	if n.Kind != ast.KindMethodCall || n.Right == ast.InvalidNode {
		return "", false
	}
	name := string(doc.Source()[doc.Tree.Nodes[n.Right].Start:doc.Tree.Nodes[n.Right].End])
	return name, sStateBagReceiver(doc, n.Left) || (stateBagField(doc, n.Left) && sStateBagReceiver(doc, doc.Tree.Nodes[n.Left].Left))
}

func (s *Server) stateBagMember(doc *Document, id ast.NodeID) bool {
	n := doc.Tree.Nodes[id]
	if n.Kind != ast.KindMemberExpr || n.Left == ast.InvalidNode {
		return false
	}
	left := doc.Tree.Nodes[n.Left]
	return sStateBagReceiver(doc, n.Left) || (left.Kind == ast.KindMemberExpr && stateBagField(doc, n.Left) && sStateBagReceiver(doc, left.Left))
}

func stateBagField(doc *Document, id ast.NodeID) bool {
	n := doc.Tree.Nodes[id]
	return n.Kind == ast.KindMemberExpr && n.Right != ast.InvalidNode && bytes.Equal(doc.Source()[doc.Tree.Nodes[n.Right].Start:doc.Tree.Nodes[n.Right].End], []byte("state"))
}

func sStateBagReceiver(doc *Document, id ast.NodeID) bool {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return false
	}
	n := doc.Tree.Nodes[id]
	if n.Kind == ast.KindIdent {
		name := doc.Source()[n.Start:n.End]
		return bytes.Equal(name, []byte("GlobalState")) || bytes.Equal(name, []byte("LocalPlayer"))
	}
	if n.Kind != ast.KindCallExpr || n.Left == ast.InvalidNode {
		return false
	}
	left := doc.Tree.Nodes[n.Left]
	if left.Kind != ast.KindIdent {
		return false
	}
	name := doc.Source()[left.Start:left.End]
	return bytes.Equal(name, []byte("Entity")) || bytes.Equal(name, []byte("Player"))
}

func isStateBagAssignment(doc *Document, id ast.NodeID) bool {
	n := doc.Tree.Nodes[id]
	if n.Parent == ast.InvalidNode || int(n.Parent) >= len(doc.Tree.Nodes) {
		return false
	}
	p := doc.Tree.Nodes[n.Parent]
	return p.Kind == ast.KindAssign && p.Left == id
}
