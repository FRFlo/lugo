package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestMetatableConditionalIndexAlternatives(t *testing.T) {
	doc := newIndexTestDocument(t, `local first = { value = "before" }
local second = { value = 42 }
local mt = { __index = first }
local object = setmetatable({}, mt)
if flag then mt.__index = second end
local result = object.value
`)
	got := inferLastNamedMember(t, doc, "value")
	if got.Basics&(TypeString|TypeNumber) != TypeString|TypeNumber {
		t.Fatalf("conditional __index result = %#v, want string | number", got)
	}
}

func TestMetatableConditionalCallAlternatives(t *testing.T) {
	doc := newIndexTestDocument(t, `local mt = { __call = function() return "before" end }
local object = setmetatable({}, mt)
if flag then mt.__call = function() return 42 end end
local result = object()
`)
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics&(TypeString|TypeNumber) != TypeString|TypeNumber {
		t.Fatalf("conditional __call result = %#v, want string | number", got)
	}
}

func TestMetatableConditionalProtectionAlternatives(t *testing.T) {
	doc := newIndexTestDocument(t, `local mt = { __metatable = "before" }
local object = setmetatable({}, mt)
if flag then mt.__metatable = 42 end
local result = getmetatable(object)
`)
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics&(TypeString|TypeNumber) != TypeString|TypeNumber {
		t.Fatalf("conditional __metatable result = %#v, want string | number", got)
	}
}

func TestMetatableConditionalIndexNavigationIsAmbiguous(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local first = { value = "before" }
local second = { value = 42 }
local mt = { __index = first }
local object = setmetatable({}, mt)
if flag then mt.__index = second end
local result = object.val--[[@conditional_index_alternatives]]ue
`)
	h.reindex()
	ctx := h.resolve("conditional_index_alternatives")
	if ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("ambiguous __index navigation selected a single target: %#v", ctx)
	}
}
