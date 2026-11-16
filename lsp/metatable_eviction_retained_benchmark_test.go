package lsp

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// BenchmarkMetatableEvictionRetained measures eviction plus rebuilding the
// server candidate index. Parsing and initial index construction happen once,
// outside the timed loop. The custom retained metric is sampled separately
// after the timed loop so forced GC and MemStats reads do not distort ns/op.
func BenchmarkMetatableEvictionRetained(b *testing.B) {
	for _, writes := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("writes_%d", writes), func(b *testing.B) {
			var source strings.Builder
			source.Grow(writes * 48)
			source.WriteString("local sink = {}\nlocal object = setmetatable({}, { __newindex = sink })\n")
			for i := 0; i < writes; i++ {
				fmt.Fprintf(&source, "object.field%d = %d\n", i, i)
			}
			source.WriteString("local result = sink.field1\n")
			text := []byte(source.String())
			newFixture := func() (*Server, *Document) {
				s := NewServer("eviction-memory")
				s.IsCI = true
				doc := benchmarkMetatableDocument(b, text)
				doc.URI, doc.Server = "file:///eviction-memory.lua", s
				s.Documents[doc.URI] = doc
				_ = s.newIndexWriteCandidates("field1")
				return s, doc
			}
			checkRebuild := func(s *Server, doc *Document) {
				evictClosedDocumentCaches(s)
				s.invalidateMetatableIndexes()
				candidates := s.newIndexWriteCandidates("field1")
				if len(candidates) == 0 || string(candidates[0].doc.nodeSource(candidates[0].node)) != "field1" {
					b.Fatal("candidate index failed to rebuild after source eviction")
				}
				runtime.KeepAlive(doc)
			}

			s, doc := newFixture()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				checkRebuild(s, doc)
			}
			b.StopTimer()

			// Sample the first eviction/rebuild transition on a fresh fixture,
			// independently from benchmark timing and after parsing is complete.
			sampleServer, sampleDoc := newFixture()
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			checkRebuild(sampleServer, sampleDoc)
			runtime.GC()
			runtime.ReadMemStats(&after)
			retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			b.ReportMetric(float64(retained), "post-eviction-delta-B")
			b.ReportMetric(float64(len(sampleDoc.evictedNodeText)), "cached-node-text")
			b.ReportMetric(float64(len(text)), "source-B")
			runtime.KeepAlive(s)
			runtime.KeepAlive(sampleServer)
		})
	}
}
