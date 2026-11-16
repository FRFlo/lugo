package lsp

import (
	"fmt"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestGetGlobalSymbolsTraversesMoreThanTenParents(t *testing.T) {
	s := NewServer("test")
	const uri = "file:///inheritance.lua"
	doc := &Document{URI: uri}
	s.Documents[uri] = doc

	for i := 0; i <= 11; i++ {
		name := fmt.Sprintf("Class%d", i)
		parent := ""
		if i < 11 {
			parent = fmt.Sprintf("Class%d", i+1)
		}
		s.GlobalIndex.AddSymbol(ResourceURI(uri), GlobalIndexScopeShared, SymbolName(name), &SymbolEntry{
			URI: uri, Parent: parent, Key: GlobalKey{PropHash: ast.HashBytes([]byte(name))},
		})
	}
	target := &SymbolEntry{URI: uri, Name: "Class11.deep", Key: GlobalKey{ReceiverHash: ast.HashBytes([]byte("Class11")), PropHash: ast.HashBytes([]byte("deep"))}}
	s.GlobalIndex.AddSymbol(ResourceURI(uri), GlobalIndexScopeShared, SymbolName("Class11.deep"), target)

	got, ok := s.getGlobalSymbols(doc, ast.HashBytes([]byte("Class0")), ast.HashBytes([]byte("deep")))
	if !ok || len(got) != 1 || got[0].Name != "Class11.deep" {
		t.Fatalf("deep inherited symbol = %+v, %v; want Class11.deep", got, ok)
	}
}
