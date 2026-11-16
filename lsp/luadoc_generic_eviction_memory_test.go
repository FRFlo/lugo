package lsp

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func genericEvictionMemoryFixture(t testing.TB, functions, proseBytes int) (*Server, *Document) {
	t.Helper()
	var source strings.Builder
	for i := 0; i < functions; i++ {
		if proseBytes > 0 {
			source.WriteString("--- ")
			source.WriteString(strings.Repeat("x", proseBytes))
			source.WriteByte('\n')
		}
		fmt.Fprintf(&source, "---@generic T\n---@param value T\n---@return T\nfunction identity%d(value) return value end\n", i)
	}
	doc := benchmarkMetatableDocument(t, []byte(source.String()))
	s := NewServer("generic-eviction-memory")
	doc.URI, doc.Server = "file:///generic-memory.lua", s
	s.Documents[doc.URI] = doc
	return s, doc
}

func TestGenericEvictionRetainsOnlyContractsAndReleasesOnUpdate(t *testing.T) {
	s, doc := genericEvictionMemoryFixture(t, 1, 1<<20)
	evictClosedDocumentCaches(s)
	if doc.Source() == nil || doc.LuaDocCache != nil || !doc.TypeCache.Empty() {
		t.Fatal("eviction must preserve source and release dense document caches")
	}
	if len(doc.evictedGenericContracts) != 1 || len(doc.evictedNodeTextData) > 64 {
		t.Fatalf("generic eviction retained excessive state: contracts=%d text=%d", len(doc.evictedGenericContracts), len(doc.evictedNodeTextData))
	}
	if cap(s.sharedCommentBuf) > 64<<10 {
		t.Fatalf("generic eviction retained prose in the shared comment buffer: capacity=%d", cap(s.sharedCommentBuf))
	}
	if len(doc.evictedGenericContractData) > 128 {
		t.Fatalf("generic contract retained prose: %d bytes", len(doc.evictedGenericContractData))
	}
	for _, entry := range doc.evictedGenericContracts {
		contract := doc.evictedGenericCallAnnotations(entry.node)
		if len(contract.Params) != 1 || contract.Params[0].Desc != "" || len(contract.Returns) != 1 || contract.Returns[0].Desc != "" {
			t.Fatal("retained generic contract must exclude descriptions")
		}
	}
	s.updateDocument(doc.URI, []byte("local replacement = 42\n"))
	if doc.evictedGenericContracts != nil || doc.evictedGenericContractData != nil {
		t.Fatal("reindex retained stale generic contracts")
	}
}

func genericEvictionContractBytes(doc *Document) int {
	return cap(doc.evictedGenericContracts)*12 + cap(doc.evictedGenericContractData)
}

func BenchmarkGenericExportEvictionMemory(b *testing.B) {
	for _, functions := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("functions_%d", functions), func(b *testing.B) {
			s, doc := genericEvictionMemoryFixture(b, functions, 128)
			source := doc.Source()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				doc.Tree.Source = source
				evictClosedDocumentCaches(s)
			}
			b.StopTimer()
			// Use a separate fixture with no external reference to its source
			// so GC can measure the actual release on first eviction.
			sample, sampleDoc := genericEvictionMemoryFixture(b, functions, 128)
			sourceBytes := len(sampleDoc.Source())
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			evictClosedDocumentCaches(sample)
			runtime.GC()
			runtime.ReadMemStats(&after)
			b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)), "post-eviction-delta-B")
			b.ReportMetric(float64(genericEvictionContractBytes(sampleDoc)), "contract-payload-B")
			b.ReportMetric(float64(sourceBytes), "source-B")
			runtime.KeepAlive(sample)
			runtime.KeepAlive(s)
			runtime.KeepAlive(source)
		})
	}
}
