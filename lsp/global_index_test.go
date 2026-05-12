package lsp

import (
	"slices"
	"testing"

	"github.com/coalaura/lugo/ast"
)

func TestGlobalIndex(t *testing.T) {
	t.Run("Lookup", func(t *testing.T) {
		idx := NewGlobalIndex()
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
		idx := NewGlobalIndex()
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
		idx := NewGlobalIndex()
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
		idx := NewGlobalIndex()
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
		idx := NewGlobalIndex()
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
		idx := NewGlobalIndex(8)
		idx.SetSource("file:///old", []byte("old source"), nil)
		idx.SetSource("file:///new", []byte("new source"), nil)

		if idx.MemoryUsage() > idx.MaxMemory {
			t.Fatalf("MemoryUsage = %d, want <= budget %d", idx.MemoryUsage(), idx.MaxMemory)
		}
		if idx.Resources["file:///old"].Source != nil {
			t.Fatal("oldest source should be evicted first")
		}
	})

	t.Run("AllSymbols", func(t *testing.T) {
		idx := NewGlobalIndex()
		idx.AddSymbol("file:///a", GlobalIndexScopeClient, "ClientA", &SymbolEntry{})
		idx.AddSymbol("file:///a", GlobalIndexScopeShared, "SharedA", &SymbolEntry{})
		idx.AddSymbol("file:///b", GlobalIndexScopeServer, "ServerB", &SymbolEntry{})

		got := make(map[ResourceURI][]SymbolName)
		for uri, entry := range idx.AllSymbols() {
			got[uri] = append(got[uri], entry.Name)
		}

		if !slices.Equal(got["file:///a"], []SymbolName{"ClientA", "SharedA"}) {
			t.Fatalf("file:///a symbols = %#v, want ClientA and SharedA", got["file:///a"])
		}
		if !slices.Equal(got["file:///b"], []SymbolName{"ServerB"}) {
			t.Fatalf("file:///b symbols = %#v, want ServerB", got["file:///b"])
		}
	})

	t.Run("SymbolsByHash", func(t *testing.T) {
		idx := NewGlobalIndex()
		key := GlobalKey{PropHash: 42}
		first := idx.AddSymbol("file:///a", GlobalIndexScopeShared, "First", &SymbolEntry{Key: key})
		second := idx.AddSymbol("file:///b", GlobalIndexScopeShared, "Second", &SymbolEntry{Key: key})

		got := idx.SymbolsByHash(key)
		if len(got) != 2 || got[0] != first || got[1] != second {
			t.Fatalf("SymbolsByHash = %#v, want original entries in hash index order", got)
		}
	})

	t.Run("VisibleSymbolsWithDependencies", func(t *testing.T) {
		idx := NewGlobalIndex()
		idx.EnsureResource("app").Dependencies = []ResourceURI{"dep"}
		idx.EnsureResource("dep")
		idx.AddSymbol("app", GlobalIndexScopeClient, "AppClient", &SymbolEntry{})
		idx.AddSymbol("app", GlobalIndexScopeServer, "AppServer", &SymbolEntry{})
		idx.AddSymbol("app", GlobalIndexScopeShared, "AppShared", &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeClient, "DepClient", &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeServer, "DepServer", &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeShared, "DepShared", &SymbolEntry{})

		visible := idx.VisibleSymbols("app", GlobalIndexScopeClient)
		got := symbolEntryNames(visible)
		want := []SymbolName{"AppClient", "AppShared", "DepClient", "DepShared"}
		if !slices.Equal(got, want) {
			t.Fatalf("VisibleSymbols client = %#v, want %#v", got, want)
		}
		if slices.Contains(got, SymbolName("AppServer")) || slices.Contains(got, SymbolName("DepServer")) {
			t.Fatalf("VisibleSymbols leaked server symbols: %#v", got)
		}
	})

	t.Run("WorkspaceSymbols", func(t *testing.T) {
		idx := NewGlobalIndex()
		idx.AddSymbol("file:///a", GlobalIndexScopeShared, "AlphaBeta", &SymbolEntry{})
		idx.AddSymbol("file:///a", GlobalIndexScopeShared, "AlphaGamma", &SymbolEntry{})
		idx.AddSymbol("file:///a", GlobalIndexScopeShared, "BetaOnly", &SymbolEntry{})

		got := symbolEntryNames(idx.WorkspaceSymbols("AB", 2))
		want := []SymbolName{"AlphaBeta"}
		if !slices.Equal(got, want) {
			t.Fatalf("WorkspaceSymbols fuzzy = %#v, want %#v", got, want)
		}

		got = symbolEntryNames(idx.WorkspaceSymbols("Alpha", 1))
		want = []SymbolName{"AlphaBeta"}
		if !slices.Equal(got, want) {
			t.Fatalf("WorkspaceSymbols limit = %#v, want %#v", got, want)
		}
	})

	t.Run("TypoSuggestions", func(t *testing.T) {
		idx := NewGlobalIndex()
		idx.EnsureResource("app").Dependencies = []ResourceURI{"dep"}
		idx.EnsureResource("dep")
		idx.AddSymbol("app", GlobalIndexScopeClient, "PlayerName", &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeShared, "PlayerCount", &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeServer, "ServerSecret", &SymbolEntry{})

		got := idx.TypoSuggestions("PlayerNane", "app", GlobalIndexScopeClient, 2)
		want := []SymbolName{"PlayerName", "PlayerCount"}
		if !slices.Equal(got, want) {
			t.Fatalf("TypoSuggestions = %#v, want %#v", got, want)
		}
		if slices.Contains(got, SymbolName("ServerSecret")) {
			t.Fatalf("TypoSuggestions included invisible server symbol: %#v", got)
		}
	})
}

func symbolEntryNames(entries []*SymbolEntry) []SymbolName {
	names := make([]SymbolName, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			names = append(names, entry.Name)
		}
	}

	return names
}

func TestDependencyCycle(t *testing.T) {
	idx := NewGlobalIndex()
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
	idx := NewGlobalIndex()
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
	idx := NewGlobalIndex()
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
	idx := NewGlobalIndex()
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
