package lsp

import (
	"github.com/FRFlo/lugo/ast"
)

// oalVectorQuickFix returns an edit only when the diagnosed argument is a
// resolved local vector3 and the native has one unambiguous numeric signature.
// The code action router in refactor.go should call this for the diagnostic
// code fivem-oal-vector-argument.
func (s *Server) oalVectorQuickFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if doc == nil || doc.Tree == nil || doc.Resolver == nil || diag.Code != "fivem-oal-vector-argument" {
		return CodeAction{}, false
	}
	// Revalidate so stale diagnostics never produce edits.
	current := false
	for _, d := range s.buildFiveMOALVectorDiagnostics(doc) {
		if d.Code == diag.Code && d.Range == diag.Range && d.Message == diag.Message {
			current = true
			break
		}
	}
	if !current {
		return CodeAction{}, false
	}
	tree, src := doc.Tree, doc.Source()
	valid := func(id ast.NodeID) bool { return id != ast.InvalidNode && int(id) < len(tree.Nodes) }
	for callID := ast.NodeID(1); int(callID) < len(tree.Nodes); callID++ {
		call := tree.Nodes[callID]
		if call.Kind != ast.KindCallExpr || call.Count == 0 || !valid(call.Left) {
			continue
		}
		functionID := call.Left
		if fn := tree.Nodes[functionID]; fn.Kind == ast.KindMemberExpr {
			functionID = fn.Right
		}
		ctx := s.resolveSymbolNode(doc.URI, doc, functionID)
		if ctx == nil {
			continue
		}
		defs := ctx.GlobalDefs
		if len(defs) == 0 && ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
			defs = []GlobalSymbol{{URI: ctx.TargetDoc.URI, NodeID: ctx.TargetDefID}}
		}
		var native *LuaDoc
		for _, def := range defs {
			target := s.Documents[def.URI]
			meta, ok := fiveMNativeMetadataFor(target, def.NodeID)
			if !ok || meta.Doc == nil {
				continue
			}
			if native != nil {
				return CodeAction{}, false
			}
			native = meta.Doc
		}
		if native == nil {
			continue
		}
		for argIndex := uint32(0); argIndex < call.Count; argIndex++ {
			argID := tree.ExtraList[call.Extra+argIndex]
			if !valid(argID) || getNodeRange(tree, argID) != diag.Range || argIndex+3 > uint32(len(native.Params)) {
				continue
			}
			arg := tree.Nodes[argID]
			if arg.Kind != ast.KindIdent || int(argID) >= len(doc.Resolver.References) || doc.Resolver.References[argID] == ast.InvalidNode {
				return CodeAction{}, false
			}
			defID := doc.Resolver.References[argID]
			isLocal := false
			for _, localDef := range doc.Resolver.LocalDefs {
				if localDef == defID {
					isLocal = true
					break
				}
			}
			if !isLocal || fiveMVectorDimension(doc.InferType(argID)) != 3 {
				return CodeAction{}, false
			}
			for i := 0; i < 3; i++ {
				if !fiveMNativeNumericParam(native.Params[int(argIndex)+i].Type) {
					return CodeAction{}, false
				}
			}
			name := string(src[arg.Start:arg.End])
			return CodeAction{Title: "Pass vector3 components explicitly", Kind: "quickfix", Diagnostics: []Diagnostic{diag}, IsPreferred: true, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: diag.Range, NewText: name + ".x, " + name + ".y, " + name + ".z"}}}}}, true
		}
	}
	return CodeAction{}, false
}
