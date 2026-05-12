package lsp

import (
	"testing"

	"github.com/coalaura/lugo/ast"
)

// TestFiveMLazyNativeLoading verifies the lazy loading behavior of FiveM native bundles.
//
// Key behaviors tested:
// 1. Native bundle Documents have fiveMNativeBundleName properly set after loading
// 2. Once loaded, native bundle Documents remain in Server.Documents (warm cache)
// 3. The same Document instance is returned on subsequent accesses (cache persistence)
// 4. Native symbols remain in GlobalIndex after lazy loading
func TestFiveMLazyNativeLoading(t *testing.T) {
	t.Run("NativeBundleHasMetadataAfterLoad", func(t *testing.T) {
		// After a fresh reindex, native bundles should have their fiveMNativeBundleName set
		h := newFiveMFixtureHarness(t, "resource_natives")

		count := countLoadedFiveMNativeBundles(h.server)
		if count != len(fiveMNativeBundleNames) {
			t.Fatalf("native bundles loaded after fresh reindex = %d, want %d", count, len(fiveMNativeBundleNames))
		}

		// Verify that all native bundles have their bundle name properly set
		for name := range fiveMNativeBundleNames {
			uri := h.server.findFiveMNativeBundleURI(name)
			if uri == "" {
				t.Fatalf("native bundle %s not found after fresh reindex", name)
			}
			doc := h.server.Documents[uri]
			if doc == nil {
				t.Fatalf("native bundle document %s not in server.Documents", uri)
			}
			if fiveMNativeBundleNameFromDocument(doc) != name {
				t.Fatalf("native bundle %s has wrong name: got %q", name, fiveMNativeBundleNameFromDocument(doc))
			}
		}
	})

	t.Run("LazyLoadOnHoverPopulatesDocuments", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_natives")

		// Clear native bundles to simulate they haven't been lazily loaded yet
		var nativeURIs []string
		for uri, doc := range h.server.Documents {
			if fiveMNativeBundleNameFromDocument(doc) != "" {
				nativeURIs = append(nativeURIs, uri)
			}
		}
		for _, uri := range nativeURIs {
			delete(h.server.Documents, uri)
		}

		// Perform hover - this triggers ensureFiveMNativeBundleLoaded internally
		hover := h.hover("native_client_call")
		if hover == nil {
			t.Fatal("hover should return non-nil result for native function")
		}

		// After hover, native bundle should be in Documents with proper metadata
		wantURI := requireFiveMNativeBundleURI(t, h.server, "natives_universal.lua")
		doc, ok := h.server.Documents[wantURI]
		if !ok {
			t.Fatalf("native bundle %s not in Documents after hover", wantURI)
		}

		// Verify the native bundle has proper metadata
		if fiveMNativeBundleNameFromDocument(doc) != "natives_universal.lua" {
			t.Fatalf("loaded native bundle has wrong name: got %q", fiveMNativeBundleNameFromDocument(doc))
		}
	})

	t.Run("WarmCachePersistsDocumentInstance", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_natives")

		// First hover loads the native bundle
		hoverFirst := h.hover("native_client_call")
		if hoverFirst == nil {
			t.Fatal("first hover should return non-nil result")
		}

		wantURI := requireFiveMNativeBundleURI(t, h.server, "natives_universal.lua")
		firstDoc := h.server.Documents[wantURI]
		if firstDoc == nil {
			t.Fatalf("native bundle %s not loaded after first hover", wantURI)
		}

		// Second hover should return the SAME document instance (warm cache)
		hoverSecond := h.hover("native_client_call")
		if hoverSecond == nil {
			t.Fatal("second hover should return non-nil result")
		}

		secondDoc := h.server.Documents[wantURI]
		if firstDoc != secondDoc {
			t.Fatal("native bundle document changed between hovers - expected same cached instance")
		}

		// Definition request should also use warm cache
		defs := h.definition("native_client_call")
		if len(defs) == 0 {
			t.Fatal("definition should return at least one result")
		}

		thirdDoc := h.server.Documents[wantURI]
		if firstDoc != thirdDoc {
			t.Fatal("native bundle document changed after definition - expected warm cache")
		}
	})

	t.Run("NativeSymbolsStayInGlobalIndex", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_natives")

		// Verify native symbol is in GlobalIndex after hover operations
		playerPedKey := GlobalKey{ReceiverHash: 0, PropHash: ast.HashBytes([]byte("PlayerPedId"))}

		// Do first hover to trigger any necessary loading
		hoverResult := h.hover("native_client_call")
		if hoverResult == nil {
			t.Fatal("hover should return non-nil result")
		}

		if syms := h.server.GlobalIndex.SymbolsByHash(playerPedKey); len(syms) == 0 {
			t.Fatal("PlayerPedId should be in GlobalIndex after hover")
		}

		// Do multiple operations - count should remain stable
		h.hover("native_client_call")
		h.definition("native_client_call")

		if syms := h.server.GlobalIndex.SymbolsByHash(playerPedKey); len(syms) == 0 {
			t.Fatal("PlayerPedId should remain in GlobalIndex after repeated operations")
		}
	})
}
