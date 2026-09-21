package lsp

import (
	"encoding/json"
	"testing"
)

func TestCancelRequestTracksNumericAndStringIDs(t *testing.T) {
	for _, id := range []any{float64(42), "request-42"} {
		s := NewServer("test")
		params, err := json.Marshal(CancelRequestParams{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		s.handleCancelRequest(Request{Params: params})
		if !s.takeCanceledRequest(id) {
			t.Fatalf("request ID %#v was not cancelled", id)
		}
		if s.takeCanceledRequest(id) {
			t.Fatalf("request ID %#v remained cancelled", id)
		}
	}
}

func TestGlobalIndexPruneKeepsReferencedResource(t *testing.T) {
	idx := NewGlobalIndex()
	idx.EnsureResource("resource-a")
	idx.EnsureResource("resource-b")
	idx.DepGraph.SetDependencies("resource-b", []ResourceURI{"resource-a"})
	idx.syncResourceEdgesLocked()

	if idx.PruneResource("resource-a") {
		t.Fatal("referenced resource was pruned")
	}
	if idx.Resources["resource-a"] == nil {
		t.Fatal("referenced resource metadata was removed")
	}
}

func TestGlobalIndexPruneRemovesEmptyDocumentScope(t *testing.T) {
	idx := NewGlobalIndex()
	idx.SetSource("file:///closed.lua", []byte("return 1"), nil)

	if !idx.PruneResource("file:///closed.lua") {
		t.Fatal("empty document scope was not pruned")
	}
	if _, ok := idx.Resources["file:///closed.lua"]; ok {
		t.Fatal("pruned document scope remains in resources")
	}
	if got := idx.MemoryUsage(); got != 0 {
		t.Fatalf("memory usage after prune = %d, want 0", got)
	}
}

func TestNormalizeURICacheIsBounded(t *testing.T) {
	s := NewServer("test")
	for i := 0; i < maxURICacheEntries+32; i++ {
		s.normalizeURI("file:///lugo-cache-test/" + string(rune('a'+i%26)) + "/file.lua?" + string(rune(i)))
	}
	if len(s.uriCache) > maxURICacheEntries {
		t.Fatalf("URI cache size = %d, limit %d", len(s.uriCache), maxURICacheEntries)
	}
	if len(s.symlinkCache) > maxSymlinkCacheEntries {
		t.Fatalf("symlink cache size = %d, limit %d", len(s.symlinkCache), maxSymlinkCacheEntries)
	}
}
