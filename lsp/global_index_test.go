package lsp

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
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
		})

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

	t.Run("TypoSuggestionsMatchesVisibleSymbols", func(t *testing.T) {
		idx := NewGlobalIndex()
		idx.EnsureResource("app").Dependencies = []ResourceURI{"dep"}
		dep := idx.EnsureResource("dep")
		idx.AddSymbol("app", GlobalIndexScopeClient, "PlayerName", &SymbolEntry{})
		idx.AddSymbol("app", GlobalIndexScopeShared, "PlayerNme", &SymbolEntry{})
		idx.AddSymbol("app", GlobalIndexScopeServer, "PlayerNam", &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeClient, "playername", &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeShared, "PläyerName", &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeServer, "PlayerNane", &SymbolEntry{})
		// The candidate key and entry name are intentionally different. A direct
		// table representative must also be considered even without a bucket.
		idx.Resources["app"].Candidates[GlobalIndexScopeClient]["unrelated"] = []*SymbolEntry{{Name: "PlayerNme"}}
		idx.Resources["app"].Client["PlayerNmae"] = &SymbolEntry{Name: "PlayerName"}
		dep.Shared["direct"] = &SymbolEntry{Name: "PlayerNami"}
		idx.Resources["app"].Candidates[GlobalIndexScopeClient]["nil"] = []*SymbolEntry{nil}

		for _, tc := range []struct {
			name  SymbolName
			scope GlobalIndexScope
			max   int
		}{
			{name: "PlayerNane", scope: GlobalIndexScopeClient, max: 10},
			{name: "PlayerNane", scope: GlobalIndexScopeServer, max: 10},
			{name: "PlayerNane", scope: GlobalIndexScopeShared, max: 10},
			{name: "PlayerNane", scope: GlobalIndexScopeClient, max: 1},
			{name: "PlayerNane", scope: GlobalIndexScopeClient, max: 0},
			{name: "PläyerNmae", scope: GlobalIndexScopeClient, max: 10},
			{name: "", scope: GlobalIndexScopeClient, max: 10},
		} {
			got := idx.TypoSuggestions(tc.name, "app", tc.scope, tc.max)
			want := legacyTypoSuggestions(idx, tc.name, "app", tc.scope, tc.max)
			if !slices.Equal(got, want) {
				t.Errorf("TypoSuggestions(%q, %q, %d) = %#v, want %#v", tc.name, tc.scope, tc.max, got, want)
			}
		}
		mutable := &SymbolEntry{Name: "PlayerNema"}
		idx.Resources["app"].Shared["mutable-key"] = mutable
		assertEquivalent := func(label string) {
			t.Helper()
			got := idx.TypoSuggestions("PlayerNane", "app", GlobalIndexScopeClient, 10)
			want := legacyTypoSuggestions(idx, "PlayerNane", "app", GlobalIndexScopeClient, 10)
			if !slices.Equal(got, want) {
				t.Errorf("%s: TypoSuggestions = %#v, want %#v", label, got, want)
			}
		}
		assertEquivalent("direct table insertion")
		mutable.Name = "PlayerNamo"
		assertEquivalent("direct entry rename")
		delete(idx.Resources["app"].Shared, "mutable-key")
		assertEquivalent("direct table deletion")
		assertPanics := func(label string, fn func()) {
			t.Helper()
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic for a negative max", label)
				}
			}()
			fn()
		}
		assertPanics("TypoSuggestions", func() { idx.TypoSuggestions("PlayerNane", "app", GlobalIndexScopeClient, -1) })
		assertPanics("legacyTypoSuggestions", func() { legacyTypoSuggestions(idx, "PlayerNane", "app", GlobalIndexScopeClient, -1) })
	})
}

func legacyTypoSuggestions(idx *GlobalIndex, name SymbolName, fromResource ResourceURI, scope GlobalIndexScope, max int) []SymbolName {
	if idx == nil || name == "" || max == 0 {
		return nil
	}
	type suggestion struct {
		name SymbolName
		dist int
	}
	needle := strings.ToLower(string(name))
	seen := make(map[SymbolName]struct{})
	suggestions := make([]suggestion, 0, max)
	for _, entry := range idx.VisibleSymbols(fromResource, scope) {
		if entry == nil || entry.Name == "" || entry.Name == name {
			continue
		}
		if _, ok := seen[entry.Name]; ok {
			continue
		}
		seen[entry.Name] = struct{}{}
		candidate := strings.ToLower(string(entry.Name))
		threshold := 3
		if len(needle) > 8 {
			threshold = 4
		}
		dist := levenshteinFast(needle, candidate, threshold)
		if dist <= threshold {
			suggestions = append(suggestions, suggestion{name: entry.Name, dist: dist})
		}
	}
	slices.SortStableFunc(suggestions, func(a, b suggestion) int {
		if a.dist != b.dist {
			return a.dist - b.dist
		}
		return strings.Compare(string(a.name), string(b.name))
	})
	if max > 0 && len(suggestions) > max {
		suggestions = suggestions[:max]
	}
	out := make([]SymbolName, 0, len(suggestions))
	for _, suggestion := range suggestions {
		out = append(out, suggestion.name)
	}
	return out
}

