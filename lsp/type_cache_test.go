package lsp

import (
	"testing"
	"unsafe"

	"github.com/FRFlo/lugo/ast"
)

func TestTypeCacheLifecycle(t *testing.T) {
	c := typeCache{nodes: 4}
	custom := TypeSet{Basics: TypeTable, CustomName: "Player", DeclURI: "file:///player.lua", DeclNode: 2, MetaURI: "file:///meta.lua", MetaNode: 3}
	c.Put(1, custom)
	if got := c.Get(1); got.CustomName != "Player" || got.DeclURI != custom.DeclURI || got.MetaURI != custom.MetaURI || got.MetaNode != 3 {
		t.Fatalf("custom/metatable result lost: %#v", got)
	}

	call := TypeSet{Basics: TypeFunction, CallURI: "file:///call.lua", CallNode: 2, CallSig: "(x: number)", CallParams: []LuaDocParam{{Name: "x"}}, CallRet: []LuaDocReturn{{Type: "string"}}}
	c.Put(2, call)
	if got := c.Get(2); got.CallSig != call.CallSig || len(got.CallParams) != 1 || len(got.CallRet) != 1 {
		t.Fatalf("call metadata lost: %#v", got)
	}

	// MetaURI alone is deliberately not a reusable result.
	c.Put(3, TypeSet{MetaURI: "file:///meta-only.lua"})
	if got := c.Get(3); reusableType(got) || got.MetaURI != "" {
		t.Fatalf("invalid cache entry retained metadata-only TypeSet: %#v", got)
	}

	c.Reset(2) // shrink, clear slots, and release references/capacity from the old tree
	if got := c.Get(1); reusableType(got) || reusableType(c.Get(2)) {
		t.Fatalf("stale result after reset: %#v / %#v", got, c.Get(2))
	}
	if len(c.values) != 0 {
		t.Fatalf("reset retained %d TypeSets", len(c.values))
	}
	c.Put(0, TypeSet{Basics: TypeNumber})
	if got := c.Get(0); got.Basics != TypeNumber {
		t.Fatalf("cache did not accept new-tree result: %#v", got)
	}
	c.Drop()
	if !c.Empty() || len(c.values) != 0 || len(c.slots) != 0 {
		t.Fatalf("drop retained cache storage: %#v", c)
	}
}

func TestTypeCacheResetBoundsLargeToSmallCapacity(t *testing.T) {
	c := typeCache{nodes: 32_000}
	for id := ast.NodeID(1); id < 32_000; id++ {
		c.Put(id, TypeSet{Basics: TypeNumber})
	}
	if c.Empty() || len(c.values) != 31_999 {
		t.Fatalf("high-density cache has %d values", len(c.values))
	}
	compactBytes := uint64(cap(c.slots))*4 + uint64(cap(c.values))*uint64(unsafe.Sizeof(TypeSet{}))
	denseBytes := uint64(c.nodes) * uint64(unsafe.Sizeof(TypeSet{}))
	if cap(c.values) > c.nodes || compactBytes*400 > denseBytes*410 {
		t.Fatalf("high-density storage exceeds 2.5%% dense overhead: compact=%d dense=%d value_cap=%d nodes=%d", compactBytes, denseBytes, cap(c.values), c.nodes)
	}
	c.Reset(8)
	if cap(c.slots) != 0 || cap(c.values) != 0 {
		t.Fatalf("large backing arrays retained after shrink: slots=%d values=%d", cap(c.slots), cap(c.values))
	}
	if c.nodes != 8 || reusableType(c.Get(1)) {
		t.Fatalf("reset did not initialize the smaller tree: %#v", c)
	}
}

func TestInferTypeCacheCyclesEditsAndEviction(t *testing.T) {
	cases := []string{
		"local a,b; a=b; b=a; return a,b",
		"local t={}; t.self=t; return t.self.self",
		"local function f() return f() end; return f()",
		"local f,g; f=function() return g() end; g=function() return f() end; return f()",
		"local unknown; return unknown.missing",
	}
	for _, source := range cases {
		t.Run(source, func(t *testing.T) {
			s := NewServer("type-cache-test")
			uri := "file:///tmp/type-cache-test.lua"
			s.updateDocument(uri, []byte(source))
			doc := s.Documents[uri]
			inferAll := func() {
				t.Helper()
				for id := range doc.Tree.Nodes {
					doc.InferType(ast.NodeID(id))
				}
				for id, active := range doc.Inferring {
					if active {
						t.Fatalf("Inferring[%d] remained set", id)
					}
				}
			}
			inferAll()
			if len(doc.TypeCache.values) == 0 && source != cases[len(cases)-1] {
				t.Fatal("inference produced no reusable cache entries")
			}
			// A computed unknown is intentionally absent and must be retried.
			if source == cases[len(cases)-1] {
				for id, node := range doc.Tree.Nodes {
					if node.Kind == ast.KindMemberExpr && reusableType(doc.TypeCache.Get(ast.NodeID(id))) {
						t.Fatalf("unknown member %d was negatively cached", id)
					}
				}
			}
			largeTreeNodes := len(doc.Tree.Nodes)
			s.updateDocument(uri, []byte(source+"\nlocal added = 42; return added"))
			doc = s.Documents[uri]
			if len(doc.Tree.Nodes) <= largeTreeNodes {
				t.Fatal("edit did not grow the AST")
			}
			inferAll()
			s.updateDocument(uri, []byte("return 1"))
			doc = s.Documents[uri]
			if len(doc.Tree.Nodes) >= largeTreeNodes {
				t.Fatal("edit did not shrink the AST")
			}
			inferAll()
			doc.TypeCache.Drop()
			if !doc.TypeCache.Empty() {
				t.Fatal("eviction did not drop the type cache")
			}
			inferAll() // first access after eviction must reconstruct a cache
			if doc.TypeCache.Empty() {
				t.Fatal("cache was not rebuilt after eviction")
			}
		})
	}
}

func TestInferTypeCacheCrossDocumentCycle(t *testing.T) {
	s := NewServer("type-cache-cross-document-test")
	s.updateDocument("file:///tmp/type-cache-a.lua", []byte("function a() return b() end; return a()"))
	s.updateDocument("file:///tmp/type-cache-b.lua", []byte("function b() return a() end"))

	for _, uri := range []string{"file:///tmp/type-cache-a.lua", "file:///tmp/type-cache-b.lua"} {
		doc := s.Documents[uri]
		for id := range doc.Tree.Nodes {
			doc.InferType(ast.NodeID(id))
		}
		for id, active := range doc.Inferring {
			if active {
				t.Fatalf("%s: Inferring[%d] remained set after cross-document recursion", uri, id)
			}
		}
		if len(doc.TypeCache.values) == 0 {
			t.Fatalf("%s: cross-document inference produced no reusable entries", uri)
		}
	}
}
