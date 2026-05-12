package lsp

import (
	"slices"
	"testing"

	"github.com/coalaura/lugo/ast"
)

func TestGlobalIndexV2(t *testing.T) {
	t.Run("Lookup", func(t *testing.T) {
		idx := NewGlobalIndexV2()
		key := GlobalKey{ReceiverHash: 7, PropHash: 11}
		entry := &SymbolEntry{Key: key, Type: Type{Primitive: TypeString}, LuaDoc: &LuaDocData{}, FiveM: &FiveMData{}, Export: &ExportData{}}

		idx.AddSymbol("file:///resource", GlobalIndexScopeClient, "PlayerName", entry)

		byScope := idx.LookupByScope("file:///resource", GlobalIndexScopeClient, "PlayerName")
		if byScope == nil {
			t.Fatal("LookupByScope returned nil")
		}
		if byScope.Type.Primitive != TypeString || byScope.LuaDoc == nil || byScope.FiveM == nil || byScope.Export == nil {
			t.Fatalf("symbol entry metadata = %+v, want type and semantic pointers preserved", byScope)
		}

		byHash := idx.LookupByHash(key)
		if len(byHash) != 1 || byHash[0].Name != "PlayerName" || byHash[0].Type.Primitive != TypeString {
			t.Fatalf("LookupByHash = %+v, want PlayerName string entry", byHash)
		}
	})

	t.Run("AddSymbolReplacesStaleHashEntry", func(t *testing.T) {
		idx := NewGlobalIndexV2()
		key := GlobalKey{ReceiverHash: 13, PropHash: 17}

		first := idx.AddSymbol("file:///resource", GlobalIndexScopeClient, "PlayerName", &SymbolEntry{Key: key, Type: Type{Primitive: TypeString}})
		if first == nil {
			t.Fatal("first AddSymbol returned nil")
		}

		second := idx.AddSymbol("file:///resource", GlobalIndexScopeClient, "PlayerName", &SymbolEntry{Key: key, Type: Type{Primitive: TypeNumber}})
		if second == nil {
			t.Fatal("second AddSymbol returned nil")
		}

		byHash := idx.LookupByHash(key)
		if len(byHash) != 1 {
			t.Fatalf("LookupByHash after re-register = %+v, want one entry", byHash)
		}
		if byHash[0].Type.Primitive != TypeNumber {
			t.Fatalf("LookupByHash after re-register = %+v, want latest entry", byHash)
		}
		if got := idx.LookupByScope("file:///resource", GlobalIndexScopeClient, "PlayerName"); got != second {
			t.Fatalf("LookupByScope = %+v, want latest symbol entry", got)
		}
	})

	t.Run("ScopePartitioning", func(t *testing.T) {
		idx := NewGlobalIndexV2()
		res := idx.RegisterFiveMResource(&FiveMResource{
			Name:        "inventory",
			RootURI:     "file:///resources/inventory",
			ClientGlobs: []string{"client.lua", "dual.lua"},
			ServerGlobs: []string{"server.lua", "dual.lua"},
			SharedGlobs: []string{"shared.lua"},
		}, true)

		if res == nil {
			t.Fatal("RegisterFiveMResource returned nil")
		}
		if res.Client == nil || res.Server == nil || res.Shared == nil {
			t.Fatalf("symbol tables were not initialized: %+v", res)
		}
		if got := res.ScriptScopes["client.lua"]; got != GlobalIndexScopeClient {
			t.Fatalf("client.lua scope = %q, want client", got)
		}
		if got := res.ScriptScopes["server.lua"]; got != GlobalIndexScopeServer {
			t.Fatalf("server.lua scope = %q, want server", got)
		}
		if got := res.ScriptScopes["shared.lua"]; got != GlobalIndexScopeShared {
			t.Fatalf("shared.lua scope = %q, want shared", got)
		}
		if got := res.ScriptScopes["dual.lua"]; got != GlobalIndexScopeShared {
			t.Fatalf("dual.lua scope = %q, want shared for EC2 dual-listed script", got)
		}

		idx.AddSymbol(res.URI, GlobalIndexScopeClient, "ClientOnly", &SymbolEntry{})
		idx.AddSymbol(res.URI, GlobalIndexScopeServer, "ServerOnly", &SymbolEntry{})
		idx.AddSymbol(res.URI, GlobalIndexScopeShared, "SharedOnly", &SymbolEntry{})

		if idx.LookupByScope(res.URI, GlobalIndexScopeClient, "ServerOnly") != nil {
			t.Fatal("server symbol leaked into client scope")
		}
		if idx.LookupByScope(res.URI, GlobalIndexScopeShared, "SharedOnly") == nil {
			t.Fatal("shared symbol missing from shared scope")
		}
	})

	t.Run("TopologicalSort", func(t *testing.T) {
		idx := NewGlobalIndexV2()
		idx.DepGraph.SetDependencies("A", []ResourceURI{"B"})
		idx.DepGraph.SetDependencies("B", []ResourceURI{"C"})
		idx.DepGraph.SetDependencies("C", nil)

		ordered, diags := idx.TopologicalSort()
		if len(diags) != 0 {
			t.Fatalf("topological sort diagnostics = %+v, want none", diags)
		}
		if !slices.Equal(ordered, []ResourceURI{"C", "B", "A"}) {
			t.Fatalf("topological order = %#v, want C, B, A", ordered)
		}
	})

	t.Run("Eviction", func(t *testing.T) {
		idx := NewGlobalIndexV2()
		idx.AddSymbol("file:///res", GlobalIndexScopeShared, "Persisted", &SymbolEntry{Type: Type{Primitive: TypeNumber}})
		tree := ast.NewTree([]byte("Persisted = 1\n"))
		idx.SetSource("file:///res", []byte("Persisted = 1\n"), tree)

		before := idx.MemoryUsage()
		if before == 0 {
			t.Fatal("MemoryUsage should account source and AST bytes before eviction")
		}
		if !idx.EvictSource("file:///res") {
			t.Fatal("EvictSource returned false")
		}
		if got := idx.MemoryUsage(); got >= before {
			t.Fatalf("MemoryUsage after eviction = %d, want less than %d", got, before)
		}
		if entry := idx.LookupByScope("file:///res", GlobalIndexScopeShared, "Persisted"); entry == nil || entry.Type.Primitive != TypeNumber {
			t.Fatalf("symbol metadata after eviction = %+v, want persisted number entry", entry)
		}
		res := idx.Resources["file:///res"]
		if res.Source != nil || res.AST != nil {
			t.Fatalf("source/AST not evicted: source=%v ast=%v", res.Source, res.AST)
		}
	})

	t.Run("MemoryBudgetEvictsLRU", func(t *testing.T) {
		idx := NewGlobalIndexV2(8)
		idx.SetSource("file:///old", []byte("old source"), nil)
		idx.SetSource("file:///new", []byte("new source"), nil)

		if idx.MemoryUsage() > idx.MaxMemory {
			t.Fatalf("MemoryUsage = %d, want <= budget %d", idx.MemoryUsage(), idx.MaxMemory)
		}
		if idx.Resources["file:///old"].Source != nil {
			t.Fatal("oldest source should be evicted first")
		}
	})
}

