package lsp

import (
	"fmt"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// stateBagKeyQuickFix suggests a spelling only when every indexed write of the
// canonical key uses the same spelling. It is a suggestion: the index is
// limited to a provably identical global state bag receiver.
func (s *Server) stateBagKeyQuickFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if doc == nil || doc.Tree == nil || (diag.Code != "fivem-state-bag-inconsistent-key" && diag.Code != "fivem-state-bag-missing-key") {
		return CodeAction{}, false
	}
	// Do not trust a diagnostic from an older document version.
	current := false
	for _, d := range s.buildFiveMStateBagDiagnostics(doc) {
		if d.Code == diag.Code && d.Range == diag.Range && d.Message == diag.Message {
			current = true
			break
		}
	}
	if !current {
		return CodeAction{}, false
	}
	id := doc.Tree.NodeAt(doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character))
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) || getNodeRange(doc.Tree, id) != diag.Range {
		return CodeAction{}, false
	}
	key := stateBagNodeText(doc, id)
	if key == "" {
		return CodeAction{}, false
	}
	readParent := doc.Tree.Nodes[id].Parent
	if readParent == ast.InvalidNode || int(readParent) >= len(doc.Tree.Nodes) {
		return CodeAction{}, false
	}
	readNode := doc.Tree.Nodes[readParent]
	if readNode.Kind != ast.KindMemberExpr && readNode.Kind != ast.KindMethodCall {
		return CodeAction{}, false
	}
	bag := stableStateBagReceiver(doc, readNode.Left)
	if bag == "" {
		return CodeAction{}, false
	}
	var spelling string
	for i := ast.NodeID(1); int(i) < len(doc.Tree.Nodes); i++ {
		n := doc.Tree.Nodes[i]
		var keyID ast.NodeID
		switch n.Kind {
		case ast.KindMemberExpr:
			if s.stateBagMember(doc, i) && isStateBagAssignment(doc, i) {
				keyID = n.Right
			}
		case ast.KindMethodCall:
			name, _ := stateBagCall(doc, i)
			if name == "set" {
				keyID = callArgument(doc, n, 0)
			}
		}
		if keyID == ast.InvalidNode || stableStateBagReceiver(doc, n.Left) != bag {
			continue
		}
		name := stateBagNodeText(doc, keyID)
		if diag.Code == "fivem-state-bag-inconsistent-key" {
			if !strings.EqualFold(name, key) {
				continue
			}
		} else if levenshteinFast(strings.ToLower(name), strings.ToLower(key), 2) > 2 {
			continue
		}
		if spelling != "" && spelling != name {
			return CodeAction{}, false
		}
		spelling = name
	}
	if spelling == "" || spelling == key || !stateBagSimpleIdentifier(spelling) {
		return CodeAction{}, false
	}
	n := doc.Tree.Nodes[id]
	replacement := spelling
	if n.Kind == ast.KindString {
		raw := doc.Source()[n.Start:n.End]
		if len(raw) < 2 || (raw[0] != '\'' && raw[0] != '"') || raw[len(raw)-1] != raw[0] {
			return CodeAction{}, false
		}
		replacement = string(raw[0]) + spelling + string(raw[0])
	} else if n.Kind != ast.KindIdent {
		return CodeAction{}, false
	}
	title := "Change state bag key to '%s'"
	if diag.Code == "fivem-state-bag-missing-key" {
		title = "Change state bag key to '%s' (suggestion)"
	}
	return CodeAction{Title: fmt.Sprintf(title, spelling), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: diag.Range, NewText: replacement}}}}}, true
}

func stateBagSimpleIdentifier(name string) bool {
	switch name {
	case "and", "break", "do", "else", "elseif", "end", "false", "for", "function", "goto", "if", "in", "local", "nil", "not", "or", "repeat", "return", "then", "true", "until", "while":
		return false
	}
	for i, c := range name {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return name != ""
}

// Only the two globally identified bags are stable; call receivers such as
// Entity(id) can designate different entities between the write and read.
func stableStateBagReceiver(doc *Document, id ast.NodeID) string {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) || doc.Resolver == nil {
		return ""
	}
	n := doc.Tree.Nodes[id]
	base := id
	if n.Kind == ast.KindMemberExpr {
		if n.Right == ast.InvalidNode || int(n.Right) >= len(doc.Tree.Nodes) || stateBagNodeText(doc, n.Right) != "state" {
			return ""
		}
		base = n.Left
	}
	if base == ast.InvalidNode || int(base) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[base].Kind != ast.KindIdent || int(base) >= len(doc.Resolver.References) || doc.Resolver.References[base] != ast.InvalidNode {
		return ""
	}
	name := stateBagNodeText(doc, base)
	if n.Kind == ast.KindIdent && name == "GlobalState" {
		return name
	}
	if n.Kind == ast.KindMemberExpr && name == "LocalPlayer" {
		return "LocalPlayer.state"
	}
	return ""
}
