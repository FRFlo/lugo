package lsp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func BenchmarkInferShadowedUnknownGsubChain(b *testing.B) {
	for _, steps := range []int{1, 2, 4, 8, 16, 32, 64, 128} {
		b.Run(fmt.Sprint(steps), func(b *testing.B) {
			var src strings.Builder
			src.WriteString("local name = unknown\n")
			for i := 0; i < steps; i++ {
				src.WriteString("local name = name:gsub('é', 'e')\n")
			}
			src.WriteString("return name\n")
			s := NewServer("gsub-bench")
			uri := "file:///tmp/gsub-bench.lua"
			s.updateDocument(uri, []byte(src.String()))
			doc := s.Documents[uri]
			id := lastReturnedIdentifier(b, doc)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				doc.TypeCache.Drop()
				doc.InferType(id)
			}
		})
	}
}

func TestInferShadowedUnknownGsubChainRemainsUnknown(t *testing.T) {
	var src strings.Builder
	src.WriteString("local name = unknown\n")
	for i := 0; i < 8; i++ {
		src.WriteString("local name = name:gsub('é', 'e')\n")
	}
	src.WriteString("return name\n")
	s := NewServer("gsub-chain-test")
	uri := "file:///tmp/gsub-chain-test.lua"
	s.updateDocument(uri, []byte(src.String()))
	doc := s.Documents[uri]
	got := doc.InferType(lastReturnedIdentifier(t, doc))
	if got.Basics != TypeUnknown || got.CustomName != "" || got.DeclNode != ast.InvalidNode || got.DeclURI != "" || got.MetaNode != ast.InvalidNode || got.MetaURI != "" || got.CallSig != "" {
		t.Fatalf("unknown gsub chain = %#v, want unresolved type", got)
	}
}

func lastReturnedIdentifier(tb testing.TB, doc *Document) ast.NodeID {
	tb.Helper()
	for id := len(doc.Tree.Nodes) - 1; id >= 0; id-- {
		node := doc.Tree.Nodes[id]
		if node.Kind != ast.KindReturn || node.Left == ast.InvalidNode {
			continue
		}
		expr := node.Left
		if doc.Tree.Nodes[expr].Kind == ast.KindExprList {
			expr = doc.Tree.ExtraList[doc.Tree.Nodes[expr].Extra]
		}
		if doc.Tree.Nodes[expr].Kind != ast.KindIdent {
			tb.Fatalf("return expression kind = %v, want identifier", doc.Tree.Nodes[expr].Kind)
		}
		return expr
	}
	tb.Fatal("return identifier not found")
	return ast.InvalidNode
}
