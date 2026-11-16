package lsp

import (
	"fmt"
	"github.com/FRFlo/lugo/ast"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// Regression: an assignment in the opposite branch must not replace the literal.
func TestSolReviewFieldWriteOppositeBranch(t *testing.T) {
	src := []byte("local mt = { __index = 'initial' }\nif condition then\n mt.__index = 'wrong'\nelse\n local result = mt.__index\nend\n")
	doc := benchmarkMetatableDocument(t, src)
	table := benchmarkFindKind(doc.Tree, ast.KindTableExpr, 0)
	result := benchmarkFindIdent(doc.Tree, "result", 0)
	value := doc.metatableFieldValueAt(table, "__index", doc.Tree.Nodes[result].Start)
	if got := string(doc.nodeSource(value)); got != "'initial'" {
		t.Fatalf("opposite branch leaked: got %q, want initial", got)
	}
}

// A warm server cache must not change its answers after ordinary cache eviction.
func TestSolReviewCandidateRebuildAfterEviction(t *testing.T) {
	s := NewServer("sol-review")
	doc := benchmarkMetatableDocument(t, []byte("local t = {}\nt.answer = 42\n"))
	doc.URI = "file:///sol-review.lua"
	doc.Server = s
	s.Documents[doc.URI] = doc
	if len(s.newIndexWriteCandidates("answer")) != 1 {
		t.Fatal("missing initial candidate")
	}
	evictClosedDocumentCaches(s)
	if doc.Source() != nil {
		t.Fatal("closed document retained its source after eviction")
	}
	s.invalidateMetatableIndexes()
	candidates := s.newIndexWriteCandidates("answer")
	if got := len(candidates); got != 1 {
		t.Fatalf("rebuild after source eviction: got %d candidates, want 1", got)
	}
	if got := string(candidates[0].doc.nodeSource(candidates[0].node)); got != "answer" {
		t.Fatalf("rebuilt candidate name = %q, want answer", got)
	}
}

func TestSolReviewSizes(t *testing.T) {
	t.Logf("Document=%d Server=%d TypeSet=%d LuaDocData=%d LuaDoc=%d candidate=%d attachment=%d fieldWrite=%d index=%d", unsafe.Sizeof(Document{}), unsafe.Sizeof(Server{}), unsafe.Sizeof(TypeSet{}), unsafe.Sizeof(LuaDocData{}), unsafe.Sizeof(LuaDoc{}), unsafe.Sizeof(newIndexWriteCandidate{}), unsafe.Sizeof(metatableAttachmentCall{}), unsafe.Sizeof(metatableFieldWrite{}), unsafe.Sizeof(metatableAttachmentIndex{}))
}

func BenchmarkSolReviewFieldIndexCold(b *testing.B) {
	doc := benchmarkMetatableDocument(b, []byte(memoryAuditDistinctFieldSource(1000)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		doc.invalidateMetatableAttachmentIndex()
		_ = doc.metatableFieldWrites()
	}
}

func BenchmarkSolReviewAttachmentCold(b *testing.B) {
	doc := benchmarkMetatableDocument(b, []byte(memoryAuditDistinctFieldSource(1000)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		doc.invalidateMetatableAttachmentIndex()
		_ = doc.metatableAttachmentCalls()
	}
}

func BenchmarkSolReviewClassRetained(b *testing.B) {
	var source strings.Builder
	source.WriteString("---@class Widget\n")
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&source, "---@field field%d string\n", i)
	}
	source.WriteString("local Widget = {}\n")
	text := []byte(source.String())

	// Standard ns/op and alloc metrics cover one complete server/document
	// update per iteration, with a fresh server so retained documents do not
	// accumulate between iterations.
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		server := NewServer("sol-review")
		server.updateDocument("file:///bench.lua", text)
		runtime.KeepAlive(server)
	}
	b.StopTimer()

	// Report a separate post-GC live-heap estimate for one retained document.
	server := NewServer("sol-review")
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	server.updateDocument("file:///bench.lua", text)
	runtime.GC()
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)), "retained-B")
	runtime.KeepAlive(server)
}

func BenchmarkSolReviewFieldLookupComparable(b *testing.B) {
	var source strings.Builder
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&source, "local noise%d = { value = %d }\n", i, i)
	}
	source.WriteString("local mt = { __newindex = {} }\nmt.__newindex = {}\n")
	doc := benchmarkMetatableDocument(b, []byte(source.String()))
	definition := benchmarkFindIdent(doc.Tree, "mt", 0)
	table := doc.getAssignedValue(definition)
	offset := uint32(source.Len())
	b.Run("indexed", func(b *testing.B) {
		value := doc.metatableFieldValueAt(table, "__newindex", offset)
		if value == ast.InvalidNode {
			b.Fatal("missing value")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = doc.metatableFieldValueAt(table, "__newindex", offset)
		}
	})
	b.Run("scan", func(b *testing.B) {
		value := metatableFieldValueAtScanBaseline(doc, definition, "__newindex", offset)
		if value == ast.InvalidNode {
			b.Fatal("missing value")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = metatableFieldValueAtScanBaseline(doc, definition, "__newindex", offset)
		}
	})
}
