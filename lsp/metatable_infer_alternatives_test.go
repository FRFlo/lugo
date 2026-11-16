package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestInferMetatableIndexConditionalTableFunctionAlternatives(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
	}{
		{
			name: "function alternative",
			source: `local proto = { value = 42 }
local mt = { __index = proto }
local object = setmetatable({}, mt)
if flag then mt.__index = function(_, key) return "fallback" end end
local result = object.value
`,
		},
		{
			name: "table alternative",
			source: `local mt = { __index = function(_, key) return "fallback" end }
local object = setmetatable({}, mt)
if flag then mt.__index = { value = 42 } end
local result = object.value
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := newIndexTestDocument(t, tc.source)
			got := inferLastNamedMember(t, doc, "value")
			if got.Basics&(TypeNumber|TypeString) != TypeNumber|TypeString {
				t.Fatalf("conditional table/function __index type = %#v, want number|string", got)
			}
		})
	}
}

func BenchmarkInferMetatableIndexConditionalTableFunctionAlternatives(b *testing.B) {
	doc := benchmarkMetatableDocument(b, []byte(`local proto = { value = 42 }
local mt = { __index = proto }
local object = setmetatable({}, mt)
if flag then mt.__index = function(_, key) return "fallback" end end
local result = object.value
`))
	member := ast.InvalidNode
	for id, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindMemberExpr && string(doc.nodeSource(node.Left)) == "object" && string(doc.nodeSource(node.Right)) == "value" {
			member = ast.NodeID(id)
		}
	}
	if member == ast.InvalidNode {
		b.Fatal("object.value read not found")
	}
	_ = doc.InferType(member)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		doc.TypeCache[member] = TypeSet{}
		_ = doc.InferType(member)
	}
}
