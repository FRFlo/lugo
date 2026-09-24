package parser_test

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
)

func TestParserResetAfterErrorsAndComments(t *testing.T) {
	cases := []struct {
		name, source string
		wantError    bool
		wantComments int
	}{
		{"invalid", "-- old\nlocal x =", true, 1},
		{"valid", "local y = 2", false, 0},
		{"invalid again", "local z =", true, 0},
		{"valid with comment", "-- new\nlocal a = 3", false, 1},
	}
	tree := ast.NewTree(nil)
	p := parser.New(nil, tree, 0)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.source)
			tree.Reset(src)
			p.Reset(src, tree)
			root := p.Parse()
			if (len(p.Errors) > 0) != tc.wantError {
				t.Errorf("errors = %v, want error = %v", p.Errors, tc.wantError)
			}
			if len(tree.Comments) != tc.wantComments {
				t.Errorf("comments = %d, want %d", len(tree.Comments), tc.wantComments)
			}
			if p.GetTree() != tree || root != tree.Root || tree.Nodes[root].End != uint32(len(src)) {
				t.Errorf("parser root/tree not associated with current source")
			}
			for _, err := range p.Errors {
				if err.End > uint32(len(src)) {
					t.Errorf("stale error position: %+v", err)
				}
			}
		})
	}
}
