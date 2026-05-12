package lsp

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestFiveMInvalidation tests various FiveM cache invalidation scenarios.
func TestFiveMInvalidation(t *testing.T) {
	t.Run("ManifestFileDelete_FiveMStateFullyRemoved", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		// Verify initial FiveM state
		resourceRoot := h.server.pathToURI(filepath.Join(h.root, "surface_resource"))
		if h.server.FiveMResourceGraph.ByRoot[resourceRoot] == nil {
			t.Fatal("resource should be registered before deletion")
		}
		if h.server.FiveMResourceGraph.ByRoot[resourceRoot] == nil {
			t.Fatal("resource should be in graph before deletion")
		}

		// Delete the manifest via watched files
		manifestURI := h.server.pathToURI(filepath.Join(h.root, "surface_resource", "fxmanifest.lua"))
		h.simulateWatchedFileDelete(manifestURI)

		// Verify Graph ByRoot cleaned
		if h.server.FiveMResourceGraph.ByRoot[resourceRoot] != nil {
			t.Fatal("FiveMResourceGraph.ByRoot should be empty after manifest delete")
		}

		// Verify graph name entry cleaned
		if h.server.FiveMResourceGraph.ByName["surface_resource"] != nil {
			t.Fatal("FiveMResourceGraph.ByName should not contain resource after manifest delete")
		}

		// Verify graph node removed
		if h.server.FiveMResourceGraph.ByRoot[resourceRoot] != nil {
			t.Fatal("FiveMResourceGraph.ByRoot should be empty after manifest delete")
		}

		// Verify all documents under resource have FiveMProfileCached=false
		for uri, doc := range h.server.Documents {
			if doc == nil {
				continue
			}
			if filepath.Dir(uri) == resourceRoot || uri == resourceRoot {
				if doc.FiveMProfileCached {
					t.Fatalf("document %s should have FiveMProfileCached=false after manifest delete", uri)
				}
			}
		}
	})

	t.Run("ManifestFileDelete_UnrelatedResourcesUnaffected", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared", "resource_dual_listed")

		dualRoot := h.server.pathToURI(filepath.Join(h.root, "dual_resource"))
		surfaceRoot := h.server.pathToURI(filepath.Join(h.root, "surface_resource"))

		// Verify both resources are registered
		if h.server.FiveMResourceGraph.ByRoot[surfaceRoot] == nil {
			t.Fatal("surface_resource should be registered")
		}
		if h.server.FiveMResourceGraph.ByRoot[dualRoot] == nil {
			t.Fatal("dual_resource should be registered")
		}

		// Delete only surface_resource's manifest
		manifestURI := h.server.pathToURI(filepath.Join(h.root, "surface_resource", "fxmanifest.lua"))
		h.simulateWatchedFileDelete(manifestURI)

		// Verify surface_resource is removed
		if h.server.FiveMResourceGraph.ByRoot[surfaceRoot] != nil {
			t.Fatal("surface_resource should be removed")
		}

		// Verify dual_resource is unaffected
		if h.server.FiveMResourceGraph.ByRoot[dualRoot] == nil {
			t.Fatal("dual_resource should be unaffected by surface_resource deletion")
		}
		if h.server.FiveMResourceGraph.ByName["dual_resource"] == nil {
			t.Fatal("dual_resource should still be in Graph.ByName")
		}
	})

	t.Run("NonManifestFileDelete_SiblingProfilesInvalidated", func(t *testing.T) {
		h := newFiveMFixtureHarness(t, "resource_client_server_shared")

		resourceRoot := h.server.pathToURI(filepath.Join(h.root, "surface_resource"))
		clientURI := h.server.pathToURI(filepath.Join(h.root, "surface_resource", "client.lua"))
		serverURI := h.server.pathToURI(filepath.Join(h.root, "surface_resource", "server.lua"))

		// Capture pre-delete profile cached state for sibling
		serverDocBefore := h.server.Documents[serverURI]
		hadCachedProfileBefore := serverDocBefore != nil && serverDocBefore.FiveMProfileCached
		if !hadCachedProfileBefore {
			t.Fatal("server.lua should have FiveMProfileCached initially")
		}

		// Delete client.lua (non-manifest file) - this removes the doc but should invalidate sibling
		h.simulateWatchedFileDelete(clientURI)

		// Verify client.lua document is gone (clearDocument called after profile invalidation)
		if h.server.Documents[clientURI] != nil {
			t.Fatal("client.lua should be removed from Documents after deletion")
		}

		// Verify server.lua document still exists
		serverDocAfter := h.server.Documents[serverURI]
		if serverDocAfter == nil {
			t.Fatal("server.lua should still exist in Documents after sibling deletion")
		}

		// Verify server.lua profile was invalidated (FiveMProfileCached=false after sibling delete)
		if serverDocAfter.FiveMProfileCached {
			t.Fatal("server.lua should have FiveMProfileCached=false after sibling Lua deletion")
		}

		// Verify Graph state persists after non-manifest deletion
		if h.server.FiveMResourceGraph.ByRoot[resourceRoot] == nil {
			t.Fatal("FiveMResourceGraph.ByRoot should still contain resource after non-manifest deletion")
		}
	})
}

// simulateWatchedFileDelete simulates a file deletion via the watched files handler.
func (h *fiveMFixtureHarness) simulateWatchedFileDelete(uri string) {
	h.t.Helper()

	params, err := json.Marshal(DidChangeWatchedFilesParams{
		Changes: []FileEvent{
			{URI: uri, Type: 3}, // Deleted
		},
	})
	if err != nil {
		h.t.Fatalf("marshal watched files params: %v", err)
	}

	h.resetRPC()
	h.server.handleDidChangeWatchedFiles(Request{RPC: "2.0", ID: 1, Params: params})
}
