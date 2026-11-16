package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
)

func TestLuaDocGenericCallSubstitutesArguments(t *testing.T) {
	doc := newIndexTestDocument(t, `---@generic T
---@param value T
---@return T
local function identity(value) return value end
local numberResult = identity(42)
local stringResult = identity("hello")
local booleanResult = identity(true)
`)
	doc.Server = NewServer("test")
	for name, want := range map[string]struct {
		basic BasicType
		label string
	}{
		"numberResult":  {TypeNumber, "number"},
		"stringResult":  {TypeString, "string"},
		"booleanResult": {TypeBoolean, "boolean"},
	} {
		got := doc.InferType(findIdentByOccurrence(t, doc.Tree, name, 0))
		if got.Basics != want.basic {
			t.Errorf("%s = %#v, want %s", name, got, want.label)
		}
	}
}

func TestLuaDocGenericCallsAreIsolatedAndCombineRepeatedTypeParameters(t *testing.T) {
	doc := newIndexTestDocument(t, `---@generic T
---@param first T
---@param second T
---@return T
local function choose(first, second) return first end
local numberResult = choose(42, 1)
local stringResult = choose("text", "other")
`)
	doc.Server = NewServer("test")
	if got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "numberResult", 0)); got.Basics != TypeNumber {
		t.Fatalf("numberResult = %#v, want number", got)
	}
	if got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "stringResult", 0)); got.Basics != TypeString {
		t.Fatalf("stringResult = %#v, want string", got)
	}
}

func TestLuaDocGenericCallUsesBoundWhenArgumentIsUnknown(t *testing.T) {
	doc := newIndexTestDocument(t, `---@generic T : Item
---@param value T
---@return T
local function identity(value) return value end
local result = identity(unknownValue)
`)
	doc.Server = NewServer("test")
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.CustomName != "Item" {
		t.Fatalf("bounded generic result = %#v, want Item", got)
	}
}

func TestLuaDocGenericCallAcrossFiveMFiles(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("provider/types.lua", `---@generic T
---@param value T
---@return T
function identity(value) return value end
`)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nshared_script '@provider/types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("consumer/use.lua", `local result = identity(42)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/consumer/use.lua")]
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics != TypeNumber {
		t.Fatalf("cross-resource generic result = %#v, want number", got)
	}
}

func TestLuaDocGenericCallRefreshesWhenProviderAnnotationChanges(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	providerURI := h.server.pathToURI(h.root + "/provider/types.lua")
	provider := `---@generic T
---@param value T
---@return T
function identity(value) return value end
`
	h.writeWorkspaceFile("provider/types.lua", provider)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nshared_script '@provider/types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("consumer/use.lua", `local result = identity(42)
`)
	h.reindex()

	consumerURI := h.server.pathToURI(h.root + "/consumer/use.lua")
	consumer := h.server.Documents[consumerURI]
	resultID := findIdentByOccurrence(t, consumer.Tree, "result", 0)
	if got := consumer.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("initial generic result = %#v, want number", got)
	}

	updatedProvider := []byte(`---@generic T
---@param value T
---@return boolean
function identity(value) return true end
	`)
	h.server.updateDocument(providerURI, updatedProvider)
	consumer = h.server.Documents[consumerURI]
	if got := consumer.InferType(resultID); got.Basics != TypeBoolean {
		t.Fatalf("updated generic result = %#v, want boolean after provider annotation change", got)
	}
}

func TestLuaDocGenericModuleCallCacheRefreshesWithProviderAnnotation(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	providerURI := h.server.pathToURI(h.root + "/lib.lua")
	h.writeWorkspaceFile("lib.lua", `local M = {}
---@generic T
---@param value T
---@return T
function M.identity(value) return value end
return M
`)
	h.writeWorkspaceFile("use.lua", `local module = require("lib")
local result = module.identity(42)
`)
	h.reindex()

	consumerURI := h.server.pathToURI(h.root + "/use.lua")
	consumer := h.server.Documents[consumerURI]
	resultID := findIdentByOccurrence(t, consumer.Tree, "result", 0)
	if got := consumer.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("initial module generic result = %#v, want number", got)
	}

	h.server.updateDocument(providerURI, []byte(`local M = {}
---@generic T
---@param value T
---@return boolean
function M.identity(value) return true end
return M
`))
	consumer = h.server.Documents[consumerURI]
	if got := consumer.InferType(resultID); got.Basics != TypeBoolean {
		t.Fatalf("updated module generic result = %#v, want boolean", got)
	}
}

func BenchmarkLuaDocGenericCallInferenceHotCold(b *testing.B) {
	source := []byte(`---@generic T
---@param value T
---@return T
local function identity(value) return value end
local result = identity(42)
	`)
	tree := ast.NewTree(source)
	p := parser.New(source, tree, 0)
	p.Parse()
	if len(p.Errors) != 0 {
		b.Fatalf("parse errors = %#v", p.Errors)
	}
	doc := &Document{Tree: tree, Resolver: semantic.New(tree), URI: "file:///generic_benchmark.lua"}
	doc.Resolver.Resolve(tree.Root)
	doc.Server = NewServer("benchmark")
	var result ast.NodeID = ast.InvalidNode
	for id, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindIdent && string(doc.nodeSource(ast.NodeID(id))) == "result" {
			result = ast.NodeID(id)
			break
		}
	}
	if result == ast.InvalidNode {
		b.Fatal("result identifier not found")
	}
	b.Run("hot", func(b *testing.B) {
		_ = doc.InferType(result)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = doc.InferType(result)
		}
	})
	b.Run("cold", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			doc.TypeCache = nil
			doc.Inferring = nil
			_ = doc.InferType(result)
		}
	})
}
