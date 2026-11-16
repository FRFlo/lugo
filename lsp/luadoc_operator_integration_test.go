package lsp

import "testing"

func TestLuaDocClassOperatorInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Distance
---@operator add(Distance): string
local Distance = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Distance
local value = {}
local result = value + value
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("annotated operator result = %#v, want string", got)
	}
}

func TestLuaDocClassOtherOperatorInference(t *testing.T) {
	for _, tc := range []struct{ name, annotation, expression, want string }{
		{"unary", "unm: string", "-value", "string"},
		{"concat", "concat(Text): number", "value .. value", "number"},
		{"call", "call: string", "value()", "string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("types.lua", "---@class Text\n---@operator "+tc.annotation+"\nlocal Text = {}\n")
			h.writeWorkspaceFile("use.lua", "---@type Text\nlocal value = {}\nlocal result = "+tc.expression+"\n")
			h.reindex()
			doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
			id := findIdentByOccurrence(t, doc.Tree, "result", 0)
			if got, want := doc.InferType(id), ParseTypeString(tc.want); got.Basics != want.Basics {
				t.Fatalf("operator result = %#v, want %s", got, tc.want)
			}
		})
	}
}

func TestLuaDocClassIndexOperatorInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Container
---@operator index(number): string
local Container = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Container
local value = {}
local result = value[1]
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("index operator result = %#v, want string", got)
	}
}

func TestLuaDocOperatorAcceptsSubclassOperand(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
local Base = {}
---@class Derived: Base
local Derived = {}
---@class Vector
---@operator add(Base): string
local Vector = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Vector
local left = {}
---@type Derived
local right = {}
local result = left + right
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("operator with subclass operand = %#v, want string", got)
	}
}

func TestLuaDocCallOperatorWithArgument(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@operator call(number): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert(42)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("annotated call result = %#v, want string", got)
	}
}

func TestLuaDocClassOverloadCallInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@overload fun(a: number): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert(42)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("overloaded class call result = %#v, want string", got)
	}
}

func TestLuaDocClassOverloadSelectsAllArguments(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@overload fun(a: number, b: number): boolean
---@overload fun(a: number, b: string): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert(42, "x")
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("overload selected = %#v, want string", got)
	}
}

func TestLuaDocClassOverloadSignatureHelp(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@overload fun(input: number): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert(--[[@overload_signature]])
`)
	h.reindex()
	help := h.signatureHelp("overload_signature")
	if help == nil || len(help.Signatures) == 0 {
		t.Fatalf("class overload signature = %#v, want signature", help)
	}
}

func TestLuaDocClassOverloadSignatureHelpSelectsMatchingOverload(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@overload fun(a: number, b: number): boolean
---@overload fun(a: number, b: string): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert(42, "x")--[[@matching_overload]]
`)
	h.reindex()
	help := h.signatureHelp("matching_overload")
	if help == nil || len(help.Signatures) != 2 {
		t.Fatalf("class overload signatures = %#v, want both overloads", help)
	}
	if help.ActiveSignature != 1 {
		t.Fatalf("active signature = %d, want second overload for (number, string)", help.ActiveSignature)
	}
}

func TestLuaDocClassOverloadSignatureHelpSelectsVariadicOverload(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@overload fun(value: number): boolean
---@overload fun(...: string): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert("x", "y")--[[@variadic_overload]]
`)
	h.reindex()
	help := h.signatureHelp("variadic_overload")
	if help == nil || len(help.Signatures) != 2 {
		t.Fatalf("class overload signatures = %#v, want both overloads", help)
	}
	if help.ActiveSignature != 1 {
		t.Fatalf("active signature = %d, want variadic string overload", help.ActiveSignature)
	}
}

func TestLuaDocClassOverloadSignatureHelpPrefersFixedOverloadForNoArguments(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@overload fun(): boolean
---@overload fun(...: string): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert()--[[@zero_argument_overload]]
`)
	h.reindex()
	help := h.signatureHelp("zero_argument_overload")
	if help == nil || len(help.Signatures) != 2 {
		t.Fatalf("class overload signatures = %#v, want both overloads", help)
	}
	if help.ActiveSignature != 0 {
		t.Fatalf("active signature = %d, want exact zero-argument overload", help.ActiveSignature)
	}
}

func TestLuaDocClassOverloadAcceptsSubclassArgument(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
local Base = {}
---@class Derived: Base
local Derived = {}
---@class Converter
---@overload fun(value: Base): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
---@type Derived
local value = {}
local result = convert(value)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("overload with subclass argument = %#v, want string", got)
	}
}

func TestLuaDocClassOverloadOptionalArgument(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Converter
---@overload fun(value: number, suffix?: string): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert(42)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("overload without optional argument = %#v, want string", got)
	}
}

func TestLuaDocClassOverloadOptionalClassAcceptsNil(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
local Base = {}
---@class Converter
---@overload fun(value?: Base): string
local Converter = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Converter
local convert = {}
local result = convert(nil)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	id := findIdentByOccurrence(t, doc.Tree, "result", 0)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("overload with nil optional class = %#v, want string", got)
	}
}
