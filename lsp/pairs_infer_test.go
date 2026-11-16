package lsp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
)

func TestInferPairsLoopVariablesFromMetamethod(t *testing.T) {
	src := []byte(`local source = setmetatable({}, { __pairs = function()
  return function() return 'key', 42 end
end })
for k, v in pairs(source) do
  local key = k
  local value = v
end
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	if got := inferNamedLocal(t, doc, "k"); got.Basics&TypeString == 0 {
		t.Fatalf("k = %#v, want string", got)
	}
	if got := inferNamedLocal(t, doc, "v"); got.Basics&TypeNumber == 0 {
		t.Fatalf("v = %#v, want number", got)
	}
}

func TestInferPairsLoopVariablesUnionAcrossBranches(t *testing.T) {
	src := []byte(`local flag = true
local source = setmetatable({}, { __pairs = function()
  if flag then return function() return 'key', 42 end end
  return function() return 1, 'value' end
end })
for k, v in pairs(source) do local key = k; local value = v end`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	if got := inferNamedLocal(t, doc, "k"); got.Basics&(TypeString|TypeNumber) != TypeString|TypeNumber {
		t.Fatalf("k = %#v, want string|number", got)
	}
	if got := inferNamedLocal(t, doc, "v"); got.Basics&(TypeString|TypeNumber) != TypeString|TypeNumber {
		t.Fatalf("v = %#v, want number|string", got)
	}
	// The second lookup must preserve the union through InferType's cache.
	if got := inferNamedLocal(t, doc, "k"); got.Basics&(TypeString|TypeNumber) != TypeString|TypeNumber {
		t.Fatalf("cached k = %#v, want string|number", got)
	}
}

func TestInferPairsExpandsIteratorCallReturns(t *testing.T) {
	src := []byte(`local function nextPair() return 'key', 42 end
local source = setmetatable({}, { __pairs = function()
  return function() return nextPair() end
end })
for k, v in pairs(source) do local key = k; local value = v end`)
	doc := newPairsInferenceTestDocument(t, src)
	if got := inferNamedLocal(t, doc, "k"); got.Basics&TypeString == 0 {
		t.Fatalf("k = %#v, want string", got)
	}
	if got := inferNamedLocal(t, doc, "v"); got.Basics&TypeNumber == 0 {
		t.Fatalf("v = %#v, want number", got)
	}
}

func TestInferPairsExpandsFactoryCallReturns(t *testing.T) {
	src := []byte(`local function makeIterator()
  return function() return 'key', 42 end
end
local source = setmetatable({}, { __pairs = function() return makeIterator() end })
for k, v in pairs(source) do local key = k; local value = v end`)
	doc := newPairsInferenceTestDocument(t, src)
	if got := inferNamedLocal(t, doc, "k"); got.Basics&TypeString == 0 {
		t.Fatalf("k = %#v, want string", got)
	}
	if got := inferNamedLocal(t, doc, "v"); got.Basics&TypeNumber == 0 {
		t.Fatalf("v = %#v, want number", got)
	}
}

func TestInferPairsMoreThan64Returns(t *testing.T) {
	t.Run("factory returns", func(t *testing.T) {
		var src strings.Builder
		src.WriteString("local flag = 0\nlocal source = setmetatable({}, { __pairs = function()\n")
		for i := 0; i < 64; i++ {
			if i == 0 {
				fmt.Fprintf(&src, "if flag == %d then", i)
			} else {
				fmt.Fprintf(&src, "elseif flag == %d then", i)
			}
			src.WriteString(" return function() return 1, 1 end\n")
		}
		src.WriteString("else return function() return false, true end end\nend })\nfor k, v in pairs(source) do local key = k; local value = v end")
		assertPairsBoolean(t, []byte(src.String()))
	})
	t.Run("iterator returns", func(t *testing.T) {
		var src strings.Builder
		src.WriteString("local flag = 0\nlocal source = setmetatable({}, { __pairs = function() return function()\n")
		for i := 0; i < 64; i++ {
			if i == 0 {
				fmt.Fprintf(&src, "if flag == %d then", i)
			} else {
				fmt.Fprintf(&src, "elseif flag == %d then", i)
			}
			src.WriteString(" return 1, 1\n")
		}
		src.WriteString("else return false, true end\nend end })\nfor k, v in pairs(source) do local key = k; local value = v end")
		assertPairsBoolean(t, []byte(src.String()))
	})
}

func assertPairsBoolean(t *testing.T, src []byte) {
	t.Helper()
	doc := newPairsInferenceTestDocument(t, src)
	for _, name := range []string{"k", "v"} {
		if got := inferNamedLocal(t, doc, name); got.Basics&TypeBoolean == 0 {
			t.Errorf("%s = %#v, want boolean from 65th return", name, got)
		}
	}
}

func newPairsInferenceTestDocument(t *testing.T, src []byte) *Document {
	t.Helper()
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	return &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
}

func inferNamedLocal(t *testing.T, doc *Document, name string) TypeSet {
	t.Helper()
	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		n := doc.Tree.Nodes[id]
		if n.Kind == ast.KindIdent && string(doc.nodeSource(id)) == name {
			if doc.referenceAt(id) == ast.InvalidNode {
				continue
			}
			return doc.InferType(id)
		}
	}
	t.Fatalf("identifier %q not found", name)
	return TypeSet{}
}

func TestInferPairsLoopVariablesOrdinaryTable(t *testing.T) {
	src := []byte(`local source = { 42 }
for k, v in pairs(source) do local key = k; local value = v end`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	if got := inferNamedLocal(t, doc, "k"); got.Basics&TypeAny == 0 {
		t.Fatalf("k = %#v, want any fallback", got)
	}
	if got := inferNamedLocal(t, doc, "v"); got.Basics&TypeNumber == 0 {
		t.Fatalf("v = %#v, want number", got)
	}
}

func TestInferPairsLoopVariablesMalformedMetamethodTerminates(t *testing.T) {
	src := []byte(`local source = setmetatable({}, { __pairs = function() return source end })
for k, v in pairs(source) do local key = k; local value = v end`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	_ = inferNamedLocal(t, doc, "k")
	_ = inferNamedLocal(t, doc, "v")
}

func TestInferPairsMissingIteratorResultIsNil(t *testing.T) {
	src := []byte(`local source = setmetatable({42}, { __pairs = function()
  return function() return 'key' end
end })
for k, v in pairs(source) do local key = k; local value = v end`)
	doc := newPairsInferenceTestDocument(t, src)
	if got := inferNamedLocal(t, doc, "v"); got.Basics != TypeNil {
		t.Fatalf("v = %#v, want nil for missing iterator result", got)
	}
}

func TestInferPairsEmptyReturnIsNil(t *testing.T) {
	src := []byte(`local source = setmetatable({42}, { __pairs = function()
  return function() return end
end })
for k, v in pairs(source) do local key = k; local value = v end`)
	doc := newPairsInferenceTestDocument(t, src)
	if got := inferNamedLocal(t, doc, "k"); got.Basics != TypeNil {
		t.Fatalf("k = %#v, want nil for empty iterator return", got)
	}
}

func TestInferPairsParenthesizedIteratorAndTailCall(t *testing.T) {
	src := []byte(`local function nextPair() return 'key', 42 end
local source = setmetatable({}, { __pairs = function()
  return (function() return (nextPair()) end)
end })
for k, v in pairs(source) do local key = k; local value = v end`)
	doc := newPairsInferenceTestDocument(t, src)
	if got := inferNamedLocal(t, doc, "k"); got.Basics&TypeString == 0 {
		t.Fatalf("k = %#v, want string", got)
	}
	if got := inferNamedLocal(t, doc, "v"); got.Basics&TypeNumber == 0 {
		t.Fatalf("v = %#v, want number", got)
	}
}

func TestInferPairsExpandsMethodCallTailReturn(t *testing.T) {
	src := []byte(`local iterator = {}
function iterator:next() return 'key', 42 end
local source = setmetatable({}, { __pairs = function()
  return function() return iterator:next() end
end })
for k, v in pairs(source) do local key = k; local value = v end`)
	doc := newPairsInferenceTestDocument(t, src)
	server := NewServer("test")
	doc.Server = server
	server.Documents[doc.URI] = doc
	if got := inferNamedLocal(t, doc, "k"); got.Basics&TypeString == 0 {
		t.Fatalf("k = %#v, want string from method-call tail return", got)
	}
	if got := inferNamedLocal(t, doc, "v"); got.Basics&TypeNumber == 0 {
		t.Fatalf("v = %#v, want number from method-call tail return", got)
	}
}

func TestInferPairsMethodCallTailCycleStaysUnknown(t *testing.T) {
	src := []byte(`local iterator = {}
function iterator:next() return self:next() end
local source = setmetatable({}, { __pairs = function()
  return function() return iterator:next() end
end })
for k, v in pairs(source) do local key = k; local value = v end`)
	doc := newPairsInferenceTestDocument(t, src)
	server := NewServer("test")
	doc.Server = server
	server.Documents[doc.URI] = doc
	if got := inferNamedLocal(t, doc, "v"); got.Basics != TypeUnknown {
		t.Fatalf("v = %#v, want unknown for recursive iterator", got)
	}
}

func BenchmarkInferPairsLoopVariablesMetamethodCold(b *testing.B) {
	src := []byte(`local function nextPair() return 'key', 42 end
local function makeIterator() return function() return nextPair() end end
local source = setmetatable({}, { __pairs = function() return makeIterator() end })
for k, v in pairs(source) do local key = k; local value = v end`)
	tree := ast.NewTree(src)
	p := parser.New(src, tree, 0)
	p.Parse()
	if len(p.Errors) != 0 {
		b.Fatalf("parse errors: %#v", p.Errors)
	}
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///bench.lua"}
	var key, value ast.NodeID
	for id := ast.NodeID(1); int(id) < len(tree.Nodes); id++ {
		if tree.Nodes[id].Kind != ast.KindIdent {
			continue
		}
		name := string(tree.Source[tree.Nodes[id].Start:tree.Nodes[id].End])
		if name == "k" {
			key = id
		}
		if name == "v" {
			value = id
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		clear(doc.TypeCache)
		clear(doc.Inferring)
		_ = doc.InferType(key)
		_ = doc.InferType(value)
	}
}
