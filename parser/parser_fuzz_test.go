package parser_test

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
)

func FuzzParserParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"local function f(x, ...) return {x = x, ...} end",
		"if a then b() elseif c then d() else e() end",
		"for k, v in pairs(t) do print(k, v) end",
		"\uFEFFCitizen.CreateThread(function() Wait(0) end)",
		"local x = [=[unterminated",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 1<<20 {
			t.Skip()
		}
		src := []byte(source)
		tree := ast.NewTree(src)
		p := parser.New(src, tree, 100)
		p.Parse()

		for i, node := range tree.Nodes {
			if node.Kind == ast.KindInvalid {
				continue
			}
			if node.Start > node.End || int(node.End) > len(src) {
				t.Fatalf("node %d has invalid bounds [%d,%d) for %d bytes", i, node.Start, node.End, len(src))
			}
		}
		for _, parseErr := range p.Errors {
			if parseErr.Start > parseErr.End || int(parseErr.End) > len(src) {
				t.Fatalf("parse error has invalid bounds [%d,%d) for %d bytes", parseErr.Start, parseErr.End, len(src))
			}
		}
	})
}
