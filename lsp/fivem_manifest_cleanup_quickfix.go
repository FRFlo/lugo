package lsp

import (
	"bytes"
	"github.com/FRFlo/lugo/ast"
)

// manifestCleanupQuickFix removes a redundant scalar directive or one literal
// list entry when its separator can be removed without touching comments.
func (s *Server) manifestCleanupQuickFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if doc == nil || doc.Tree == nil || !doc.IsFiveMManifest {
		return CodeAction{}, false
	}
	var fresh []Diagnostic
	switch diag.Code {
	case "fivem-self-dependency":
		fresh = s.buildFiveMResourceGraphDiagnostics(doc)
	case "fivem-duplicate-export":
		fresh = s.buildFiveMResourceExportContractDiagnostics(doc)
	default:
		return CodeAction{}, false
	}
	current := false
	for _, d := range fresh {
		if d.Code == diag.Code && d.Range == diag.Range && d.Message == diag.Message {
			current = true
			break
		}
	}
	if !current {
		return CodeAction{}, false
	}
	res := s.parseFiveMManifest(doc)
	if res == nil || res.Manifest == nil {
		return CodeAction{}, false
	}
	var matchingName, matchingValue string
	for _, entry := range res.Manifest.Entries {
		if entry.ValueRange == diag.Range && !entry.LoaderInjected && !entry.ReservedKey {
			matchingName, matchingValue = entry.RawName, entry.Value
			break
		}
	}
	if matchingName == "" || matchingValue == "" {
		return CodeAction{}, false
	}
	if fix, ok := s.manifestListEntryCleanupQuickFix(doc, uri, diag, res.Manifest, matchingName, matchingValue); ok {
		return fix, true
	}
	if diag.Code == "fivem-duplicate-export" {
		count := 0
		for _, entry := range res.Manifest.Entries {
			if !entry.LoaderInjected && !entry.ReservedKey && entry.RawName == matchingName && entry.Value == matchingValue {
				count++
			}
		}
		if count < 2 {
			return CodeAction{}, false
		}
	}
	id := doc.Tree.NodeAt(doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character))
	for id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes) && doc.Tree.Nodes[id].Kind != ast.KindCallExpr {
		id = doc.Tree.Nodes[id].Parent
	}
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		// Manifest table-call shorthand (dependencies { ... }) can have no
		// parent edge from the table values to the directive call.
		for candidate := ast.NodeID(1); int(candidate) < len(doc.Tree.Nodes); candidate++ {
			n := doc.Tree.Nodes[candidate]
			if n.Kind != ast.KindCallExpr || n.Left == ast.InvalidNode || int(n.Left) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[n.Left].Kind != ast.KindIdent {
				continue
			}
			if string(doc.Source()[doc.Tree.Nodes[n.Left].Start:doc.Tree.Nodes[n.Left].End]) != matchingName {
				continue
			}
			arg := callArgument(doc, n, 0)
			if arg != ast.InvalidNode && int(arg) < len(doc.Tree.Nodes) && doc.Tree.Nodes[arg].Kind == ast.KindTableExpr {
				for i := uint32(0); i < doc.Tree.Nodes[arg].Count; i++ {
					if getNodeRange(doc.Tree, doc.Tree.ExtraList[doc.Tree.Nodes[arg].Extra+i]) == diag.Range {
						id = candidate
						break
					}
				}
			}
			if id == candidate {
				break
			}
		}
	}
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return CodeAction{}, false
	}
	call := doc.Tree.Nodes[id]
	if call.Count != 1 || call.Left == ast.InvalidNode || int(call.Left) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[call.Left].Kind != ast.KindIdent {
		return CodeAction{}, false
	}
	arg := callArgument(doc, call, 0)
	if arg == ast.InvalidNode || int(arg) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[arg].Kind != ast.KindString || getNodeRange(doc.Tree, arg) != diag.Range {
		return CodeAction{}, false
	}
	if bytes.Contains(doc.Source()[call.Start:doc.Tree.Nodes[arg].Start], []byte("--")) || bytes.Contains(doc.Source()[doc.Tree.Nodes[arg].End:call.End], []byte("--")) {
		return CodeAction{}, false
	}
	if string(doc.Source()[doc.Tree.Nodes[call.Left].Start:doc.Tree.Nodes[call.Left].End]) != matchingName {
		return CodeAction{}, false
	}
	return CodeAction{Title: "Remove redundant manifest directive", Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: getNodeRange(doc.Tree, id), NewText: ""}}}}}, true
}

// manifestListEntryCleanupQuickFix removes one literal element and its adjacent
// comma. It refuses spans containing comments, so every comment remains byte-for-byte.
func (s *Server) manifestListEntryCleanupQuickFix(doc *Document, uri string, diag Diagnostic, manifest *FiveMManifest, name, value string) (CodeAction, bool) {
	var entry *FiveMManifestEntry
	for _, candidate := range manifest.Entries {
		if candidate.RawName == name && candidate.Value == value && candidate.ValueRange == diag.Range && candidate.Shape == FiveMManifestEntryTableValue {
			copy := candidate
			if entry != nil {
				return CodeAction{}, false
			}
			entry = &copy
		}
	}
	if entry == nil {
		return CodeAction{}, false
	}
	src := doc.Source()
	start := doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character)
	end := doc.Tree.Offset(diag.Range.End.Line, diag.Range.End.Character)
	lineStart := doc.Tree.Offset(entry.Range.Start.Line, entry.Range.Start.Character)
	lineEnd := doc.Tree.Offset(entry.Range.End.Line, entry.Range.End.Character)
	if end > uint32(len(src)) || lineEnd > uint32(len(src)) || lineStart > start || end > lineEnd {
		return CodeAction{}, false
	}
	openRel := bytes.IndexByte(src[lineStart:start], '{')
	closeRel := bytes.IndexByte(src[end:lineEnd], '}')
	if openRel < 0 || closeRel < 0 {
		return CodeAction{}, false
	}
	open, close := lineStart+uint32(openRel), end+uint32(closeRel)
	if open >= start || close < end {
		return CodeAction{}, false
	}
	// Prefer consuming the following comma; for the final item consume the preceding comma.
	if end > start && end <= uint32(len(src)) {
		if p := bytes.IndexByte(src[end:close], ','); p >= 0 {
			end += uint32(p + 1)
		} else {
			if p := bytes.LastIndexByte(src[open:start], ','); p >= 0 {
				start = open + uint32(p)
			} else {
				// A one-element list has no separator; delete only the literal.
			}
		}
	}
	if end > uint32(len(src)) || start > end || bytes.Contains(src[start:end], []byte("--")) {
		return CodeAction{}, false
	}
	sl, sc := doc.Tree.Position(start)
	el, ec := doc.Tree.Position(end)
	rng := Range{Start: Position{Line: sl, Character: sc}, End: Position{Line: el, Character: ec}}
	return CodeAction{Title: "Remove redundant manifest list entry", Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: rng, NewText: ""}}}}}, true
}
