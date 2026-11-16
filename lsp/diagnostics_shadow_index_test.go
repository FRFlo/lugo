package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestCheckGlobalShadowingReadsLiveIndex(t *testing.T) {
	s := NewServer("shadow-diagnostics")
	s.GlobalIndex = NewGlobalIndex()
	resource := ResourceURI("file:///resource.lua")
	uri := string(resource)
	key := GlobalKey{PropHash: ast.HashBytes([]byte("customShadowUnique"))}
	doc := &Document{Server: s, URI: uri, Path: "/resource.lua", Tree: &ast.Tree{}}
	s.Documents[uri] = doc

	local := &SymbolEntry{Name: "customShadowUnique", Key: key, URI: uri, NodeID: ast.InvalidNode}
	wrongKey := &SymbolEntry{Name: "other", Key: GlobalKey{PropHash: 42}, URI: uri, NodeID: ast.InvalidNode}
	s.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "shadow", local)
	s.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "other", wrongKey)

	check := func() []Diagnostic {
		t.Helper()
		s.diagBuf = s.diagBuf[:0]
		s.checkGlobalShadowing(uri, []byte("customShadowUnique"), false, Range{}, func(candidate *Document) bool {
			return candidate != nil
		})
		return append([]Diagnostic(nil), s.diagBuf...)
	}
	assertRelated := func(stage, wantURI string) {
		t.Helper()
		diags := check()
		if wantURI == "" {
			if len(diags) != 0 {
				t.Fatalf("%s: diagnostics=%d, want none (diagnostics %#v)", stage, len(diags), diags)
			}
			return
		}
		if len(diags) != 1 {
			t.Fatalf("%s: diagnostics=%d, want 1", stage, len(diags))
		}
		got := ""
		if len(diags[0].RelatedInformation) > 0 {
			got = diags[0].RelatedInformation[0].Location.URI
		}
		if got != wantURI {
			t.Fatalf("%s: related definition URI=%q, want %q (diagnostic %#v)", stage, got, wantURI, diags[0])
		}
	}

	assertRelated("initial visible definition", uri)

	// A fresh colliding key inserted during publication takes precedence by
	// the index's live VisibleSymbols order.
	collision := &SymbolEntry{Name: "customShadowUnique", Key: key, URI: "file:///collision.lua", NodeID: ast.InvalidNode}
	collisionDoc := &Document{Server: s, URI: collision.URI, Path: "/collision.lua", Tree: &ast.Tree{}}
	s.Documents[collision.URI] = collisionDoc
	s.GlobalIndex.AddSymbol(resource, GlobalIndexScopeShared, "collision", collision)
	assertRelated("after colliding insertion", collision.URI)

	// Mutating pointer-backed entry keys must immediately remove the
	// diagnostic, without a stale snapshot.
	local.Key = GlobalKey{PropHash: 43}
	collision.Key = GlobalKey{PropHash: 44}
	assertRelated("after live key mutation", "")
}
