package lsp

import (
	"fmt"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

// BenchmarkConditionalNewIndexInference measures field inference after N
// conditional writes of the same field are redirected through __newindex.
// Parsing and semantic resolution are intentionally outside the timed region.
func BenchmarkConditionalNewIndexInference(b *testing.B) {
	for _, writes := range []int{10, 50, 100, 200} {
		b.Run(fmt.Sprintf("writes_%d", writes), func(b *testing.B) {
			source := conditionalNewIndexBenchmarkSource(writes)
			doc := benchmarkMetatableDocument(b, []byte(source))
			member := benchmarkFindConditionalMember(doc.Tree, "answer")
			if member == ast.InvalidNode {
				b.Fatal("sink.answer read not found")
			}

			got := doc.InferType(member)
			want := TypeNumber | TypeString | TypeBoolean
			if got.Basics&want != want {
				b.Fatalf("inferred union = %#v, want number|string|boolean", got)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				// InferType caches AST results. Clear only this lookup so each
				// iteration exercises the newindex-write collection path.
				doc.TypeCache[member] = TypeSet{}
				_ = doc.InferType(member)
			}
		})
	}
}

func conditionalNewIndexBenchmarkSource(writes int) string {
	source := "local sink = {}\nlocal object = setmetatable({}, { __newindex = sink })\n"
	for i := 0; i < writes; i++ {
		var value string
		switch i % 3 {
		case 0:
			value = fmt.Sprintf("%d", i)
		case 1:
			value = fmt.Sprintf("%q", "value")
		default:
			value = "true"
		}
		source += fmt.Sprintf("if condition%d then object.answer = %s end\n", i, value)
	}
	source += "local result = sink.answer\n"
	return source
}

func benchmarkFindConditionalMember(tree *ast.Tree, name string) ast.NodeID {
	for id, node := range tree.Nodes {
		if id == 0 || node.Kind != ast.KindMemberExpr || node.Right == ast.InvalidNode {
			continue
		}
		if string(tree.Source[tree.Nodes[node.Right].Start:tree.Nodes[node.Right].End]) == name &&
			string(tree.Source[tree.Nodes[node.Left].Start:tree.Nodes[node.Left].End]) == "sink" {
			return ast.NodeID(id)
		}
	}
	return ast.InvalidNode
}
