package ast_test

import (
	"slices"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
)

func TestTreeReuseAcrossInputs(t *testing.T) {
	cases := []struct {
		name, source string
		lines        []uint32
		comments     int
		ident        string
	}{
		{"initial", "-- first\nlocal old = 1\n", []uint32{0, 9, 23}, 1, "old"},
		{"shorter", "local new = 2", []uint32{0}, 0, "new"},
		{"empty", "", []uint32{0}, 0, ""},
		{"trailing newline", "-- next\nlocal last = 3\n", []uint32{0, 8, 23}, 1, "last"},
	}
	tree := ast.NewTree(nil)
	p := parser.New(nil, tree, 0)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			tree.Reset(src)
			p.Reset(src, tree)
			root := p.Parse()
			if len(p.Errors) != 0 {
				t.Fatalf("parse errors: %v", p.Errors)
			}
			if tree.Root != root || root == ast.InvalidNode || tree.Nodes[root].End != uint32(len(src)) {
				t.Fatalf("root %d does not cover current input", root)
			}
			if !slices.Equal(tree.LineOffsets, tc.lines) || len(tree.Comments) != tc.comments {
				t.Fatalf("lines = %v, comments = %d; want %v, %d", tree.LineOffsets, len(tree.Comments), tc.lines, tc.comments)
			}
			found := false
			for _, node := range tree.Nodes[1:] {
				if node.End > uint32(len(src)) {
					t.Fatalf("stale node extends past source: %+v", node)
				}
				if node.Kind == ast.KindIdent && string(src[node.Start:node.End]) == tc.ident {
					found = true
				}
			}
			if tc.ident != "" && !found {
				t.Errorf("missing identifier %q", tc.ident)
			}
		})
	}
}