func TestDependencyCycle(t *testing.T) {
	idx := NewGlobalIndexV2()
	idx.DepGraph.SetDependencies("A", []ResourceURI{"B"})
	idx.DepGraph.SetDependencies("B", []ResourceURI{"A"})

	ordered, diags := idx.TopologicalSort()
	if len(ordered) != 2 {
		t.Fatalf("cycle topological order len = %d, want 2", len(ordered))
	}
	if len(diags) == 0 {
		t.Fatal("expected circular dependency diagnostic")
	}
	if diags[0].Severity != SeverityWarning || diags[0].Code != "fivem-circular-dependency" {
		t.Fatalf("cycle diagnostic = %+v, want warning fivem-circular-dependency", diags[0])
	}
}

func TestScopePartitioning(t *testing.T) {
	idx := NewGlobalIndexV2()
	manifest := &FiveMManifest{Entries: []FiveMManifestEntry{
		{EmittedName: "client_script", Value: "client.lua"},
		{EmittedName: "server_script", Value: "server.lua"},
		{EmittedName: "client_script", Value: "dual.lua"},
		{EmittedName: "server_script", Value: "dual.lua"},
		{EmittedName: "shared_script", Value: "shared.lua"},
	}}

	res := idx.RegisterFiveMResource(&FiveMResource{Name: "res", RootURI: "file:///res", Manifest: manifest}, true)
	if got := res.ScriptScopes["dual.lua"]; got != GlobalIndexScopeShared {
		t.Fatalf("dual.lua scope = %q, want shared", got)
	}
	if got := res.ScriptScopes["client.lua"]; got != GlobalIndexScopeClient {
		t.Fatalf("client.lua scope = %q, want client", got)
	}
	if got := res.ScriptScopes["server.lua"]; got != GlobalIndexScopeServer {
		t.Fatalf("server.lua scope = %q, want server", got)
	}
	if got := res.ScriptScopes["shared.lua"]; got != GlobalIndexScopeShared {
		t.Fatalf("shared.lua scope = %q, want shared", got)
	}
}

func TestTopologicalSort(t *testing.T) {
	idx := NewGlobalIndexV2()
	idx.RegisterFiveMResource(&FiveMResource{Name: "c", RootURI: "c"}, true)
	idx.RegisterFiveMResource(&FiveMResource{Name: "b", RootURI: "b", Dependencies: []string{"c"}}, true)
	idx.RegisterFiveMResource(&FiveMResource{Name: "a", RootURI: "a", Dependencies: []string{"b"}}, true)

	ordered, diags := idx.TopologicalSort()
	if len(diags) != 0 {
		t.Fatalf("topological sort diagnostics = %+v, want none", diags)
	}
	if !slices.Equal(ordered, []ResourceURI{"c", "b", "a"}) {
		t.Fatalf("topological order = %#v, want c, b, a", ordered)
	}

	a := idx.Resources["a"]
	if !slices.Equal(a.Dependencies, []ResourceURI{"b"}) {
		t.Fatalf("a dependencies = %#v, want b", a.Dependencies)
	}
	c := idx.Resources["c"]
	if !slices.Equal(c.Dependents, []ResourceURI{"b"}) {
		t.Fatalf("c dependents = %#v, want b", c.Dependents)
	}
}

func TestEviction(t *testing.T) {
	idx := NewGlobalIndexV2()
	idx.AddSymbol("file:///evict", GlobalIndexScopeShared, "KeepMe", &SymbolEntry{Type: Type{Primitive: TypeBoolean}})
	idx.SetSource("file:///evict", []byte("KeepMe = true\n"), ast.NewTree([]byte("KeepMe = true\n")))

	before := idx.MemoryUsage()
	if !idx.EvictSource("file:///evict") {
		t.Fatal("EvictSource returned false")
	}
	if idx.MemoryUsage() >= before {
		t.Fatalf("memory after eviction = %d, want less than %d", idx.MemoryUsage(), before)
	}
	if entry := idx.LookupByScope("file:///evict", GlobalIndexScopeShared, "KeepMe"); entry == nil || entry.Type.Primitive != TypeBoolean {
		t.Fatalf("symbol entry after eviction = %+v, want retained boolean metadata", entry)
	}
}
