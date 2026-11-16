package lsp

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceLuaDocEnrichmentUpdatesMatchingClassCandidate(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_scripts { 'a.lua', 'b.lua' }\n")
	h.writeWorkspaceFile("resource/a.lua", "---@class Duplicate<A>\n---@field fromA A\nlocal Duplicate = {}\n")
	h.writeWorkspaceFile("resource/b.lua", "---@class Duplicate<B>\n---@field fromB B\nlocal Duplicate = {}\n")
	h.reindex()

	resource := ResourceURI(h.server.pathToURI(filepath.Join(h.root, "resource")))
	candidates := h.server.GlobalIndex.LookupCandidatesByScope(resource, GlobalIndexScopeShared, "Duplicate")
	if len(candidates) != 2 {
		t.Fatalf("Duplicate candidates = %d, want 2: %+v", len(candidates), candidates)
	}
	wantParams := map[string]string{
		h.server.pathToURI(filepath.Join(h.root, "resource", "a.lua")): "A",
		h.server.pathToURI(filepath.Join(h.root, "resource", "b.lua")): "B",
	}
	for _, candidate := range candidates {
		if candidate == nil {
			t.Fatal("nil class candidate")
		}
		want, ok := wantParams[candidate.URI]
		if !ok {
			t.Fatalf("unexpected candidate URI %q", candidate.URI)
		}
		if candidate.LuaDoc == nil || candidate.LuaDoc.GenericClass == nil || len(candidate.LuaDoc.GenericClass.Params) != 1 || candidate.LuaDoc.GenericClass.Params[0].Name != want {
			t.Errorf("candidate %s generic data = %+v, want %q", candidate.URI, candidate.LuaDoc, want)
		}
		if candidate.URI == h.server.pathToURI(filepath.Join(h.root, "resource", "a.lua")) {
			if field := h.server.GlobalIndex.LookupCandidatesByScope(resource, GlobalIndexScopeShared, "Duplicate.fromA"); len(field) != 1 || field[0].URI != candidate.URI {
				t.Errorf("fromA field candidates = %+v, want only source %s", field, candidate.URI)
			}
		}
	}
}
