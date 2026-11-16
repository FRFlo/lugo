package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvictedManifestLegacyFallbackAndReplacement(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/__resource.lua", "client_script 'legacy.lua'\n")
	h.writeWorkspaceFile("resource/fxmanifest.lua", "client_script 'modern.lua'\n")
	h.writeWorkspaceFile("resource/legacy.lua", "return true\n")
	h.writeWorkspaceFile("resource/modern.lua", "return true\n")
	h.reindex()
	rootURI := h.server.pathToURI(filepath.Join(h.root, "resource"))
	legacyURI := rootURI + "/__resource.lua"
	modernURI := rootURI + "/fxmanifest.lua"
	modern := h.server.Documents[modernURI]
	if modern.FiveMManifestResource != h.server.FiveMResourceGraph.ResourceByRoot(rootURI) {
		t.Fatal("manifest cache must reuse the graph's semantic object")
	}
	evictClosedDocumentCaches(h.server)
	if modern.Source() != nil || h.server.Documents[legacyURI].Source() != nil {
		t.Fatal("both manifest source buffers must be released")
	}
	if err := os.Remove(filepath.Join(h.root, "resource", "fxmanifest.lua")); err != nil {
		t.Fatal(err)
	}
	h.reindex()
	resource := h.server.FiveMResourceGraph.ResourceByRoot(rootURI)
	if resource == nil || resource.ManifestURI != legacyURI || len(resource.ClientGlobs) != 1 || resource.ClientGlobs[0] != "legacy.lua" {
		t.Fatalf("evicted legacy fallback lost metadata: %#v", resource)
	}
	if h.server.Documents[modernURI] != nil {
		t.Fatal("removed modern manifest was retained")
	}
	h.writeWorkspaceFile("resource/__resource.lua", "client_script 'replacement.lua'\n")
	h.reindex()
	replacement := h.server.FiveMResourceGraph.ResourceByRoot(rootURI)
	if replacement == resource || len(replacement.ClientGlobs) != 1 || replacement.ClientGlobs[0] != "replacement.lua" {
		t.Fatalf("changed legacy manifest reused stale metadata: %#v", replacement)
	}
	evictClosedDocumentCaches(h.server)
	h.server.workspaceReadDir = func(string) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	h.reindex()
	if got := h.server.FiveMResourceGraph.ResourceByRoot(rootURI); got != replacement {
		t.Fatal("incomplete scan lost the evicted manifest metadata")
	}
	// A direct update to an empty buffer must also replace the cached model.
	h.server.updateDocument(legacyURI, []byte("\n"))
	if got := h.server.Documents[legacyURI].FiveMManifestResource; got == replacement || len(got.ClientGlobs) != 0 {
		t.Fatalf("empty manifest update kept stale metadata: %#v", got)
	}
}
