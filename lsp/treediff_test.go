package lsp

import (
	"reflect"
	"testing"

	"github.com/coalaura/lugo/ast"
)

func TestTreeDiffer_DiffIdenticalTrees(t *testing.T) {
	oldTree := testTree("ab", []ast.Node{
		{Start: 0, End: 2, Kind: ast.KindFile},
		{Start: 0, End: 1, Parent: 1, Kind: ast.KindIdent},
	})
	newTree := testTree("ab", []ast.Node{
		{Start: 0, End: 2, Kind: ast.KindFile},
		{Start: 0, End: 1, Parent: 1, Kind: ast.KindIdent},
	})

	got := TreeDiffer{}.Diff(oldTree, newTree)
	if !reflect.DeepEqual(got, TreeDiffResult{}) {
		t.Fatalf("expected empty diff, got %#v", got)
	}
}

func TestTreeDiffer_DiffSingleNodeChange(t *testing.T) {
	oldTree := testTree("ab", []ast.Node{
		{Start: 0, End: 2, Kind: ast.KindFile},
		{Start: 0, End: 1, Parent: 1, Kind: ast.KindIdent},
	})
	newTree := testTree("ab", []ast.Node{
		{Start: 0, End: 2, Kind: ast.KindFile},
		{Start: 0, End: 1, Parent: 0, Kind: ast.KindIdent},
	})

	got := TreeDiffer{}.Diff(oldTree, newTree)
	want := TreeDiffResult{Modified: []ast.NodeID{2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestTreeDiffer_DiffNodeAddition(t *testing.T) {
	oldTree := testTree("a", []ast.Node{
		{Start: 0, End: 1, Kind: ast.KindFile},
	})
	newTree := testTree("a", []ast.Node{
		{Start: 0, End: 1, Kind: ast.KindFile},
		{Start: 0, End: 1, Parent: 1, Kind: ast.KindIdent},
	})

	got := TreeDiffer{}.Diff(oldTree, newTree)
	want := TreeDiffResult{Added: []ast.NodeID{2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestTreeDiffer_DiffNodeRemoval(t *testing.T) {
	oldTree := testTree("a", []ast.Node{
		{Start: 0, End: 1, Kind: ast.KindFile},
		{Start: 0, End: 1, Parent: 1, Kind: ast.KindIdent},
	})
	newTree := testTree("a", []ast.Node{
		{Start: 0, End: 1, Kind: ast.KindFile},
	})

	got := TreeDiffer{}.Diff(oldTree, newTree)
	want := TreeDiffResult{Removed: []ast.NodeID{2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func testTree(src string, nodes []ast.Node) *ast.Tree {
	tree := &ast.Tree{Source: []byte(src), Root: 1, Nodes: make([]ast.Node, len(nodes)+1)}
	for i, node := range nodes {
		tree.Nodes[i+1] = node
	}
	return tree
}
