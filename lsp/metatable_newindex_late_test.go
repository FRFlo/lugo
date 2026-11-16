package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestNewIndexLateMetatableFieldRoutesInferenceAndNavigation(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local sink = {}
local mt = {}
local object = {}
mt.__newindex = sink
setmetatable(object, mt)
object.answer = 42
local result = sink.--[[@late_newindex_answer]]answer
`)
	h.reindex()
	ctx := h.resolve("late_newindex_answer")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("late __newindex write did not navigate to sink assignment: %#v", ctx)
	}
	if ctx.TargetDoc != h.docForMarker("late_newindex_answer") {
		t.Fatalf("late __newindex target document = %v, want source document", ctx.TargetDoc)
	}
	got := h.docForMarker("late_newindex_answer").InferType(ctx.TargetDefID)
	if got.Basics&TypeNumber == 0 {
		t.Fatalf("late __newindex sink field type = %#v, want number", got)
	}
}

func TestNewIndexNilFallbackChainSupportsNavigation(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local storage = {}
local sink = setmetatable({answer=nil}, {__newindex=storage})
local object = setmetatable({}, {__newindex=sink})
object.answer = 42
local result = storage.--[[@nil_chain_navigation]]answer
`)
	h.reindex()
	ctx := h.resolve("nil_chain_navigation")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("nil fallback chain did not navigate to routed assignment: %#v", ctx)
	}
	got := ctx.TargetDoc.InferType(ctx.TargetDefID)
	if got.Basics != TypeNumber {
		t.Fatalf("navigated assignment type = %#v, want number", got)
	}
}

func TestNewIndexLateMetatableOrderReplacementAndClear(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local first = {}
local second = {}
local mt = {}
local object = {}
mt.__newindex = first
setmetatable(object, mt)
object.value = "first"
mt.__newindex = second
object.value = 2
mt.__newindex = nil
object.value = true
local objectValue = object.value
local firstValue = first.value
local secondValue = second.--[[@late_replacement]]value
`)
	h.reindex()
	doc := h.docForMarker("late_replacement")

	for id := len(doc.Tree.Nodes) - 1; id > 0; id-- {
		n := doc.Tree.Nodes[id]
		if n.Kind != ast.KindMemberExpr || n.Right == ast.InvalidNode || string(doc.nodeSource(n.Right)) != "value" {
			continue
		}
		switch string(doc.nodeSource(n.Left)) {
		case "first":
			if got := doc.InferType(ast.NodeID(id)); got.Basics&TypeString == 0 || got.Basics&TypeNumber != 0 {
				t.Fatalf("first sink value = %#v, want only pre-replacement string", got)
			}
		case "second":
			if got := doc.InferType(ast.NodeID(id)); got.Basics&TypeNumber == 0 {
				t.Fatalf("second sink value = %#v, want replacement number", got)
			}
		case "object":
			if n.Parent != ast.InvalidNode && doc.Tree.Nodes[n.Parent].Kind == ast.KindExprList && doc.Tree.Nodes[doc.Tree.Nodes[n.Parent].Parent].Kind == ast.KindLocalAssign {
				if got := doc.InferType(ast.NodeID(id)); got.Basics&TypeBoolean == 0 {
					t.Fatalf("write after clearing __newindex was not stored on object: %#v", got)
				}
			}
		}
	}
}

func TestNewIndexLateMetatableRespectsShadowedBindingAndLiteralKey(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local sink = {}
local mt = {}
local object = {}
mt.__newindex = sink
setmetatable(object, mt)
object["answer"] = 9
do
  local object = {}
  object["answer"] = "shadow"
end
local result = sink.--[[@late_literal_key]]answer
`)
	h.reindex()
	doc := h.docForMarker("late_literal_key")
	got := inferLastNamedMember(t, doc, "answer")
	if got.Basics&TypeNumber == 0 || got.Basics&TypeString != 0 {
		t.Fatalf("literal-key write or binding shadowing inferred incorrectly: %#v", got)
	}
}
