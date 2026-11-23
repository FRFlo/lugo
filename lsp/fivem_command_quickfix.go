package lsp

import (
	"fmt"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMCommandQuickFixes provides explicit declaration alternatives when a
// literal command is known. Other command diagnostics need choices (ACE
// principal or which mapping to retain) that this protocol surface cannot ask
// for safely, so they intentionally produce no action.
func (s *Server) fiveMCommandQuickFixes(doc *Document, uri string, diag Diagnostic) []CodeAction {
	if s == nil || doc == nil || doc.Tree == nil || diag.Code != "fivem-command-missing-declaration" {
		return nil
	}
	name := fiveMCommandNameAtDiagnostic(doc, diag)
	if !safeFiveMCommandName(name) {
		return nil
	}
	source := doc.Source()
	if source == nil {
		return nil
	}
	insertAt := uint32(len(source))
	if uint64(insertAt) > uint64(len(source)) {
		return nil
	}
	prefix := "\n"
	if len(source) == 0 || source[len(source)-1] == '\n' {
		prefix = ""
	}
	quotedName := strings.ReplaceAll(strings.ReplaceAll(name, "\\", "\\\\"), "'", "\\'")
	body := prefix + "RegisterCommand('" + quotedName + "', function(source, args, raw)\n\t-- TODO: implement command behavior\nend, %s)\n"
	rng := getRange(doc.Tree, insertAt, insertAt)
	makeAction := func(restricted bool, label string) CodeAction {
		value := "false"
		if restricted {
			value = "true"
		}
		return CodeAction{
			Title: "Declare '" + name + "' as " + label,
			Kind:  "quickfix", Diagnostics: []Diagnostic{diag},
			Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{
				uri: {{Range: rng, NewText: fmt.Sprintf(body, value)}},
			}},
		}
	}
	return []CodeAction{makeAction(true, "restricted (ACE permission required)"), makeAction(false, "unrestricted")}
}

func fiveMCommandNameAtDiagnostic(doc *Document, diag Diagnostic) string {
	if doc == nil || doc.Tree == nil {
		return ""
	}
	offset := doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character)
	id := doc.Tree.NodeAt(offset)
	var literal ast.NodeID = ast.InvalidNode
	var enclosingCall ast.NodeID = ast.InvalidNode
	for id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes) {
		if literal == ast.InvalidNode {
			if _, ok := fiveMLiteralString(doc, id); ok {
				literal = id
			}
		}
		if doc.Tree.Nodes[id].Kind == ast.KindCallExpr {
			enclosingCall = id
			break
		}
		id = doc.Tree.Nodes[id].Parent
	}
	if literal == ast.InvalidNode {
		return ""
	}
	name, ok := fiveMLiteralString(doc, literal)
	if !ok {
		return ""
	}
	if enclosingCall != ast.InvalidNode {
		callName, _ := fiveMCall(doc, enclosingCall)
		if callName == "ExecuteCommand" {
			fields := strings.Fields(name)
			if len(fields) == 0 {
				return ""
			}
			// Match the diagnostic producer: ExecuteCommand strings are split
			// on whitespace and only the first token is the command name.
			name = fields[0]
		}
	}
	return name
}

func safeFiveMCommandName(name string) bool {
	if strings.TrimSpace(name) != name || name == "" || strings.ContainsAny(name, "\r\n\x00") {
		return false
	}
	return true
}
