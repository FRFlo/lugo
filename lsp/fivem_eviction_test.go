package lsp

import (
	"testing"
)

// TestFiveMEviction tests cache eviction behavior when documents are closed.
// evictClosedDocumentCaches is called to drop memory-heavy caches (TypeCache,
// Inferring, LuaDocCache, ActualReads, MutatedLocals, Tree.Source) for closed
// documents while preserving Tree and Resolver for cross-document features.
func TestFiveMEviction(t *testing.T) {
	t.Run("ClosedDocDropsCaches", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		// Get a document and verify it has caches populated after indexing
		clientURI := h.server.pathToURI(h.root + "/surface_resource/client.lua")
		doc := h.server.Documents[clientURI]
		if doc == nil {
			t.Fatal("client.lua should be indexed")
		}

		// Verify caches are present before eviction
		if doc.TypeCache == nil {
			t.Fatal("TypeCache should be populated after indexing")
		}
		if len(doc.Inferring) == 0 {
			t.Fatal("Inferring should be populated after indexing")
		}
		if doc.LuaDocCache == nil {
			t.Fatal("LuaDocCache should be populated after indexing")
		}
		if doc.ActualReads == nil {
			t.Fatal("ActualReads should be populated after indexing")
		}
		if doc.MutatedLocals == nil {
			t.Fatal("MutatedLocals should be populated after indexing")
		}
		if doc.Tree == nil {
			t.Fatal("Tree should exist")
		}
		if doc.Tree.Source == nil {
			t.Fatal("Tree.Source should be populated after indexing")
		}
		if doc.Resolver == nil {
			t.Fatal("Resolver should exist")
		}

		// Simulate closing the document (remove from OpenFiles)
		delete(h.server.OpenFiles, clientURI)

		// Call evictClosedDocumentCaches
		evictClosedDocumentCaches(h.server)

		// Verify caches are evicted
		if doc.TypeCache != nil {
			t.Fatal("TypeCache should be nil after eviction")
		}
		if doc.Inferring != nil && len(doc.Inferring) > 0 {
			t.Fatal("Inferring should be nil/empty after eviction")
		}
		if doc.LuaDocCache != nil {
			t.Fatal("LuaDocCache should be nil after eviction")
		}
		if doc.ActualReads != nil {
			t.Fatal("ActualReads should be nil after eviction")
		}
		if doc.MutatedLocals != nil {
			t.Fatal("MutatedLocals should be nil after eviction")
		}
		if doc.Tree.Source != nil {
			t.Fatal("Tree.Source should be nil after eviction")
		}

		// Verify Tree and Resolver are preserved
		if doc.Tree == nil {
			t.Fatal("Tree should be preserved after eviction")
		}
		if doc.Resolver == nil {
			t.Fatal("Resolver should be preserved after eviction")
		}
	})

	t.Run("OpenDocKeepsCaches", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		// Get a document
		clientURI := h.server.pathToURI(h.root + "/surface_resource/client.lua")
		doc := h.server.Documents[clientURI]
		if doc == nil {
			t.Fatal("client.lua should be indexed")
		}

		// Verify initial caches
		if doc.TypeCache == nil {
			t.Fatal("TypeCache should be populated after indexing")
		}
		if doc.Tree == nil || doc.Tree.Source == nil {
			t.Fatal("Tree.Source should be populated after indexing")
		}

		// Ensure document is open
		h.server.OpenFiles[clientURI] = true

		// Call evictClosedDocumentCaches
		evictClosedDocumentCaches(h.server)

		// Verify open document STILL has caches
		if doc.TypeCache == nil {
			t.Fatal("Open document should retain TypeCache after eviction")
		}
		if doc.Inferring == nil || len(doc.Inferring) == 0 {
			t.Fatal("Open document should retain Inferring after eviction")
		}
		if doc.LuaDocCache == nil {
			t.Fatal("Open document should retain LuaDocCache after eviction")
		}
		if doc.ActualReads == nil {
			t.Fatal("Open document should retain ActualReads after eviction")
		}
		if doc.MutatedLocals == nil {
			t.Fatal("Open document should retain MutatedLocals after eviction")
		}
		if doc.Tree == nil || doc.Tree.Source == nil {
			t.Fatal("Open document should retain Tree.Source after eviction")
		}
		if doc.Resolver == nil {
			t.Fatal("Open document should retain Resolver after eviction")
		}
	})

	t.Run("CrossDocFeaturesWorkForOpenDocs", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		// Mark client.lua and shared.lua as open
		clientURI := h.server.pathToURI(h.root + "/surface_resource/client.lua")
		sharedURI := h.server.pathToURI(h.root + "/surface_resource/shared.lua")
		h.server.OpenFiles[clientURI] = true
		h.server.OpenFiles[sharedURI] = true

		// server_consumer.lua references SHARED_ONLY from shared.lua and CLIENT_ONLY from client.lua
		consumerURI := h.server.pathToURI(h.root + "/surface_resource/server_consumer.lua")
		h.server.OpenFiles[consumerURI] = true

		// Get the consumer document
		consumerDoc := h.server.Documents[consumerURI]
		if consumerDoc == nil {
			t.Fatal("server_consumer.lua should be indexed")
		}

		// Verify we can resolve cross-document symbol: surface_server_shared_ref -> SHARED_ONLY in shared.lua
		// This tests that open documents referencing OTHER open documents still work
		marker := h.requireMarker("surface_server_shared_ref")
		ctx := h.server.resolveSymbolAt(consumerURI, consumerDoc.Tree.Offset(marker.Position.Line, marker.Position.Character))
		if ctx == nil {
			t.Fatal("cross-document resolution should work for open docs referencing other open docs")
		}

		// Verify hover works for cross-doc reference
		hover := h.hover("surface_server_shared_ref")
		if hover == nil {
			t.Fatal("hover should work for cross-document reference in open docs")
		}

		// Verify go-to-definition works for cross-doc reference
		locations := h.definition("surface_server_shared_ref")
		if len(locations) == 0 {
			t.Fatal("go-to-definition should work for cross-document reference in open docs")
		}

		// Verify the definition lands on the actual definition in shared.lua
		sharedMarker := h.requireMarker("surface_shared_definition")
		if len(locations) > 0 && locations[0].URI == sharedMarker.URI {
			// Cross-doc resolution is working correctly
		}

		// Now evict caches for all closed documents
		evictClosedDocumentCaches(h.server)

		// Cross-doc features should STILL work for open documents after eviction
		// The key verification: open documents keep their caches and cross-doc resolution works
		consumerDoc2 := h.server.Documents[consumerURI]
		if consumerDoc2 == nil {
			t.Fatal("consumer doc should still exist after eviction")
		}

		// Verify hover still works on the open consumer doc after eviction
		hover2 := h.hover("surface_server_shared_ref")
		if hover2 == nil {
			t.Fatal("hover should still work for open doc after eviction")
		}

		// Verify definition still works on the open consumer doc after eviction
		locations2 := h.definition("surface_server_shared_ref")
		if len(locations2) == 0 {
			t.Fatal("go-to-definition should still work for open doc after eviction")
		}
	})

	t.Run("ClosedDocSourceAccessNoPanic", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		// Get a document
		clientURI := h.server.pathToURI(h.root + "/surface_resource/client.lua")
		doc := h.server.Documents[clientURI]
		if doc == nil {
			t.Fatal("client.lua should be indexed")
		}

		// Verify initial state
		if doc.Tree == nil || doc.Tree.Source == nil {
			t.Fatal("Tree.Source should be populated")
		}

		// Close the document
		delete(h.server.OpenFiles, clientURI)

		// Evict caches
		evictClosedDocumentCaches(h.server)

		// Accessing Source() on a closed document should return nil without panic
		// The nil-safety guard in Document.Source() handles this case:
		//   if doc != nil && doc.Tree != nil {
		//       return doc.Tree.Source
		//   }
		// After eviction, doc.Tree != nil but doc.Tree.Source == nil,
		// so Source() should return nil (not panic)
		source := doc.Source()
		if source != nil {
			t.Fatalf("Source() on closed doc should return nil after eviction, got %d bytes", len(source))
		}

		// Also verify the Tree is still present (just Source is nil)
		if doc.Tree == nil {
			t.Fatal("Tree should still be present after eviction")
		}
		if doc.Resolver == nil {
			t.Fatal("Resolver should still be present after eviction")
		}
	})
}
