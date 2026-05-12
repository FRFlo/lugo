package lsp

import (
	"testing"

	"github.com/coalaura/lugo/semantic"
)

func TestParity_V1V2SymbolCountAndType(t *testing.T) {
	src := []byte("local x = 5\nlocal y = x + 1\n")

	legacyTree := parseResolverV2Lua(t, src)
	legacyResolver := semantic.New(legacyTree)
	legacyResolver.Resolve(legacyTree.Root)
	legacyDoc := &Document{Tree: legacyTree, Resolver: legacyResolver}

	v2Tree := parseResolverV2Lua(t, src)
	v2Resolver := NewResolverV2(v2Tree, ResolverV2Options{SemanticData: NewSemanticDataTable()})
	if err := v2Resolver.Resolve(v2Tree.Root); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	legacyCount := len(legacyResolver.LocalDefs) + len(legacyResolver.GlobalDefs)
	v2Count := len(v2Resolver.LocalDefs) + len(v2Resolver.GlobalDefs)
	if legacyCount != v2Count {
		t.Fatalf("symbol count = %d, want %d", v2Count, legacyCount)
	}
	if legacyCount != 2 {
		t.Fatalf("symbol count = %d, want 2", legacyCount)
	}

	legacyX := findIdentByOccurrence(t, legacyTree, "x", 1)
	if got := legacyDoc.InferType(legacyX); got.Basics != TypeNumber {
		t.Fatalf("v1 x type = %#v, want number", got)
	}

	v2X := findIdentByOccurrence(t, v2Tree, "x", 1)
	if got := v2Resolver.Data.Get(NodeID(v2X)); got == nil || got.Type.Primitive != TypeNumber {
		t.Fatalf("v2 x type = %#v, want number", got)
	}
}
