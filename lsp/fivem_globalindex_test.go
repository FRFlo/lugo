package lsp

import (
	"testing"
)

// TestFiveMGlobalIndexCompaction tests that GlobalIndex compaction correctly removes
// empty entries after document removal while preserving non-empty entries.
func TestFiveMGlobalIndexCompaction(t *testing.T) {
	t.Run("EmptyKeysDeletedAfterRemoval", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		// Get the shared.lua document which defines SHARED_ONLY
		sharedURI := h.server.pathToURI(h.root + "/surface_resource/shared.lua")
		doc := h.server.Documents[sharedURI]
		if doc == nil {
			t.Fatal("shared.lua should be in Documents")
		}

		// Collect GlobalIndex keys before removal
		var keyToCheck GlobalKey
		var found bool
		for _, exp := range doc.ExportedGlobalDefs {
			keyToCheck = exp.Key
			found = true
			break
		}
		if !found {
			t.Fatal("shared.lua should export global defs")
		}

		// Verify the key exists in GlobalIndex
		if _, ok := h.server.GlobalIndex[keyToCheck]; !ok {
			t.Fatal("GlobalIndex should contain key for SHARED_ONLY before removal")
		}

		// Remove the document
		h.server.clearDocument(sharedURI)

		// Verify the GlobalIndex key is completely gone (not just empty)
		if _, ok := h.server.GlobalIndex[keyToCheck]; ok {
			t.Fatal("GlobalIndex key should be deleted after document removal, not just empty")
		}
	})

	t.Run("NonEmptyKeysPreserved", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		// Write two files that define the same global symbol
		h.writeWorkspaceFile("surface_resource/shared.lua", `
			MY_GLOBAL = "first"
		`)
		h.writeWorkspaceFile("surface_resource/aux.lua", `
			MY_GLOBAL = "second"
		`)

		// Reindex to pick up the new files
		h.reindex()

		sharedURI := h.server.pathToURI(h.root + "/surface_resource/shared.lua")
		auxURI := h.server.pathToURI(h.root + "/surface_resource/aux.lua")

		sharedDoc := h.server.Documents[sharedURI]
		auxDoc := h.server.Documents[auxURI]
		if sharedDoc == nil || auxDoc == nil {
			t.Fatal("both documents should be indexed")
		}

		// Find the key for MY_GLOBAL
		var keyToCheck GlobalKey
		var found bool
		for _, exp := range sharedDoc.ExportedGlobalDefs {
			keyToCheck = exp.Key
			found = true
			break
		}
		if !found {
			t.Fatal("shared.lua should export MY_GLOBAL")
		}

		// Verify the key exists and has 2 symbols (from both documents)
		syms, ok := h.server.GlobalIndex[keyToCheck]
		if !ok {
			t.Fatal("GlobalIndex should contain MY_GLOBAL key")
		}
		if len(syms) != 2 {
			t.Fatalf("MY_GLOBAL should have 2 symbols in GlobalIndex, got %d", len(syms))
		}

		// Remove aux.lua (one of the two defining documents)
		h.server.clearDocument(auxURI)

		// Verify the key still exists with the remaining symbol
		syms, ok = h.server.GlobalIndex[keyToCheck]
		if !ok {
			t.Fatal("GlobalIndex key should still exist after removing one contributor")
		}
		if len(syms) != 1 {
			t.Fatalf("MY_GLOBAL should have 1 symbol after removal, got %d", len(syms))
		}

		// Verify the remaining symbol is from shared.lua, not aux.lua
		if syms[0].URI != sharedURI {
			t.Fatalf("remaining symbol should be from shared.lua, got URI %s", syms[0].URI)
		}
	})

	t.Run("SizeReducedAfterRemoval", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		// Write a file with multiple unique global symbols
		h.writeWorkspaceFile("surface_resource/multidef.lua", `
			GLOBAL_A = 1
			GLOBAL_B = 2
			GLOBAL_C = 3
		`)

		// Reindex to pick up the new file
		h.reindex()

		multiURI := h.server.pathToURI(h.root + "/surface_resource/multidef.lua")
		multiDoc := h.server.Documents[multiURI]
		if multiDoc == nil {
			t.Fatal("multidef.lua should be indexed")
		}

		// Count how many global keys this document contributes
		initialGlobalCount := len(multiDoc.ExportedGlobalDefs)
		if initialGlobalCount < 3 {
			t.Fatalf("multidef.lua should export at least 3 globals, got %d", initialGlobalCount)
		}

		// Record GlobalIndex size before removal
		sizeBefore := len(h.server.GlobalIndex)

		// Remove the document
		h.server.clearDocument(multiURI)

		// Verify GlobalIndex size is reduced
		sizeAfter := len(h.server.GlobalIndex)
		if sizeAfter >= sizeBefore {
			t.Fatalf("GlobalIndex size should be reduced after removal: before=%d, after=%d", sizeBefore, sizeAfter)
		}

		// The key for each defined symbol should be gone from GlobalIndex
		for _, exp := range multiDoc.ExportedGlobalDefs {
			if _, ok := h.server.GlobalIndex[exp.Key]; ok {
				t.Fatal("GlobalIndex key should be deleted after document removal")
			}
		}
	})
}
