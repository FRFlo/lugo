package lsp

import (
	"path/filepath"
	"testing"
)

func TestParity_Hover(t *testing.T) {
	h := newFiveMFixtureHarness(t, "plain_lua")
	hover := h.hover("plain_source")
	if hover == nil || hover.Contents.Value == "" {
		t.Fatal("hover parity check failed: expected non-empty hover")
	}

	compat := WrapCompatDocument(h.docForMarker("plain_source"))
	if compat.Document() == nil {
		t.Fatal("compat document should expose the underlying document")
	}
}

func TestParity_Completion(t *testing.T) {
	h := newFiveMFixtureHarness(t, "plain_lua")
	h.writeWorkspaceFile("parity_completion.lua", `local completion_value = { alpha = 1, beta = 2 }
return completion_value.--[[@parity_completion]]
`)
	h.reindex()

	completion := h.completion("parity_completion")
	if !completionHasLabel(completion, "alpha") {
		t.Fatalf("completion parity check failed: expected alpha, got %#v", completion.Items)
	}

	if !completionHasLabel(completion, "beta") {
		t.Fatalf("completion parity check failed: expected beta, got %#v", completion.Items)
	}
}

func TestParity_GotoDefinition(t *testing.T) {
	h := newFiveMFixtureHarness(t, "plain_lua")
	h.writeWorkspaceFile("parity_definition.lua", `local function target()
	return 1
end

return --[[@parity_definition]]target()
`)
	h.reindex()

	defs := h.definition("parity_definition")
	if len(defs) == 0 {
		t.Fatal("goto definition parity check failed: expected at least one location")
	}

	wantURI := h.server.pathToURI(filepath.Join(h.root, "parity_definition.lua"))
	if defs[0].URI != wantURI {
		t.Fatalf("goto definition parity check failed: got %s, want %s", defs[0].URI, wantURI)
	}
}

func TestParity_FindReferences(t *testing.T) {
	h := newFiveMFixtureHarness(t, "plain_lua")
	h.writeWorkspaceFile("parity_references.lua", `local function target()
	return 1
end

return target() + --[[@parity_references]]target()
`)
	h.reindex()

	refs := h.references("parity_references", true)
	if len(refs) < 3 {
		t.Fatalf("find references parity check failed: got %d locations, want at least 3", len(refs))
	}
}

func TestParity_Diagnostics(t *testing.T) {
	h := newFiveMFixtureHarness(t, "plain_lua")
	h.server.DiagUndefinedGlobals = true
	h.writeWorkspaceFile("parity_diagnostics.lua", `return --[[@parity_diagnostics]]MissingGlobal
`)
	h.reindex()

	diags := h.diagnostics("parity_diagnostics.lua")
	if !hasDiagnosticCode(diags, "undefined-global") {
		t.Fatalf("diagnostics parity check failed: expected undefined-global, got %#v", diags)
	}
}

func BenchmarkParity_GlobalIndex_Legacy(b *testing.B) {
	idx := make(LegacyGlobalIndex)
	key := GlobalKey{ReceiverHash: 1, PropHash: 2}
	idx[key] = []GlobalSymbol{{URI: "file:///bench.lua", Name: "alpha"}}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = idx[key]
	}
}

func BenchmarkParity_GlobalIndex_Compat(b *testing.B) {
	idx := NewCompatGlobalIndex()
	key := GlobalKey{ReceiverHash: 1, PropHash: 2}
	idx.SetLegacy(key, []GlobalSymbol{{URI: "file:///bench.lua", Name: "alpha"}})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = idx.Lookup("", key)
	}
}

func BenchmarkParity_Document_Legacy(b *testing.B) {
	doc := &Document{TypeCache: []TypeSet{{}}}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = doc.TypeCache
	}
}

func BenchmarkParity_Document_Compat(b *testing.B) {
	compat := WrapCompatDocument(&Document{TypeCache: []TypeSet{{}}})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = compat.TypeCacheView()
	}
}
