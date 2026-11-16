package lsp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestRefreshReclassifiesUnchangedDocuments(t *testing.T) {
	for _, evicted := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "evicted"}[evicted], func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("lib/mod.lua", "return { field = 1 }")
			h.server.refreshWorkspace()
			uri := h.server.pathToURI(filepath.Join(h.root, "lib", "mod.lua"))
			doc := h.server.Documents[uri]
			if doc == nil || doc.IsLibrary || !doc.IsWorkspace || doc.ModuleName != "lib.mod" {
				t.Fatalf("initial classification = %#v", doc)
			}
			if evicted {
				evictClosedDocumentCaches(h.server)
			} else {
				h.server.OpenFiles[uri] = true
			}
			h.rpcOut.Reset()
			h.server.setLibraryPaths([]string{filepath.Join(h.root, "lib")})
			h.server.refreshWorkspace()
			if !doc.IsLibrary || doc.IsWorkspace || doc.ModuleName != "mod" {
				t.Fatalf("library classification: library=%v workspace=%v module=%q", doc.IsLibrary, doc.IsWorkspace, doc.ModuleName)
			}
			if got := h.server.resolveModule(uri, "mod"); got != doc {
				t.Fatalf("module lookup = %v, want reclassified document", got)
			}
			foundNew, foundOld := false, false
			for _, entries := range h.server.GlobalIndex.HashIndex {
				for _, entry := range entries {
					if entry.URI == uri {
						foundNew = foundNew || entry.Name == "mod.field"
						foundOld = foundOld || entry.Name == "lib.mod.field"
					}
				}
			}
			if !foundNew || foundOld {
				t.Fatalf("module symbols: new=%v old=%v", foundNew, foundOld)
			}
			if doc.Tree.Source == nil {
				t.Fatal("reclassified document source was not restored")
			}
			if evicted && !strings.Contains(h.rpcOut.String(), `"diagnostics":[]`) {
				t.Fatal("closed document's workspace diagnostics were not cleared")
			}
			h.server.setLibraryPaths(nil)
			h.server.refreshWorkspace()
			if doc.IsLibrary || !doc.IsWorkspace || doc.ModuleName != "lib.mod" {
				t.Fatalf("restored classification: library=%v workspace=%v module=%q", doc.IsLibrary, doc.IsWorkspace, doc.ModuleName)
			}
		})
	}
}

func TestRefreshRebindsRequireFieldsWhenModuleRootChanges(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("a/mod.lua", "return { field = 1 }")
	h.writeWorkspaceFile("b/mod.lua", "return { field = 2 }")
	h.writeWorkspaceFile("b/main.lua", "local m = require('mod')\nprint(m.field)")
	h.server.refreshWorkspace()
	mainURI := h.server.pathToURI(filepath.Join(h.root, "b", "main.lua"))
	aURI := h.server.pathToURI(filepath.Join(h.root, "a", "mod.lua"))
	bURI := h.server.pathToURI(filepath.Join(h.root, "b", "mod.lua"))
	doc := h.server.Documents[mainURI]
	getHash := func() uint64 {
		for _, field := range doc.Resolver.PendingFields {
			if field.PropHash == ast.HashBytes([]byte("field")) {
				return field.ReceiverHash
			}
		}
		t.Fatal("missing field reference")
		return 0
	}
	// Re-finalize once all documents are available: their initial processing order
	// is not deterministic.
	h.server.finalizeDocumentUpdate(mainURI, doc.Source(), doc.Tree, doc.Errors, doc)
	if got, want := getHash(), ast.HashBytesConcat([]byte("module:"), nil, []byte(bURI)); got != want {
		t.Fatalf("initial receiver hash = %d, want %d", got, want)
	}
	h.server.setLibraryPaths([]string{filepath.Join(h.root, "a")})
	h.server.refreshWorkspace()
	if got, want := getHash(), ast.HashBytesConcat([]byte("module:"), nil, []byte(aURI)); got != want {
		t.Fatalf("receiver hash after root change = %d, want %d", got, want)
	}
}

func TestRefreshBindsRequireWhenTargetAppearsAfterConsumer(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	mainURI := h.server.pathToURI(filepath.Join(h.root, "main.lua"))
	targetURI := h.server.pathToURI(filepath.Join(h.root, "mod.lua"))
	h.server.OpenFiles[mainURI] = true
	h.server.updateDocument(mainURI, []byte("local m = require('mod')\nprint(m.field)"))
	h.writeWorkspaceFile("mod.lua", "return { field = 1 }")
	h.server.refreshWorkspace()
	doc := h.server.Documents[mainURI]
	want := ast.HashBytesConcat([]byte("module:"), nil, []byte(targetURI))
	for _, field := range doc.Resolver.PendingFields {
		if field.PropHash == ast.HashBytes([]byte("field")) {
			if field.ReceiverHash != want {
				t.Fatalf("receiver hash = %d, want %d", field.ReceiverHash, want)
			}
			return
		}
	}
	t.Fatal("missing field reference")
}
