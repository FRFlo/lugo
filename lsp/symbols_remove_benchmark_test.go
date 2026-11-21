package lsp

import (
	"fmt"
	"testing"
)

func BenchmarkRemoveGlobalIndexDocumentSymbolsMiss(b *testing.B) {
	for _, count := range []int{100, 10_000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			index := NewGlobalIndex()
			resource := NewResourceScope("file:///benchmark")
			index.Resources[resource.URI] = resource
			for i := 0; i < count; i++ {
				name := SymbolName(fmt.Sprint(i))
				entry := &SymbolEntry{URI: "file:///other.lua"}
				resource.Candidates[GlobalIndexScopeShared][name] = []*SymbolEntry{entry}
				resource.Shared[name] = entry
			}
			server := &Server{GlobalIndex: index}
			b.ReportAllocs()
			for b.Loop() {
				server.removeGlobalIndexDocumentSymbols("file:///missing.lua")
			}
		})
	}
}
