package lsp

import (
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestEvictedGlobalReferenceTextIsInterned(t *testing.T) {
	doc := newIndexTestDocument(t, `local first = repeatedGlobal
local second = repeatedGlobal
`)
	doc.cacheEvictedMetatableNodeText()
	doc.Tree.Source = nil
	if got := strings.Count(string(doc.evictedNodeTextData), "repeatedGlobal"); got != 1 {
		t.Fatalf("retained copies of repeated global spelling = %d, want 1", got)
	}
	if _, ok := doc.evictedGlobalRefs[ast.HashBytes([]byte("repeatedGlobal"))]; !ok {
		t.Fatal("eviction did not retain the unresolved global hash for invalidation")
	}
}

func TestReloadedDocumentReleasesEvictedNodeText(t *testing.T) {
	const staleName = "staleEvictedGlobalName"
	doc := newIndexTestDocument(t, "local value = "+staleName+"\n")
	server := NewServer("test")
	doc.Server = server
	server.Documents[doc.URI] = doc
	evictClosedDocumentCaches(server)
	if doc.Source() == nil || len(doc.evictedNodeTextData) == 0 {
		t.Fatal("setup did not retain source and compact node text during cache eviction")
	}
	if _, ok := doc.evictedGlobalRefs[ast.HashBytes([]byte(staleName))]; !ok {
		t.Fatal("setup did not retain the stale global hash")
	}

	server.updateDocument(doc.URI, []byte("local value = replacementGlobalName\n"))
	if doc.evictedNodeText != nil || doc.evictedNodeTextData != nil || doc.evictedGlobalRefs != nil {
		t.Fatalf("reloaded document retained evicted text: entries=%d bytes=%d hashes=%d", len(doc.evictedNodeText), len(doc.evictedNodeTextData), len(doc.evictedGlobalRefs))
	}
	if got := string(doc.nodeSource(findIdentByOccurrence(t, doc.Tree, "replacementGlobalName", 0))); got != "replacementGlobalName" {
		t.Fatalf("reloaded source lookup = %q, want replacement token", got)
	}
}

func TestMetatableIndexInvalidationReleasesCandidates(t *testing.T) {
	s := NewServer("test")
	doc := newIndexTestDocument(t, `local sink = {}
sink.value = 1
`)
	doc.Server = s
	s.Documents[doc.URI] = doc
	_ = s.newIndexWriteCandidates("value")
	if s.newIndexWrites == nil {
		t.Fatal("candidate cache was not built")
	}
	s.invalidateMetatableIndexes()
	if s.newIndexWrites != nil {
		t.Fatal("invalidated cache still retains candidate documents")
	}
}

func TestClosedDocumentEvictionReleasesMetatableIndex(t *testing.T) {
	s := NewServer("test")
	doc := newIndexTestDocument(t, `local mt = {}
mt.__index = {}
`)
	doc.Server = s
	s.Documents[doc.URI] = doc
	_ = doc.metatableAttachmentCalls()
	if doc.metatableCalls == nil {
		t.Fatal("document cache was not built")
	}
	evictClosedDocumentCaches(s)
	if doc.metatableCalls != nil {
		t.Fatal("closed document still retains metatable index")
	}
}

func TestOpenAndLibraryDocumentsKeepMetatableIndex(t *testing.T) {
	for _, tc := range []struct {
		name          string
		open, library bool
	}{
		{name: "open", open: true},
		{name: "library", library: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer("test")
			doc := newIndexTestDocument(t, "local mt = {}\nmt.__index = {}\n")
			doc.Server, doc.IsLibrary = s, tc.library
			s.Documents[doc.URI] = doc
			s.OpenFiles[doc.URI] = tc.open
			cached := doc.metatableAttachmentCalls()
			evictClosedDocumentCaches(s)
			if doc.metatableCalls != cached {
				t.Fatal("active or library document cache was evicted")
			}
		})
	}
}

func TestOrdinaryIdentifierInferenceDoesNotIndexUnrelatedFieldWrites(t *testing.T) {
	doc := newIndexTestDocument(t, `local object = {}
object.first = 1
object.second = 2
local result = object
`)
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	_ = doc.InferType(id)
	if doc.metatableCalls != nil && len(doc.metatableCalls.fieldWrites) != 0 {
		t.Fatalf("ordinary identifier inference indexed %d receivers without metatables", len(doc.metatableCalls.fieldWrites))
	}
}
