package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/semantic"
)

func TestMetatableBatchRefreshInvalidatesRemovedProvider(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("setup.lua", "object = {}\nsetmetatable(object, { __index = { answer = 42 } })\n")
	h.writeWorkspaceFile("middle.lua", "intermediate = object.--[[@batch_removed_middle]]answer\n")
	h.writeWorkspaceFile("read.lua", "local result = --[[@batch_removed_read]]intermediate\n")
	h.reindex()
	middle := h.docForMarker("batch_removed_middle")
	reader := h.docForMarker("batch_removed_read")
	resultID := findIdentByOccurrence(t, reader.Tree, "result", 0)
	if got := reader.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("before deletion = %#v, want number", got)
	}
	if got := inferLastNamedMember(t, middle, "answer"); got.Basics != TypeNumber {
		t.Fatalf("middle before deletion = %#v, want number", got)
	}
	if err := os.Remove(filepath.Join(h.root, "setup.lua")); err != nil {
		t.Fatal(err)
	}
	// Exercise the refresh prune phase with the existing consumers still warm.
	// Re-finalizing either consumer would hide a missing invalidation here.
	h.server.metatableInvalidationBatch = make(map[string]struct{})
	h.server.clearDocument(h.server.pathToURI(filepath.Join(h.root, "setup.lua")))
	h.server.flushMetatableInvalidationBatch()
	if got := reader.InferType(resultID); got.Basics&TypeNumber != 0 {
		t.Fatalf("transitive reader retained removed provider type: %#v", got)
	}
	if got := inferLastNamedMember(t, middle, "answer"); got.Basics&TypeNumber != 0 {
		t.Fatalf("middle retained removed provider type: %#v", got)
	}
}

func TestMetatableInvalidationBatchMatchesSequentialInvalidation(t *testing.T) {
	newServer := func() *Server {
		s := NewServer("metatable-invalidation-batch")
		for _, uri := range []string{"a", "b", "mid", "consumer"} {
			cache := typeCache{nodes: 1}
			inferring := true
			if uri == "a" || uri == "b" {
				// These are replaced documents, so finalizeDocumentUpdate has
				// already reset their own type caches and inference guards.
				inferring = false
			} else {
				cache.Put(0, TypeSet{Basics: TypeNumber})
			}
			s.Documents[uri] = &Document{TypeCache: cache, Inferring: []bool{inferring}}
		}
		s.trackMetatableTypeDependency("a", "mid")
		s.trackMetatableTypeDependency("mid", "consumer")
		s.trackMetatableTypeDependency("b", "consumer")
		s.trackMetatableTypeDependency("consumer", "a")
		return s
	}
	assertInvalidated := func(t *testing.T, s *Server) {
		t.Helper()
		for _, uri := range []string{"a", "b", "mid", "consumer"} {
			doc := s.Documents[uri]
			if got := doc.TypeCache.Get(0).Basics; got != TypeUnknown {
				t.Errorf("cache for %s = %#x, want invalidated", uri, got)
			}
			if doc.Inferring[0] {
				t.Errorf("inference guard for %s remained active", uri)
			}
		}
	}

	sequential := newServer()
	if got := sequential.Documents["consumer"].TypeCache.Get(0).Basics; got != TypeNumber {
		t.Fatalf("test precondition consumer cache = %#x, want number", got)
	}
	sequential.invalidateMetatableIndexes("a")
	sequential.invalidateMetatableIndexes("b")
	assertInvalidated(t, sequential)

	batched := newServer()
	batched.metatableInvalidationBatch = make(map[string]struct{})
	batched.invalidateMetatableIndexes("a")
	batched.invalidateMetatableIndexes("b")
	batched.flushMetatableInvalidationBatch()
	assertInvalidated(t, batched)
}

func TestMetatableInvalidationBatchFlushResetsChangedDocuments(t *testing.T) {
	s := NewServer("metatable-invalidation-batch-reset")
	doc := &Document{TypeCache: typeCache{nodes: 1}, Inferring: []bool{true}}
	doc.TypeCache.Put(0, TypeSet{Basics: TypeNumber})
	s.Documents["changed"] = doc
	s.metatableInvalidationBatch = make(map[string]struct{})
	s.invalidateMetatableIndexes("changed")
	s.flushMetatableInvalidationBatch()
	if got := doc.TypeCache.Get(0).Basics; got != TypeUnknown {
		t.Fatalf("changed document cache = %#x, want reset", got)
	}
	if doc.Inferring[0] {
		t.Fatal("changed document inference guard remained active")
	}
}

func TestMetatableBatchRefreshInvalidatesWhenProviderLoadsLater(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("read.lua", "local result = object.--[[@batch_late_provider_read]]answer\n")
	h.reindex()
	readDoc := h.docForMarker("batch_late_provider_read")
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics != TypeUnknown {
		t.Fatalf("before provider load = %#v, want unknown", got)
	}

	h.writeWorkspaceFile("setup.lua", "object = {}\nsetmetatable(object, { __index = { answer = 42 } })\n")
	h.reindex()
	readDoc = h.docForMarker("batch_late_provider_read")
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("after batched provider load = %#v, want number", got)
	}
}

func BenchmarkMetatableInvalidationBatchVsSequential(b *testing.B) {
	const documents = 128
	newWorkspace := func() (*Server, []string) {
		s := NewServer("metatable-invalidation-benchmark")
		uris := make([]string, documents)
		for i := 0; i < documents; i++ {
			uri := fmt.Sprintf("file:///bench/%03d.lua", i)
			uris[i] = uri
			var source strings.Builder
			nodes := make([]ast.Node, documents+1)
			refs := make([]ast.NodeID, 0, documents-1)
			var offset uint32
			for nameIndex := 0; nameIndex < documents; nameIndex++ {
				name := fmt.Sprintf("global_%03d", nameIndex)
				if nameIndex > 0 {
					source.WriteByte(' ')
					offset++
				}
				start := offset
				source.WriteString(name)
				offset += uint32(len(name))
				nodeID := ast.NodeID(nameIndex + 1)
				nodes[nodeID] = ast.Node{Kind: ast.KindIdent, Start: start, End: offset}
				if nameIndex > 0 {
					refs = append(refs, nodeID)
				}
			}
			tree := &ast.Tree{Source: []byte(source.String()), Nodes: nodes}
			resolver := &semantic.Resolver{Tree: tree, GlobalDefs: []ast.NodeID{1}, GlobalRefs: refs}
			s.Documents[uri] = &Document{Server: s, URI: uri, Tree: tree, Resolver: resolver}
		}
		return s, uris
	}

	b.Run("sequential", func(b *testing.B) {
		s, uris := newWorkspace()
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			for _, uri := range uris {
				s.invalidateMetatableIndexes(uri)
			}
		}
	})
	b.Run("batched", func(b *testing.B) {
		s, uris := newWorkspace()
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			s.metatableInvalidationBatch = make(map[string]struct{}, len(uris))
			for _, uri := range uris {
				s.invalidateMetatableIndexes(uri)
			}
			s.flushMetatableInvalidationBatch()
		}
	})
}
