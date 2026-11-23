package lsp

import (
	"bytes"
	"strings"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/lexer"
	"github.com/FRFlo/lugo/token"
)

// luaDiagnosticQuickFix only uses the current syntax tree and exact diagnostic
// ranges. These actions are intentional suggestions, not source.fixAll edits.
func luaDiagnosticQuickFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if doc == nil || doc.Tree == nil || len(doc.Errors) != 0 {
		return CodeAction{}, false
	}
	tree, src := doc.Tree, doc.Source()
	makeAction := func(title string, start, end uint32, replacement string) (CodeAction, bool) {
		if start > end || end > uint32(len(src)) {
			return CodeAction{}, false
		}
		return CodeAction{Title: title, Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: getRange(tree, start, end), NewText: replacement}}}}}, true
	}
	valid := func(id ast.NodeID) bool { return id != ast.InvalidNode && int(id) < len(tree.Nodes) }
	matches := func(id ast.NodeID) bool { return valid(id) && getNodeRange(tree, id) == diag.Range }
	switch diag.Code {
	case "incorrect-vararg":
		for i := 1; i < len(tree.Nodes); i++ {
			n := tree.Nodes[i]
			if n.Kind != ast.KindVararg || !matches(ast.NodeID(i)) {
				continue
			}
			for parent := n.Parent; valid(parent); parent = tree.Nodes[parent].Parent {
				fn := tree.Nodes[parent]
				if fn.Kind == ast.KindFile {
					break
				}
				if fn.Kind != ast.KindFunctionExpr {
					continue
				}
				if n.Parent == parent {
					break
				} // a parameter, not a vararg expression
				if fn.Count > 0 {
					last := tree.ExtraList[fn.Extra+fn.Count-1]
					if !valid(last) || tree.Nodes[last].Kind == ast.KindVararg {
						break
					}
					return makeAction("Add vararg parameter", tree.Nodes[last].End, tree.Nodes[last].End, ", ...")
				}
				// Lex the function header, so comments and method names cannot be
				// mistaken for the opening parenthesis.
				lex := lexer.New(src[fn.Start:fn.End])
				for tok := lex.Next(); tok.Kind != token.EOF; tok = lex.Next() {
					if tok.Kind == token.LParen {
						pos := fn.Start + tok.End
						return makeAction("Add vararg parameter", pos, pos, "...")
					}
				}
				break
			}
		}
	case "unbalanced-assignment":
		for i := 1; i < len(tree.Nodes); i++ {
			n := tree.Nodes[i]
			if (n.Kind != ast.KindAssign && n.Kind != ast.KindLocalAssign) || !valid(n.Left) || !valid(n.Right) {
				continue
			}
			lhs, rhs := tree.Nodes[n.Left], tree.Nodes[n.Right]
			if rhs.Count == 0 || lhs.Count <= rhs.Count {
				continue
			}
			first := tree.ExtraList[lhs.Extra+rhs.Count]
			last := tree.ExtraList[lhs.Extra+lhs.Count-1]
			if !valid(first) || !valid(last) || getRange(tree, tree.Nodes[first].Start, tree.Nodes[last].End) != diag.Range {
				continue
			}
			lastRHS := tree.ExtraList[rhs.Extra+rhs.Count-1]
			if !valid(lastRHS) {
				continue
			}
			value := tree.Nodes[lastRHS]
			if value.Kind == ast.KindCallExpr || value.Kind == ast.KindMethodCall || value.Kind == ast.KindVararg {
				continue
			}
			return makeAction("Make implicit nil values explicit", value.End, value.End, strings.Repeat(", nil", int(lhs.Count-rhs.Count)))
		}
	case "constant-condition", "unreachable-branch":
		for i := 1; i < len(tree.Nodes); i++ {
			n := tree.Nodes[i]
			if n.Kind != ast.KindIf || !valid(n.Left) || !valid(n.Right) || luaQuickFixHasComments(tree, n.Start, n.End) {
				continue
			}
			literal := func(id ast.NodeID) (bool, bool) {
				if !valid(id) {
					return false, false
				}
				switch tree.Nodes[id].Kind {
				case ast.KindTrue:
					return true, true
				case ast.KindFalse, ast.KindNil:
					return false, true
				}
				return false, false
			}
			truth, ok := literal(n.Left)
			if !ok {
				continue
			}
			matched := matches(n.Left) || (!truth && matches(n.Right))
			selected := ast.InvalidNode
			if truth {
				selected = n.Right
			}
			for j := uint32(0); j < n.Count; j++ {
				childID := tree.ExtraList[n.Extra+j]
				if !valid(childID) {
					ok = false
					break
				}
				child := tree.Nodes[childID]
				if selected != ast.InvalidNode && matches(childID) {
					matched = true
				}
				switch child.Kind {
				case ast.KindElseIf:
					v, lit := literal(child.Left)
					if !lit || !valid(child.Right) {
						ok = false
						break
					}
					if selected == ast.InvalidNode && (matches(child.Left) || (!v && matches(child.Right))) {
						matched = true
					}
					if selected == ast.InvalidNode && v {
						selected = child.Right
					}
				case ast.KindElse:
					if !valid(child.Left) {
						ok = false
						break
					}
					if selected == ast.InvalidNode {
						selected = child.Left
					}
				default:
					ok = false
				}
			}
			if !ok || !matched {
				continue
			}
			body := ""
			if valid(selected) {
				b := tree.Nodes[selected]
				body = string(src[b.Start:b.End])
			}
			return makeAction("Simplify constant if statement", n.Start, n.End, "do\n"+body+"\nend")
		}
	case "duplicate-field":
		for i := 1; i < len(tree.Nodes); i++ {
			table := tree.Nodes[i]
			if table.Kind != ast.KindTableExpr || luaQuickFixHasComments(tree, table.Start, table.End) {
				continue
			}
			for j := uint32(1); j < table.Count; j++ {
				currentID := tree.ExtraList[table.Extra+j]
				if !valid(currentID) {
					continue
				}
				current := tree.Nodes[currentID]
				if current.Kind != ast.KindRecordField || !matches(current.Left) || !valid(current.Right) {
					continue
				}
				cv := tree.Nodes[current.Right]
				if !luaQuickFixLiteral(cv.Kind) {
					continue
				}
				for k := uint32(0); k < j; k++ {
					previousID := tree.ExtraList[table.Extra+k]
					nextID := tree.ExtraList[table.Extra+k+1]
					if !valid(previousID) || !valid(nextID) {
						continue
					}
					prev := tree.Nodes[previousID]
					if prev.Kind != ast.KindRecordField || !valid(prev.Left) || !valid(prev.Right) {
						continue
					}
					pk, ck, pv := tree.Nodes[prev.Left], tree.Nodes[current.Left], tree.Nodes[prev.Right]
					if pk.Kind != ast.KindIdent || ck.Kind != ast.KindIdent || pv.Kind != cv.Kind || !bytes.Equal(src[pk.Start:pk.End], src[ck.Start:ck.End]) || !bytes.Equal(src[pv.Start:pv.End], src[cv.Start:cv.End]) {
						continue
					}
					next := tree.Nodes[nextID]
					if prev.End > next.Start || !luaQuickFixFieldSeparator(src[prev.End:next.Start]) {
						continue
					}
					return makeAction("Remove identical duplicate field", prev.Start, next.Start, "")
				}
			}
		}
	}
	return CodeAction{}, false
}

func luaQuickFixLiteral(kind ast.NodeKind) bool {
	switch kind {
	case ast.KindNil, ast.KindTrue, ast.KindFalse, ast.KindNumber, ast.KindString:
		return true
	}
	return false
}

func luaQuickFixHasComments(tree *ast.Tree, start, end uint32) bool {
	for _, c := range tree.Comments {
		if c.Start < end && c.End > start {
			return true
		}
	}
	return false
}

func luaQuickFixFieldSeparator(b []byte) bool {
	seen := false
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n':
		case ',', ';':
			if seen {
				return false
			}
			seen = true
		default:
			return false
		}
	}
	return seen
}
