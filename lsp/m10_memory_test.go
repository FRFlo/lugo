package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
)

func TestTrimSharedBuffersDropsOnlyOversizedCapacity(t *testing.T) {
	s := NewServer("test")
	s.diagBuf = make([]Diagnostic, 0, maxReusableBufferCapacity+1)
	s.semTokensBuf = make([]SemanticToken, 0, 8)
	s.sharedParser.Errors = make([]parser.ParseError, 0, maxReusableParserCapacity+1)

	s.trimSharedBuffers()

	if s.diagBuf != nil {
		t.Fatal("oversized diagnostic buffer was retained")
	}
	if cap(s.semTokensBuf) != 8 {
		t.Fatalf("normal semantic token capacity = %d, want 8", cap(s.semTokensBuf))
	}
	if s.sharedParser.Errors != nil {
		t.Fatal("oversized parser error buffer was retained")
	}
}

func TestEvictClosedDocumentCachesTrimsASTAndResolver(t *testing.T) {
	s := NewServer("test")
	tree := ast.NewTree([]byte("return 1"))
	tree.Nodes = make([]ast.Node, 1, maxReusableASTCapacity+1)
	resolver := semantic.New(tree)
	resolver.References = make([]ast.NodeID, 1, maxReusableASTCapacity+1)
	s.Documents["file:///closed.lua"] = &Document{
		Server:   s,
		URI:      "file:///closed.lua",
		Tree:     tree,
		Resolver: resolver,
	}

	evictClosedDocumentCaches(s)

	if tree.Source != nil {
		t.Fatal("closed document source was retained")
	}
	if cap(tree.Nodes) > maxReusableASTCapacity {
		t.Fatalf("AST node capacity = %d, still above limit", cap(tree.Nodes))
	}
	if cap(resolver.References) > maxReusableASTCapacity {
		t.Fatalf("resolver reference capacity = %d, still above limit", cap(resolver.References))
	}
}
