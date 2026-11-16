package lsp

import (
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestFiveMGlobalReferencesFilterPreservesResults(t *testing.T) {
	for _, tc := range []struct {
		name       string
		marker     string
		wantURIHas string
	}{
		{name: "resource export", marker: "export_scoped_ping_call", wantURIHas: "/export_consumer/"},
		{name: "provided alias", marker: "provide_fallback_ping_call", wantURIHas: "/provide_consumer/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarness(t, "resource_exports", "resource_provides")
			h.writeWorkspaceFile("export_consumer/shadow.lua", `local exports = { export_resource = {} }
return exports.export_resource:ping()`)
			h.reindex()
			withDeclaration := h.references(tc.marker, true)
			withoutDeclaration := h.references(tc.marker, false)
			if len(withDeclaration) != 1 || len(withoutDeclaration) != 1 {
				t.Fatalf("complete reference results with declaration=%+v without=%+v; want exactly the export call", withDeclaration, withoutDeclaration)
			}

			without := make(map[Location]bool, len(withoutDeclaration))
			for _, ref := range withoutDeclaration {
				without[ref] = true
			}
			added := 0
			for _, ref := range withDeclaration {
				if without[ref] {
					delete(without, ref)
					continue
				}
				added++
			}
			if len(without) != 0 {
				t.Errorf("includeDeclaration=false lost references: %+v", without)
			}
			if added > 1 {
				t.Errorf("includeDeclaration=true added %d locations, want at most the declaration", added)
			}
			foundExpectedCall := false
			for _, ref := range withDeclaration {
				if strings.Contains(ref.URI, "/shadow.lua") {
					t.Errorf("shadowed local exports was treated as a FiveM bridge: %+v", ref)
				}
				if strings.Contains(ref.URI, tc.wantURIHas) {
					foundExpectedCall = true
				}
			}
			if !foundExpectedCall {
				t.Errorf("references do not include expected export target %q: %+v", tc.wantURIHas, withDeclaration)
			}
		})
	}
}

func TestFiveMGlobalReferencesSkipInvalidExportCandidate(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_exports")
	marker := h.requireMarker("export_scoped_ping_call")
	doc := h.server.Documents[marker.URI]
	ctx := h.server.resolveSymbolAt(marker.URI, doc.Tree.Offset(marker.Position.Line, marker.Position.Character))
	if ctx == nil || !ctx.IsGlobal || ctx.FiveMExportRes == "" {
		t.Fatalf("marker context = %+v, want a global export context", ctx)
	}

	// This malformed bridge candidate has no usable name or receiver. It must be
	// rejected by the validity/name guards before receiver extraction.
	bridgeDoc := h.server.Documents[marker.URI]
	bridgeDoc.Tree.Nodes = append(bridgeDoc.Tree.Nodes, ast.Node{Kind: ast.KindMemberExpr, Left: ast.InvalidNode, Right: ast.InvalidNode})
	for range h.server.iterateGlobalReferences(ctx) {
	}
}
