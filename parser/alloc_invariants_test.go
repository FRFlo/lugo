package parser_test

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
)

func TestParserSteadyStateAllocations(t *testing.T) {
	// Match BenchmarkParser: reuse the same tree and parser for each parse.
	src := []byte(`
		local function fib(n)
			if n < 2 then return n end
			return fib(n-1) + fib(n-2)
		end
		local result = fib(10)
	`)
	tree := ast.NewTree(src)
	p := parser.New(src, tree, 0)
	parse := func() {
		tree.Reset(src)
		p.Reset(src, tree)
		p.Parse()
	}
	parse() // Allow backing arrays to grow before measuring steady state.
	if got := testing.AllocsPerRun(100, parse); got != 0 {
		t.Fatalf("parser allocations per parse = %g, want 0", got)
	}
	if len(p.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", p.Errors)
	}
}
