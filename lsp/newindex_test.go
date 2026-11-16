package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/semantic"
)

func TestNewIndexFallbackTableFieldInference(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = {}
local object = setmetatable({}, { __newindex = sink })
object.answer = 42
local result = sink.answer
`)
	got := inferLastNamedMember(t, doc, "answer")
	if got.Basics&TypeNumber == 0 {
		t.Fatalf("fallback table field type = %#v, want number", got)
	}
}

func TestNewIndexExistingFieldKeepsWriteOnReceiver(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = { answer = "fallback" }
local object = setmetatable({ answer = false }, { __newindex = sink })
object.answer = 42
local result = object.answer
`)
	got := inferLastNamedMember(t, doc, "answer")
	if got.Basics&TypeBoolean == 0 || got.Basics&TypeString != 0 {
		t.Fatalf("existing receiver field type = %#v, want receiver boolean and no fallback string", got)
	}
}

func TestNewIndexFunctionDoesNotPredictArbitraryWrites(t *testing.T) {
	doc := newIndexTestDocument(t, `local object = setmetatable({}, { __newindex = function(_, key, value) rawset({}, key, value) end })
object.answer = 42
local result = object.answer
`)
	got := inferLastNamedMember(t, doc, "answer")
	if got.Basics&TypeNumber != 0 {
		t.Fatalf("function __newindex was treated as a predictable table write: %#v", got)
	}
}

func inferLastNamedMember(t *testing.T, doc *Document, name string) TypeSet {
	t.Helper()
	for id := len(doc.Tree.Nodes) - 1; id > 0; id-- {
		node := doc.Tree.Nodes[id]
		if node.Kind == ast.KindMemberExpr && node.Right != ast.InvalidNode && string(doc.nodeSource(node.Right)) == name {
			return doc.InferType(ast.NodeID(id))
		}
	}
	t.Fatalf("member %q not found", name)
	return TypeSet{}
}

func newIndexTestDocument(t *testing.T, source string) *Document {
	t.Helper()
	tree := parseResolverLua(t, []byte(source))
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	return &Document{Tree: tree, Resolver: resolver, URI: "file:///newindex.lua"}
}

func TestNewIndexResolverUsesFallbackAssignment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local sink = {}
local object = setmetatable({}, { __newindex = sink })
object.answer = 42
local result = sink.--[[@newindex_answer]]answer
`)
	h.reindex()
	ctx := h.resolve("newindex_answer")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("fallback table write did not resolve: %#v", ctx)
	}
}

func TestNewIndexFollowsMetatableChain(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = {}
local middle = setmetatable({}, { __newindex = sink })
local object = setmetatable({}, { __newindex = middle })
object.answer = 42
local result = sink.answer
`)
	if got := inferLastNamedMember(t, doc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("chained fallback table field type = %#v, want number", got)
	}
}

func TestNewIndexNilFieldRoutesWrite(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = {}
local object = setmetatable({ answer = nil }, { __newindex = sink })
object.answer = 42
local own = object.answer
local result = sink.answer
`)
	if got := inferLastNamedMember(t, doc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("fallback after nil field = %#v, want number", got)
	}
	for id := len(doc.Tree.Nodes) - 1; id > 0; id-- {
		node := doc.Tree.Nodes[id]
		if node.Kind == ast.KindMemberExpr && string(doc.nodeSource(node.Left)) == "object" && string(doc.nodeSource(node.Right)) == "answer" {
			if got := doc.InferType(ast.NodeID(id)); got.Basics&TypeNumber != 0 {
				t.Fatalf("routed write incorrectly became an own field: %#v", got)
			}
			return
		}
	}
	t.Fatal("object.answer read not found")
}

func TestNewIndexIgnoresStaleCandidateNodes(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = {}`)
	server := NewServer("test")
	doc.Server = server
	server.Documents[doc.URI] = doc
	server.newIndexWrites = &newIndexWriteIndex{version: server.metatableIndexVersion, byName: map[string][]newIndexWriteCandidate{
		"value": {{tree: doc.Tree, doc: doc, node: 1, member: ast.NodeID(len(doc.Tree.Nodes) + 100)}},
	}}
	_, _, _ = doc.newIndexWrite(TypeSet{DeclNode: 1, DeclURI: doc.URI}, "value", ^uint32(0))
}

func TestNewIndexFallbackTableBracketWrite(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = {}
local object = setmetatable({}, { __newindex = sink })
object["answer"] = 42
local result = sink.answer
`)
	if got := inferLastNamedMember(t, doc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("bracket write fallback field type = %#v, want number", got)
	}
}

func TestNewIndexFallbackTableEscapedBracketWrite(t *testing.T) {
	doc := newIndexTestDocument(t, `local sink = {}
local object = setmetatable({}, { __newindex = sink })
object["\x61nswer"] = 42
local result = sink.answer
`)
	if got := inferLastNamedMember(t, doc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("escaped bracket write fallback field type = %#v, want number", got)
	}
}
