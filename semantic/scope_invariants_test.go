package semantic_test

import (
	"slices"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
)

func TestScopeResolutionBoundaries(t *testing.T) {
	// Identifiers are ordered by source position, not arena insertion order.
	for _, tc := range []struct {
		name, source string
		want         []int // definition index for each occurrence; -1 means global
	}{
		{"block restores outer", "local x = 1\ndo local x = 2; print(x) end\nprint(x)", []int{0, 1, 1, 0}},
		{"sibling blocks", "do local x = 1; print(x) end\ndo local x = 2; print(x) end", []int{0, 0, 2, 2}},
		{"local not visible before declaration", "print(x)\nlocal x = 1\nprint(x)", []int{-1, 1, 1}},
		{"function parameter does not escape", "local function f(x) return x end\nprint(x)", []int{0, 0, -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			tree := ast.NewTree(src)
			p := parser.New(src, tree, 0)
			root := p.Parse()
			if len(p.Errors) > 0 {
				t.Fatalf("parse errors: %v", p.Errors)
			}
			r := semantic.New(tree)
			r.Resolve(root)
			ids := findIdents(t, tree, "x")
			slices.SortFunc(ids, func(a, b ast.NodeID) int {
				return int(tree.Nodes[a].Start) - int(tree.Nodes[b].Start)
			})
			if len(ids) != len(tc.want) {
				t.Fatalf("found %d x identifiers, want %d", len(ids), len(tc.want))
			}
			for i, want := range tc.want {
				def := ast.InvalidNode
				if want >= 0 {
					def = ids[want]
				}
				if got := r.References[ids[i]]; got != def {
					t.Errorf("occurrence %d resolved to %d, want %d", i, got, def)
				}
			}
		})
	}
}

func TestResolverResetClearsPreviousDocument(t *testing.T) {
	tree := ast.NewTree(nil)
	p := parser.New(nil, tree, 0)
	r := semantic.New(tree)
	for _, tc := range []struct {
		name, source string
		wantRefs     int
		wantGlobals  int
	}{
		{"local and global", "local x = 1\nprint(x)", 1, 1},
		{"empty", "", 0, 0},
		{"global only", "print(x)", 0, 2},
		{"local again", "local x = 2\nprint(x)", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			tree.Reset(src)
			p.Reset(src, tree)
			root := p.Parse()
			if len(p.Errors) > 0 {
				t.Fatalf("parse errors: %v", p.Errors)
			}
			r.Reset()
			r.Resolve(root)
			if len(r.References) != len(tree.Nodes) || len(r.LocalDefs) != tc.wantRefs || len(r.GlobalRefs) != tc.wantGlobals {
				t.Errorf("references length = %d, local defs = %d, global refs = %d; want %d, %d, %d", len(r.References), len(r.LocalDefs), len(r.GlobalRefs), len(tree.Nodes), tc.wantRefs, tc.wantGlobals)
			}
			if tc.wantRefs == 0 {
				for i, def := range r.References {
					if def != ast.InvalidNode {
						t.Errorf("stale reference at node %d to %d", i, def)
					}
				}
			}
		})
	}
}
