package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkWorkspaceRefreshNewDocumentsPeak samples HeapAlloc while a refresh
// parses a burst of new files. The refresh's two-phase result collection keeps
// all parsed trees reachable until every parser worker exits. This benchmark
// reports a sampled high-water mark; it is intentionally an integration
// benchmark, not a precise allocator microbenchmark.
func BenchmarkWorkspaceRefreshNewDocumentsPeak(b *testing.B) {
	const files = 96
	const statements = 500
	h := newFiveMFixtureHarnessWithoutIndex(b)
	var source strings.Builder
	for i := range statements {
		fmt.Fprintf(&source, "local value%d = { key = %d, nested = { flag = true } }\n", i, i)
	}
	contents := source.String()
	for i := range files {
		path := filepath.Join(h.root, fmt.Sprintf("new-%03d.lua", i))
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	var aggregatePeak uint64
	for b.Loop() {
		// Make each iteration represent new documents. The prior iteration's
		// documents are cleared to avoid measuring retained workspace state.
		for uri := range h.server.Documents {
			h.server.clearDocument(uri)
		}
		runtime.GC()
		var peak atomic.Uint64
		var done atomic.Bool
		var sampler = make(chan struct{})
		go func() {
			defer close(sampler)
			var ms runtime.MemStats
			for !done.Load() {
				runtime.ReadMemStats(&ms)
				for old := peak.Load(); ms.HeapAlloc > old && !peak.CompareAndSwap(old, ms.HeapAlloc); old = peak.Load() {
				}
				time.Sleep(100 * time.Microsecond)
			}
		}()
		h.reindex()
		done.Store(true)
		<-sampler
		if value := peak.Load(); value > aggregatePeak {
			aggregatePeak = value
		}
	}
	b.ReportMetric(float64(aggregatePeak)/(1<<20), "peak-MiB")
}
