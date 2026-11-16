package lsp

import (
	"slices"
	"testing"
)

func TestVisibleSymbolsByKeyMatchesVisibleSymbols(t *testing.T) {
	tests := []struct {
		name  string
		scope GlobalIndexScope
	}{
		{name: "client", scope: GlobalIndexScopeClient},
		{name: "server", scope: GlobalIndexScopeServer},
		{name: "shared", scope: GlobalIndexScopeShared},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := NewGlobalIndex()
			key := GlobalKey{ReceiverHash: 7, PropHash: 11}
			other := GlobalKey{ReceiverHash: 8, PropHash: 12}
			root := idx.EnsureResource("root")
			depZ := idx.EnsureResource("dep-z")
			depA := idx.EnsureResource("dep-a")
			root.Dependencies = []ResourceURI{"dep-z", "missing", "dep-a"}
			selected := &SymbolEntry{Key: key}
			shared := &SymbolEntry{Key: key}
			collision := &SymbolEntry{Key: key}
			root.tableForScope(tt.scope)["z-map"] = selected
			root.tableForScope(tt.scope)["a-map"] = &SymbolEntry{Key: other}
			root.Shared["s-map"] = shared
			// The same pointer appears in a later table and must be deduplicated.
			root.Shared["t-map"] = selected
			depZ.tableForScope(tt.scope)["x-map"] = collision
			depZ.Shared["a-map"] = &SymbolEntry{Key: key}
			depA.tableForScope(tt.scope)["a-map"] = &SymbolEntry{Key: key}

			want := make([]*SymbolEntry, 0)
			for _, entry := range idx.VisibleSymbols("root", tt.scope) {
				if entry != nil && entry.Key == key {
					want = append(want, entry)
				}
			}
			got := idx.VisibleSymbolsByKey("root", tt.scope, key)
			if !slices.Equal(got, want) {
				t.Fatalf("filtered visible entries differ\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestVisibleSymbolsByKeyReadsMutationsLive(t *testing.T) {
	idx := NewGlobalIndex()
	res := idx.EnsureResource("root")
	key := GlobalKey{PropHash: 1}
	entry := &SymbolEntry{Key: key}
	res.Shared["name"] = entry
	if got := idx.VisibleSymbolsByKey("root", GlobalIndexScopeShared, key); len(got) != 1 || got[0] != entry {
		t.Fatalf("initial lookup = %#v", got)
	}
	entry.Key = GlobalKey{PropHash: 2}
	if got := idx.VisibleSymbolsByKey("root", GlobalIndexScopeShared, key); len(got) != 0 {
		t.Fatalf("lookup after direct key mutation = %#v, want none", got)
	}
	entry.Key = key
	delete(res.Shared, "name")
	res.Shared["replacement"] = entry
	if got := idx.VisibleSymbolsByKey("root", GlobalIndexScopeShared, key); len(got) != 1 || got[0] != entry {
		t.Fatalf("lookup after direct table mutation = %#v", got)
	}
}

func TestVisibleSymbolsByKeyMatchesCandidatesAndAliases(t *testing.T) {
	idx := NewGlobalIndex()
	key := GlobalKey{PropHash: 42}
	first := idx.AddSymbol("root", GlobalIndexScopeClient, "alpha", &SymbolEntry{URI: "a", NodeID: 1, Key: key})
	second := idx.AddSymbol("root", GlobalIndexScopeClient, "alpha", &SymbolEntry{URI: "b", NodeID: 2, Key: key})
	third := idx.AddSymbol("root", GlobalIndexScopeClient, "zeta", &SymbolEntry{URI: "c", NodeID: 3, Key: key})
	// Compatibility tables can contain aliases and entry names can change.
	idx.Resources["root"].Client["beta"] = third
	third.Name = "changed"
	first.Name = "zzz"
	want := []*SymbolEntry{}
	for _, entry := range idx.VisibleSymbols("root", GlobalIndexScopeClient) {
		if entry.Key == key {
			want = append(want, entry)
		}
	}
	got := idx.VisibleSymbolsByKey("root", GlobalIndexScopeClient, key)
	if !slices.Equal(got, want) || len(got) != 3 || got[0] != first || got[1] != second || got[2] != third {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}
