package lsp

import (
	"fmt"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// manifestDirectiveQuickFix only corrects casing of a known directive. Custom
// directives and runtime API calls are intentionally not guessed.
func (s *Server) manifestDirectiveQuickFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if doc == nil || doc.Tree == nil || !doc.IsFiveMManifest || diag.Code != "fivem-manifest-unknown-directive" {
		return CodeAction{}, false
	}
	current := false
	for _, d := range s.buildFiveMManifestDiagnostics(doc) {
		if d.Code == diag.Code && d.Range == diag.Range && d.Message == diag.Message {
			current = true
			break
		}
	}
	if !current {
		return CodeAction{}, false
	}
	id := doc.Tree.NodeAt(doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character))
	for id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes) && getNodeRange(doc.Tree, id) != diag.Range {
		id = doc.Tree.Nodes[id].Parent
	}
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return CodeAction{}, false
	}
	call := doc.Tree.Nodes[id]
	if call.Kind != ast.KindCallExpr || call.Left == ast.InvalidNode || int(call.Left) >= len(doc.Tree.Nodes) {
		return CodeAction{}, false
	}
	callee := doc.Tree.Nodes[call.Left]
	if callee.Kind != ast.KindIdent {
		return CodeAction{}, false
	}
	name := string(doc.Source()[callee.Start:callee.End])
	var match string
	for _, candidate := range []string{"fx_version", "game", "games", "client_script", "client_scripts", "server_script", "server_scripts", "shared_script", "shared_scripts", "file", "files", "dependency", "dependencies", "export", "exports", "server_export", "server_exports", "ui_page", "loadscreen", "data_file", "this_is_a_map", "lua54", "use_experimental_fxv2_oal", "provide", "author", "description", "version"} {
		if strings.EqualFold(name, candidate) && name != candidate {
			if match != "" {
				return CodeAction{}, false
			}
			match = candidate
		}
	}
	if match == "" {
		return CodeAction{}, false
	}
	return CodeAction{Title: fmt.Sprintf("Change manifest directive to '%s'", match), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: getNodeRange(doc.Tree, call.Left), NewText: match}}}}}, true
}
