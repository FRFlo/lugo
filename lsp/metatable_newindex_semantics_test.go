package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestNewIndexExistingFieldWritesStayOnReceiver(t *testing.T) {
	doc := newIndexTestDocument(t, `local o={}
o.answer=1
local sink={}
setmetatable(o,{__newindex=sink})
o.answer='updated'
local result=o.answer
local fallback=sink.answer
`)
	result := inferMemberForReceiver(t, doc, "o", "answer")
	if result.Basics&TypeString == 0 || result.Basics&TypeNumber != 0 {
		t.Fatalf("receiver answer = %#v, want updated string only", result)
	}
	fallback := inferMemberForReceiver(t, doc, "sink", "answer")
	if fallback.Basics&(TypeNumber|TypeString) != 0 {
		t.Fatalf("sink answer = %#v, want no redirected writes", fallback)
	}
}

func TestNewIndexNilAndShadowingSemantics(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want TypeSet
	}{
		{
			name: "first nil routes",
			src: `local sink={}
local o={answer=nil}
setmetatable(o,{__newindex=sink})
o.answer=1
local result=sink.answer
`,
			want: TypeSet{Basics: TypeNumber},
		},
		{
			name: "nil deletion routes subsequent write",
			src: `local sink={}
local o={answer=1}
setmetatable(o,{__newindex=sink})
o.answer=nil
o.answer='routed'
local result=sink.answer
`,
			want: TypeSet{Basics: TypeString},
		},
		{
			name: "shadowed receiver does not route outer write",
			src: `local sink={}
local o={}
setmetatable(o,{__newindex=sink})
do
  local o={answer=1}
  o.answer='inner'
end
o.answer=2
local result=sink.answer
`,
			want: TypeSet{Basics: TypeNumber},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := newIndexTestDocument(t, tt.src)
			got := inferMemberForReceiver(t, doc, "sink", "answer")
			if got.Basics&tt.want.Basics != tt.want.Basics {
				t.Fatalf("sink answer = %#v, want basics %#v", got, tt.want)
			}
		})
	}
}

func inferMemberForReceiver(t *testing.T, doc *Document, receiver, name string) TypeSet {
	t.Helper()
	for id := len(doc.Tree.Nodes) - 1; id > 0; id-- {
		node := doc.Tree.Nodes[id]
		if node.Kind == ast.KindMemberExpr && node.Right != ast.InvalidNode && string(doc.nodeSource(node.Right)) == name && string(doc.nodeSource(node.Left)) == receiver {
			return doc.InferType(ast.NodeID(id))
		}
	}
	t.Fatalf("member %s.%s not found", receiver, name)
	return TypeSet{}
}

func TestNewIndexSequentialWritesUseLatestValue(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = {}
local object = setmetatable({}, { __newindex = sink })
object.answer = 1
object.answer = "updated"
local result = sink.answer
`)
	got := inferMemberForReceiver(t, doc, "sink", "answer")
	if got.Basics != TypeString {
		t.Fatalf("sequential writes = %#v, want latest string", got)
	}
}

func TestNewIndexNilFieldContinuesFallbackChain(t *testing.T) {
	doc := newIndexTestDocument(t, `local storage = {}
local sink = setmetatable({answer=nil}, {__newindex=storage})
local object = setmetatable({}, {__newindex=sink})
object.answer = 42
local result = storage.answer
`)
	got := inferMemberForReceiver(t, doc, "storage", "answer")
	if got.Basics != TypeNumber {
		t.Fatalf("storage.answer = %#v, want redirected number", got)
	}
}

func TestNewIndexNilAssignmentContinuesFallbackChainInSourceOrder(t *testing.T) {
	doc := newIndexTestDocument(t, `local storage = {}
local sink = {answer="initial"}
setmetatable(sink, {__newindex=storage})
local object = setmetatable({}, {__newindex=sink})
object.answer = 1
sink.answer = nil
object.answer = "after nil"
local result = storage.answer
`)
	got := inferMemberForReceiver(t, doc, "storage", "answer")
	if got.Basics != TypeString {
		t.Fatalf("storage.answer = %#v, want only post-nil string", got)
	}
}

func TestNewIndexConditionalNilOwnFieldContinuesFallbackChain(t *testing.T) {
	doc := newIndexTestDocument(t, `local storage = {}
local sink = {answer="initial"}
setmetatable(sink, {__newindex=storage})
local object = setmetatable({}, {__newindex=sink})
if condition then sink.answer = nil end
object.answer = 42
local result = storage.answer
`)
	got := inferMemberForReceiver(t, doc, "storage", "answer")
	if got.Basics&TypeNumber == 0 {
		t.Fatalf("storage.answer = %#v, want number from conditional nil path", got)
	}
}

func TestNewIndexUnconditionalWriteAfterConditionalNilRestoresPresence(t *testing.T) {
	doc := newIndexTestDocument(t, `local storage = {}
local sink = {answer="initial"}
setmetatable(sink, {__newindex=storage})
local object = setmetatable({}, {__newindex=sink})
if condition then sink.answer = nil end
sink.answer = "reset"
object.answer = 42
local result = storage.answer
`)
	got := inferMemberForReceiver(t, doc, "storage", "answer")
	if got.Basics&TypeNumber != 0 {
		t.Fatalf("storage.answer = %#v, unconditional reset should block routing", got)
	}
}

func TestNewIndexNilFallbackCycleTerminatesWithoutInventingWrite(t *testing.T) {
	doc := newIndexTestDocument(t, `local storage = {}
local first = {}
local second = {}
setmetatable(first, {__newindex=second})
setmetatable(second, {__newindex=first})
second.answer = nil
local object = setmetatable({}, {__newindex=first})
object.answer = true
local result = storage.answer
`)
	got := inferMemberForReceiver(t, doc, "storage", "answer")
	if got.Basics&TypeBoolean != 0 {
		t.Fatalf("storage.answer = %#v, cyclic fallback invented a write", got)
	}
}

func BenchmarkNewIndexNilFallbackChain(b *testing.B) {
	doc := benchmarkMetatableDocument(b, []byte(`local storage = {}
local sink = {answer="initial"}
setmetatable(sink, {__newindex=storage})
local object = setmetatable({}, {__newindex=sink})
object.answer = 1
sink.answer = nil
object.answer = "after nil"
local result = storage.answer
`))
	server := NewServer("benchmark")
	doc.Server = server
	doc.URI = "file:///benchmark/nil-newindex-chain.lua"
	server.Documents[doc.URI] = doc
	var member ast.NodeID
	for id, node := range doc.Tree.Nodes {
		if id != 0 && node.Kind == ast.KindMemberExpr && string(doc.nodeSource(node.Left)) == "storage" && string(doc.nodeSource(node.Right)) == "answer" {
			member = ast.NodeID(id)
			break
		}
	}
	if member == ast.InvalidNode {
		b.Fatal("storage.answer read not found")
	}
	if got := doc.InferType(member); got.Basics != TypeString {
		b.Fatalf("storage.answer = %#v, want string", got)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		doc.TypeCache[member] = TypeSet{}
		_ = doc.InferType(member)
	}
}
