package lsp

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// TestEvictionRetainsGlobalReferenceHashes checks that closed documents keep
// the compact name hashes needed for later provider invalidation.
func TestEvictionRetainsGlobalReferenceHashes(t *testing.T) {
	const names = 256
	var source strings.Builder
	for i := range names {
		fmt.Fprintf(&source, "local value%d = globalName%d\n", i, i)
	}
	doc := benchmarkMetatableDocument(t, []byte(source.String()))
	doc.URI = "file:///evicted-global-refs.lua"
	s := NewServer("evicted-global-refs")
	doc.Server = s
	s.Documents[doc.URI] = doc

	evictClosedDocumentCaches(s)
	runtime.GC()
	runtime.KeepAlive(s)
	if doc.Source() != nil {
		t.Fatal("source must be released after eviction")
	}
	if got := len(doc.evictedGlobalRefs); got != names {
		t.Fatalf("retained global reference hashes = %d, want %d", got, names)
	}
}

// BenchmarkEvictedGlobalReferenceRetention measures the retained text/index
// representation for many distinct and repeated unresolved global names.
func BenchmarkEvictedGlobalReferenceRetention(b *testing.B) {
	for _, names := range []int{100, 1000, 5000} {
		for _, unique := range []int{1, names} {
			b.Run(fmt.Sprintf("refs_%d/unique_%d", names, unique), func(b *testing.B) {
				var source strings.Builder
				for i := range names {
					fmt.Fprintf(&source, "local value%d = globalName%d\n", i, i%unique)
				}
				text := []byte(source.String())
				newFixture := func() (*Server, *Document) {
					doc := benchmarkMetatableDocument(b, append([]byte(nil), text...))
					doc.URI = "file:///evicted-global-refs.lua"
					s := NewServer("evicted-global-refs")
					doc.Server = s
					s.Documents[doc.URI] = doc
					return s, doc
				}
				b.ReportAllocs()
				b.ReportMetric(float64(len(text)), "source-B")
				b.ResetTimer()
				b.StopTimer()
				for i := 0; i < b.N; i++ {
					s, _ := newFixture()
					b.StartTimer()
					evictClosedDocumentCaches(s)
					b.StopTimer()
					runtime.KeepAlive(s)
				}

				s, doc := newFixture()
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				evictClosedDocumentCaches(s)
				runtime.GC()
				runtime.ReadMemStats(&after)
				runtime.KeepAlive(s)
				retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
				if retained < 0 {
					retained = 0
				}
				b.ReportMetric(float64(retained), "eviction-delta-B")
				b.ReportMetric(float64(len(doc.evictedNodeText)), "retained-node-entries")
				b.ReportMetric(float64(len(doc.evictedNodeTextData)), "retained-text-B")
				b.ReportMetric(float64(len(doc.evictedGlobalRefs)), "retained-global-hashes")
			})
		}
	}
}
