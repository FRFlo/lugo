package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
)

func TestLuaDocGenericCallUnifiesNestedParameterTypes(t *testing.T) {
	cases := []struct {
		name       string
		generic    string
		paramType  string
		returnType string
		argument   string
		wantBasic  BasicType
		wantCustom string
	}{
		{
			name:       "direct nested type",
			generic:    "T",
			paramType:  "Box<T>",
			returnType: "T",
			argument:   "Box<string>",
			wantBasic:  TypeString,
		},
		{
			name:       "recursively nested type",
			generic:    "T",
			paramType:  "Box<Container<T>>",
			returnType: "T",
			argument:   "Box<Container<number>>",
			wantBasic:  TypeNumber,
		},
		{
			name:       "union with optional branch",
			generic:    "T",
			paramType:  "Box<T> | nil",
			returnType: "T",
			argument:   "Box<boolean>",
			wantBasic:  TypeBoolean,
		},
		{
			name:       "multiple type parameters",
			generic:    "K, V",
			paramType:  "Pair<K, V>",
			returnType: "V",
			argument:   "Pair<string, number>",
			wantBasic:  TypeNumber,
		},
		{
			name:       "preserve aliased argument type",
			generic:    "T",
			paramType:  "Box<T>",
			returnType: "T",
			argument:   "Box<TextAlias>",
			wantCustom: "TextAlias",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := newIndexTestDocument(t, `---@generic `+tc.generic+`
---@param input `+tc.paramType+`
---@return `+tc.returnType+`
local function unwrap(input) return nil end
---@type `+tc.argument+`
local value = {}
local result = unwrap(value)
`)
			doc.Server = NewServer("test")
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
			if got.Basics != tc.wantBasic || got.CustomName != tc.wantCustom {
				t.Fatalf("inferred result = %+v, want basic %v custom %q", got, tc.wantBasic, tc.wantCustom)
			}
		})
	}
}

func TestLuaDocGenericMethodUnifiesNestedParameterType(t *testing.T) {
	doc := newIndexTestDocument(t, `local object = {}
---@generic T
---@param input Box<T>
---@return T
function object:unwrap(input) return nil end
---@type Box<boolean>
local value = {}
local result = object:unwrap(value)
`)
	doc.Server = NewServer("test")
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics != TypeBoolean {
		t.Fatalf("nested generic method result = %+v, want boolean", got)
	}
}

func BenchmarkLuaDocGenericNestedParameterInference(b *testing.B) {
	source := []byte(`---@generic T
---@param input Box<Container<T>>
---@return T
local function unwrap(input) return nil end
---@type Box<Container<number>>
local value = {}
local result = unwrap(value)
	`)
	tree := ast.NewTree(source)
	p := parser.New(source, tree, 0)
	p.Parse()
	if len(p.Errors) > 0 {
		b.Fatalf("parse errors = %#v", p.Errors)
	}
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///generic_nested_benchmark.lua"}
	doc.Server = NewServer("benchmark")
	resultID := ast.InvalidNode
	for id, node := range tree.Nodes {
		if node.Kind == ast.KindIdent && string(doc.nodeSource(ast.NodeID(id))) == "result" {
			resultID = ast.NodeID(id)
			break
		}
	}
	if resultID == ast.InvalidNode {
		b.Fatal("result identifier not found")
	}
	doc.InferType(resultID)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		doc.TypeCache.Drop()
		doc.Inferring = nil
		_ = doc.InferType(resultID)
	}
}
