package lsp

import (
	"runtime"
	"strings"
	"testing"
)

func BenchmarkFiveMManifestEvictionMemory(b *testing.B) {
	newFixture := func() (*Server, *Document) {
		doc := benchmarkMetatableDocument(b, []byte("-- "+strings.Repeat("x", 1<<20)+"\nfx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n"))
		s := NewServer("manifest-eviction-memory")
		doc.URI, doc.Server = "file:///resource/fxmanifest.lua", s
		doc.IsFiveMManifest = true
		s.Documents[doc.URI] = doc
		s.registerFiveMManifestResource(s.parseFiveMManifest(doc))
		return s, doc
	}
	s, doc := newFixture()
	source := doc.Source()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		doc.Tree.Source = source
		evictClosedDocumentCaches(s)
	}
	b.StopTimer()
	sample, sampleDoc := newFixture()
	sourceBytes := len(sampleDoc.Source())
	model := sampleDoc.FiveMManifestResource
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	evictClosedDocumentCaches(sample)
	runtime.GC()
	runtime.ReadMemStats(&after)
	if sampleDoc.FiveMManifestResource != model || sample.FiveMResourceGraph.ResourceByRoot(model.RootURI) != model {
		b.Fatal("manifest eviction copied the semantic model")
	}
	b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)), "post-eviction-delta-B")
	b.ReportMetric(float64(sourceBytes), "source-B")
	runtime.KeepAlive(sample)
	runtime.KeepAlive(s)
	runtime.KeepAlive(source)
}