func BenchmarkWorkspaceSymbols(b *testing.B) {
	idx := NewGlobalIndex()
	for i := range 10000 {
		name := SymbolName("GeneratedWorkspaceSymbol" + strconv.Itoa(i))
		idx.AddSymbol(ResourceURI("file:///workspace"), GlobalIndexScopeShared, name, &SymbolEntry{})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		idx.WorkspaceSymbols("gws", 100)
	}
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

	res := idx.RegisterFiveMResource(&FiveMResource{Name: "res", RootURI: "file:///res", Manifest: manifest})
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

func TestRegisterFiveMResourceUpdatesResourceEdgesIncrementally(t *testing.T) {
	const resourceCount = 64
	idx := NewGlobalIndex()

	// Register in reverse dependency order so each target is initially absent.
	// Later registrations must update only the affected reverse edges.
	for i := resourceCount - 1; i >= 0; i-- {
		name := ResourceURI("resource-" + strconv.Itoa(i))
		resource := &FiveMResource{Name: string(name), RootURI: string(name)}
		if i > 0 {
			resource.Dependencies = []string{"resource-" + strconv.Itoa(i-1)}
		}
		idx.RegisterFiveMResource(resource)
	}

	for i := 0; i < resourceCount; i++ {
		name := ResourceURI("resource-" + strconv.Itoa(i))
		scope := idx.Resources[name]
		if scope == nil {
			t.Fatalf("missing scope for %q", name)
		}
		if i > 0 && !slices.Equal(scope.Dependencies, []ResourceURI{ResourceURI("resource-" + strconv.Itoa(i-1))}) {
			t.Fatalf("%q dependencies = %#v", name, scope.Dependencies)
		}
		if i+1 < resourceCount && !slices.Equal(scope.Dependents, []ResourceURI{ResourceURI("resource-" + strconv.Itoa(i+1))}) {
			t.Fatalf("%q dependents = %#v", name, scope.Dependents)
		}
	}

	first, firstDiagnostics := idx.TopologicalSort()
	second, secondDiagnostics := idx.TopologicalSort()
	if len(firstDiagnostics) != 0 || len(secondDiagnostics) != 0 {
		t.Fatalf("topological sort diagnostics = %#v, %#v; want none", firstDiagnostics, secondDiagnostics)
	}
	if !slices.Equal(first, second) {
		t.Fatalf("topological sort was not deterministic: %#v then %#v", first, second)
	}
	for i, resource := range first {
		if want := ResourceURI("resource-" + strconv.Itoa(i)); resource != want {
			t.Fatalf("topological order = %#v, want ascending resources", first)
		}
	}

	idx.RegisterFiveMResource(&FiveMResource{Name: "resource-1", RootURI: "resource-1", Dependencies: []string{"resource-3"}})
	if got := idx.Resources["resource-0"].Dependents; len(got) != 0 {
		t.Fatalf("resource-0 dependents after re-register = %#v, want none", got)
	}
	if got := idx.Resources["resource-3"].Dependents; !slices.Contains(got, ResourceURI("resource-1")) {
		t.Fatalf("resource-3 dependents after re-register = %#v, want resource-1", got)
	}
}

func BenchmarkRegisterFiveMResourceScaling(b *testing.B) {
	for _, resourceCount := range []int{10, 100, 1000} {
		b.Run(strconv.Itoa(resourceCount), func(b *testing.B) {
			resources := make([]*FiveMResource, resourceCount)
			for i := range resources {
				name := "resource-" + strconv.Itoa(i)
				resources[i] = &FiveMResource{Name: name, RootURI: name}
				if i > 0 {
					resources[i].Dependencies = []string{"resource-" + strconv.Itoa(i-1)}
				}
			}

			b.ReportAllocs()
			for b.Loop() {
				idx := NewGlobalIndex()
				for _, resource := range resources {
					idx.RegisterFiveMResource(resource)
				}
			}
		})
	}
}

func TestTopologicalSort(t *testing.T) {
	idx := NewGlobalIndex()
	idx.RegisterFiveMResource(&FiveMResource{Name: "c", RootURI: "c"})
	idx.RegisterFiveMResource(&FiveMResource{Name: "b", RootURI: "b", Dependencies: []string{"c"}})
	idx.RegisterFiveMResource(&FiveMResource{Name: "a", RootURI: "a", Dependencies: []string{"b"}})

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

func TestGlobalIndexRetainsSameNameDefinitionsWithinSource(t *testing.T) {
	idx := NewGlobalIndex()
	key := GlobalKey{PropHash: 42}
	first := &SymbolEntry{Key: key, NodeID: ast.NodeID(1)}
	idx.AddSymbol("resource", GlobalIndexScopeShared, "value", first)
	second := &SymbolEntry{Key: key, NodeID: ast.NodeID(2)}
	idx.AddSymbol("resource", GlobalIndexScopeShared, "value", second)
	if got := idx.LookupByHash(key); len(got) != 2 || got[0].NodeID != first.NodeID || got[1].NodeID != second.NodeID {
		t.Fatalf("LookupByHash() = %#v, want both same-source definitions in NodeID order", got)
	}
}

func TestGlobalIndexMultiSourceViewsAndRemoval(t *testing.T) {
	idx := NewGlobalIndex()
	key := GlobalKey{ReceiverHash: 17, PropHash: 23}
	idx.AddSymbol("consumer", GlobalIndexScopeShared, "_U", &SymbolEntry{Name: "_U", URI: "file:///consumer.lua", NodeID: 1, Key: key})
	idx.AddSymbol("dependency", GlobalIndexScopeShared, "_U", &SymbolEntry{Name: "_U", URI: "file:///dependency.lua", NodeID: 2, Key: key})
	idx.Resources["consumer"].Dependencies = []ResourceURI{"dependency"}

	if got := idx.SymbolsByHash(key); len(got) != 2 {
		t.Fatalf("hash view returned %d candidates, want 2", len(got))
	}
	if got := idx.VisibleSymbols("consumer", GlobalIndexScopeClient); len(got) != 2 {
		t.Fatalf("visible view returned %d candidates, want 2", len(got))
	}
	if got := idx.VisibleSymbolsByKey("consumer", GlobalIndexScopeClient, key); len(got) != 2 {
		t.Fatalf("visible key view returned %d candidates, want 2", len(got))
	}
	if got := idx.SymbolsForReceivers(map[uint64]bool{17: true}); len(got) != 2 {
		t.Fatalf("receiver view returned %d candidates, want 2", len(got))
	}
	if got := idx.WorkspaceSymbols("_U", -1); len(got) != 2 {
		t.Fatalf("workspace view returned %d candidates, want 2", len(got))
	}
	allCount := 0
	for range idx.AllSymbols() {
		allCount++
	}
	if allCount != 2 {
		t.Fatalf("AllSymbols returned %d candidates, want 2", allCount)
	}

	server := &Server{GlobalIndex: idx}
	server.removeGlobalIndexDocumentSymbols("file:///consumer.lua")
	if got := idx.SymbolsByHash(key); len(got) != 1 || got[0].URI != "file:///dependency.lua" {
		t.Fatalf("hash view after removal = %+v, want surviving dependency", got)
	}
	if got := idx.LookupByScope("consumer", GlobalIndexScopeShared, "_U"); got != nil {
		t.Fatalf("removed consumer entry remained indexed: %+v", got)
	}
	if got := idx.LookupByScope("dependency", GlobalIndexScopeShared, "_U"); got == nil || got.URI != "file:///dependency.lua" {
		t.Fatalf("dependency survivor missing: %+v", got)
	}

	promotion := NewGlobalIndex()
	promotion.AddSymbol("resource", GlobalIndexScopeShared, "_U", &SymbolEntry{Name: "_U", URI: "file:///a.lua", NodeID: 1, Key: key})
	promotion.AddSymbol("resource", GlobalIndexScopeShared, "_U", &SymbolEntry{Name: "_U", URI: "file:///b.lua", NodeID: 2, Key: key})
	promoter := &Server{GlobalIndex: promotion}
	promoter.removeGlobalIndexDocumentSymbols("file:///a.lua")
	if got := promotion.LookupByScope("resource", GlobalIndexScopeShared, "_U"); got == nil || got.URI != "file:///b.lua" {
		t.Fatalf("representative after deletion = %+v, want promoted b.lua", got)
	}
	promoter.removeGlobalIndexDocumentSymbols("file:///b.lua")
	if got := promotion.SymbolsByHash(key); len(got) != 0 {
		t.Fatalf("hash candidates after final deletion = %+v, want none", got)
	}
}

func TestGlobalIndexKeepsDifferentKeysForSameNode(t *testing.T) {
	idx := NewGlobalIndex()
	first := &SymbolEntry{Name: "Alias", URI: "file:///a.lua", NodeID: 7, Key: GlobalKey{PropHash: 1}}
	second := &SymbolEntry{Name: "Alias", URI: "file:///a.lua", NodeID: 7, Key: GlobalKey{PropHash: 2}}
	idx.AddSymbol("resource", GlobalIndexScopeShared, "Alias", first)
	idx.AddSymbol("resource", GlobalIndexScopeShared, "Alias", second)
	if got := idx.SymbolsByHash(first.Key); len(got) != 1 || got[0] != first {
		t.Fatalf("first key candidates = %+v", got)
	}
	if got := idx.SymbolsByHash(second.Key); len(got) != 1 || got[0] != second {
		t.Fatalf("second key candidates = %+v", got)
	}
}

func TestGlobalIndexUnderscoreCollisionIsOrderIndependent(t *testing.T) {
	key := GlobalKey{PropHash: 0x554e44455253434f}
	for _, order := range [][]string{{"file:///zz_stub.lua", "file:///fixture.lua"}, {"file:///fixture.lua", "file:///zz_stub.lua"}} {
		s := NewServer("underscore-collision")
		s.GlobalIndex = NewGlobalIndex()
		for _, uri := range order {
			doc := &Document{Server: s, URI: uri, Path: uri, Tree: &ast.Tree{}}
			s.Documents[uri] = doc
			s.GlobalIndex.AddSymbol("resource", GlobalIndexScopeShared, "_U", &SymbolEntry{Name: "_U", URI: uri, NodeID: 1, Key: key})
		}
		entries := s.GlobalIndex.SymbolsByHash(key)
		if len(entries) != 2 {
			t.Fatalf("order %v retained %d _U definitions, want 2", order, len(entries))
		}
		visible := s.visibleGlobalSymbolsFromEntries(nil, entries, 1)
		if len(visible) != 1 || visible[0].URI != "file:///fixture.lua" {
			t.Fatalf("order %v selected _U definition %#v, want fixture", order, visible)
		}
	}
}

func TestVisibleGlobalCandidatesFilterBeforeLimit(t *testing.T) {
	s := NewServer("visible-collision")
	src := &Document{Server: s, URI: "file:///root/use.lua", FiveMProfile: FiveMExecutionProfile{Kind: FiveMProfileClient, ResourceRoot: "file:///root"}, FiveMProfileCached: true}
	hidden := &Document{Server: s, URI: "file:///root/a_server.lua", FiveMProfile: FiveMExecutionProfile{Kind: FiveMProfileServer, ResourceRoot: "file:///root"}, FiveMProfileCached: true}
	visible := &Document{Server: s, URI: "file:///root/z_client.lua", FiveMProfile: FiveMExecutionProfile{Kind: FiveMProfileClient, ResourceRoot: "file:///root"}, FiveMProfileCached: true}
	s.Documents[src.URI], s.Documents[hidden.URI], s.Documents[visible.URI] = src, hidden, visible
	entries := []*SymbolEntry{{Name: "call", URI: hidden.URI, NodeID: 1}, {Name: "call", URI: visible.URI, NodeID: 2}}
	got := s.visibleGlobalSymbolsFromEntries(src, entries, 1)
	if len(got) != 1 || got[0].URI != visible.URI {
		t.Fatalf("visible selection = %#v, want visible client candidate", got)
	}
}

func TestGlobalAliasSkipsInvisibleCandidateBeforeSelection(t *testing.T) {
	s := NewServer("visible-alias")
	for uri, source := range map[string]string{
		"file:///root/a_server.lua": "AliasClass = HiddenClass",
		"file:///root/z_client.lua": "AliasClass = VisibleClass",
	} {
		s.updateDocument(uri, []byte(source))
		profile := FiveMExecutionProfile{Kind: FiveMProfileServer, ResourceRoot: "file:///root"}
		if uri == "file:///root/z_client.lua" {
			profile.Kind = FiveMProfileClient
		}
		doc := s.Documents[uri]
		doc.FiveMProfile, doc.FiveMProfileCached = profile, true
	}
	source := &Document{Server: s, URI: "file:///root/use.lua", FiveMProfile: FiveMExecutionProfile{Kind: FiveMProfileClient, ResourceRoot: "file:///root"}, FiveMProfileCached: true}
	s.Documents[source.URI] = source
	want := ast.HashBytes([]byte("VisibleClass"))
	if got := s.getGlobalAlias(source, ast.HashBytes([]byte("AliasClass"))); got != want {
		t.Fatalf("visible alias = %#x, want %#x", got, want)
	}
}
