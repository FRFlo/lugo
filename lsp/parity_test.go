package lsp

import (
	"testing"

	"github.com/coalaura/lugo/semantic"
)

func TestParity_V1AndCurrentSymbolCountAndType(t *testing.T) {
	src := []byte("local x = 5\nlocal y = x + 1\n")

	legacyTree := parseResolverLua(t, src)
	legacyResolver := semantic.New(legacyTree)
	legacyResolver.Resolve(legacyTree.Root)
	legacyDoc := &Document{Tree: legacyTree, Resolver: legacyResolver}

	currentTree := parseResolverLua(t, src)
	resolver := NewResolver(currentTree, ResolverOptions{SemanticData: NewSemanticDataTable()})
	if err := resolver.Resolve(currentTree.Root); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	legacyCount := len(legacyResolver.LocalDefs) + len(legacyResolver.GlobalDefs)
	currentCount := len(resolver.LocalDefs) + len(resolver.GlobalDefs)
	if legacyCount != currentCount {
		t.Fatalf("symbol count = %d, want %d", currentCount, legacyCount)
	}
	if legacyCount != 2 {
		t.Fatalf("symbol count = %d, want 2", legacyCount)
	}

	legacyX := findIdentByOccurrence(t, legacyTree, "x", 1)
	if got := legacyDoc.InferType(legacyX); got.Basics != TypeNumber {
		t.Fatalf("v1 x type = %#v, want number", got)
	}

	currentX := findIdentByOccurrence(t, currentTree, "x", 1)
	if got := resolver.Data.Get(NodeID(currentX)); got == nil || got.Type.Primitive != TypeNumber {
		t.Fatalf("current x type = %#v, want number", got)
	}
}
