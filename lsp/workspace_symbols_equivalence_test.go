package lsp

import (
	"fmt"
	"slices"
	"testing"
)

func TestWorkspaceSymbolsLimitedMatchesUnbounded(t *testing.T) {
	idx := NewGlobalIndex()
	scopes := []GlobalIndexScope{GlobalIndexScopeClient, GlobalIndexScopeServer, GlobalIndexScopeShared}
	for resource := range 7 {
		for i := range 240 {
			name := SymbolName(fmt.Sprintf("Symbol%03d_%d", i%80, (i*13+resource)%17))
			idx.AddSymbol(ResourceURI(fmt.Sprintf("file:///resource/%d", resource)), scopes[i%len(scopes)], name, &SymbolEntry{})
		}
	}
	for _, query := range []string{"", "symbol", "s0", "Symb001", "zzz"} {
		full := idx.WorkspaceSymbols(query, -1)
		for _, limit := range []int{1, 7, 100, 3000} {
			want := full[:min(limit, len(full))]
			for range 5 {
				got := idx.WorkspaceSymbols(query, limit)
				if !slices.Equal(got, want) {
					t.Fatalf("query %q limit %d returned different entries/order from unlimited search", query, limit)
				}
			}
		}
	}
}

func TestWorkspaceSymbolsSharedEntriesPreserveStableOrder(t *testing.T) {
	idx := NewGlobalIndex()
	a := idx.AddSymbol("file:///a", GlobalIndexScopeShared, "A", &SymbolEntry{})
	b := idx.AddSymbol("file:///a", GlobalIndexScopeShared, "B", &SymbolEntry{})
	// Entries may be registered under more than one name, and AddSymbol updates
	// the shared entry's display name each time.
	idx.AddSymbol("file:///b", GlobalIndexScopeClient, "Z", a)
	idx.AddSymbol("file:///c", GlobalIndexScopeClient, "Z", b)
	want := idx.WorkspaceSymbols("", -1)
	for range 100 {
		if got := idx.WorkspaceSymbols("", 2); !slices.Equal(got, want[:2]) {
			t.Fatal("limited search changed stable ordering for shared entries")
		}
	}
}

func TestWorkspaceSymbolsPreservesCandidateAndLegacyRepresentatives(t *testing.T) {
	idx := NewGlobalIndex()
	first := idx.AddSymbol("file:///a", GlobalIndexScopeShared, "WorkspaceMenu", &SymbolEntry{URI: "file:///a", NodeID: 1})
	second := idx.AddSymbol("file:///b", GlobalIndexScopeShared, "WorkspaceMenu", &SymbolEntry{URI: "file:///b", NodeID: 2})
	legacy := &SymbolEntry{Name: "WorkspaceMenu", URI: "file:///legacy", NodeID: 3}
	idx.EnsureResource("file:///legacy").Shared["WorkspaceMenu"] = legacy

	want := []*SymbolEntry{first, second, legacy}
	full := idx.WorkspaceSymbols("WorkspaceMenu", -1)
	if !slices.Equal(full, want) {
		t.Fatalf("unbounded results = %#v, want %#v", full, want)
	}
	for limit := 1; limit <= len(want); limit++ {
		if got := idx.WorkspaceSymbols("WorkspaceMenu", limit); !slices.Equal(got, want[:limit]) {
			t.Fatalf("limit %d results = %#v, want prefix %#v", limit, got, want[:limit])
		}
	}
}
