package lsp

import "testing"

func TestLuaDocGenericMethodCallSubstitutesArgument(t *testing.T) {
	doc := newIndexTestDocument(t, `local object = {}
---@generic T
---@param self table
---@param value T
---@return T
function object:identity(value) return value end
local result = object:identity(42)
`)
	doc.Server = NewServer("test")
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics != TypeNumber {
		t.Fatalf("generic method result = %#v, want number", got)
	}
}

func TestLuaDocGenericMethodWithoutExplicitSelfParameter(t *testing.T) {
	doc := newIndexTestDocument(t, `local object = {}
---@generic T
---@param value T
---@return T
function object:identity(value) return value end
local result = object:identity(42)
`)
	doc.Server = NewServer("test")
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics != TypeNumber {
		t.Fatalf("generic method without @param self = %#v, want number", got)
	}
}
