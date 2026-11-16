package lsp

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

var metatableMemoryAuditIndex map[string][]newIndexWriteCandidate

// BenchmarkNewIndexWriteIndexScaling isolates the allocation cost of building
// the server's persistent, name-keyed redirected-write index. Parsing and
// resolution are intentionally outside the timed region.
func BenchmarkNewIndexWriteIndexScaling(b *testing.B) {
	for _, writes := range []int{10, 50, 100, 200} {
		b.Run(fmt.Sprintf("writes_%d", writes), func(b *testing.B) {
			doc := benchmarkMetatableDocument(b, []byte(memoryAuditNewIndexSource(writes)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				metatableMemoryAuditIndex = buildNewIndexWriteIndexFromSlice([]*Document{doc})
			}
			metatableMemoryAuditIndex = nil
		})
	}
}

// BenchmarkNewIndexWriteIndexRetained estimates additional live heap after
// lazily building the server index. The documents and ASTs are already live
// before the first heap snapshot, so this isolates the index's maps/slices and
// candidate records, subject to normal MemStats measurement noise.
func BenchmarkNewIndexWriteIndexRetained(b *testing.B) {
	for _, docs := range []int{1, 10, 50} {
		for _, writes := range []int{100, 1000} {
			b.Run(fmt.Sprintf("documents_%d/writes_%d", docs, writes), func(b *testing.B) {
				s := NewServer("memory-audit")
				s.IsCI = true
				for i := 0; i < docs; i++ {
					uri := fmt.Sprintf("file:///audit/%d.lua", i)
					doc := benchmarkMetatableDocument(b, []byte(memoryAuditNewIndexSource(writes)))
					doc.URI = uri
					doc.Server = s
					s.Documents[uri] = doc
				}
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				_ = s.newIndexWriteCandidates("answer")
				runtime.GC()
				runtime.ReadMemStats(&after)
				runtime.KeepAlive(s)
				retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
				if retained < 0 {
					retained = 0 // below the resolution of this heap snapshot
				}
				b.ReportMetric(float64(retained), "retained-B")
				b.ReportMetric(float64(retained)/float64(docs*writes), "retained-B/doc-write")
				b.ReportMetric(float64(docs), "documents")
				b.ReportMetric(float64(writes), "writes/doc")

				// clearDocument removes each document from the server and bumps
				// the cache version. The existing cache still points at its docs
				// until a later lookup rebuilds it.
				for i := 0; i < docs; i++ {
					s.clearDocument(fmt.Sprintf("file:///audit/%d.lua", i))
				}
				runtime.GC()
				var afterClose, afterDrop runtime.MemStats
				runtime.ReadMemStats(&afterClose)
				s.newIndexWrites = nil
				runtime.GC()
				runtime.ReadMemStats(&afterDrop)
				released := int64(afterClose.HeapAlloc) - int64(afterDrop.HeapAlloc)
				if released < 0 {
					released = 0
				}
				b.ReportMetric(float64(released), "released-on-drop-B")
				runtime.KeepAlive(s)
			})
		}
	}
}

// BenchmarkMetatableAttachmentIndexRetained measures the per-document cache,
// which indexes all resolved field writes (not only __newindex writes).
func BenchmarkMetatableAttachmentIndexRetained(b *testing.B) {
	for _, writes := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("writes_%d", writes), func(b *testing.B) {
			doc := benchmarkMetatableDocument(b, []byte(memoryAuditDistinctFieldSource(writes)))
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_ = doc.metatableFieldWrites()
			runtime.GC()
			runtime.ReadMemStats(&after)
			runtime.KeepAlive(doc)
			retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			if retained < 0 {
				retained = 0
			}
			b.ReportMetric(float64(retained), "retained-B")
			b.ReportMetric(float64(retained)/float64(writes), "retained-B/write")
			b.ReportMetric(float64(writes), "writes")
		})
	}
}

func memoryAuditNewIndexSource(writes int) string {
	var source strings.Builder
	source.Grow(90 + writes*45)
	source.WriteString("local sink = {}\nlocal object = setmetatable({}, { __newindex = sink })\n")
	for i := 0; i < writes; i++ {
		fmt.Fprintf(&source, "if condition%d then object.answer = %d end\n", i, i)
	}
	source.WriteString("local result = sink.answer\n")
	return source.String()
}

func memoryAuditDistinctFieldSource(writes int) string {
	var source strings.Builder
	source.Grow(20 + writes*24)
	source.WriteString("local target = {}\n")
	for i := 0; i < writes; i++ {
		fmt.Fprintf(&source, "target.field%d = %d\n", i, i)
	}
	return source.String()
}

func BenchmarkMetatableAttachmentOnlyRetained(b *testing.B) {
	for _, writes := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("writes_%d", writes), func(b *testing.B) {
			doc := benchmarkMetatableDocument(b, []byte(memoryAuditDistinctFieldSource(writes)))
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_ = doc.metatableAttachmentCalls()
			runtime.GC()
			runtime.ReadMemStats(&after)
			runtime.KeepAlive(doc)
			retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			if retained < 0 {
				retained = 0
			}
			b.ReportMetric(float64(retained), "retained-B")
		})
	}
}

func BenchmarkGlobalMetatableAttachmentIndexRetained(b *testing.B) {
	for _, calls := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("calls_%d", calls), func(b *testing.B) {
			var source strings.Builder
			source.Grow(calls * 58)
			for i := 0; i < calls; i++ {
				fmt.Fprintf(&source, "setmetatable(object, { __index = { field%d = %d } })\n", i, i)
			}
			doc := benchmarkMetatableDocument(b, []byte(source.String()))
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			index := doc.metatableAttachmentCalls()
			runtime.GC()
			runtime.ReadMemStats(&after)
			runtime.KeepAlive(doc)
			retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			if retained < 0 {
				retained = 0
			}
			b.ReportMetric(float64(retained), "retained-B")
			b.ReportMetric(float64(retained)/float64(calls), "retained-B/call")
			b.ReportMetric(float64(len(index.byGlobalName[ast.HashBytes([]byte("object"))])), "indexed-calls")
		})
	}
}
