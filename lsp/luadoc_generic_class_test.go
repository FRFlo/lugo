package lsp

import (
	"fmt"
	"strings"
	"testing"
)

func TestLuaDocGenericClassFieldSubstitution(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Box<T>
---@field value T
local Box = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Box<string>
local box = {}
local result = box.value
`)
	h.reindex()

	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "value"); got.Basics != TypeString || got.CustomName != "" {
		t.Fatalf("Box<string>.value type = %+v, want string", got)
	}
}

func TestLuaDocGenericClassParentSubstitution(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base<T>
---@field payload T
local Base = {}

---@class StringChild: Base<string>
local StringChild = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type StringChild
local child = {}
local result = child.payload
`)
	h.reindex()

	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "payload"); got.Basics != TypeString || got.CustomName != "" {
		t.Fatalf("Base<string>.payload type = %+v, want string", got)
	}
}

func TestLuaDocGenericClassFactoryRuntimeSubstitution(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `---@class Box<T>
---@field value T
local Box = {}

---@generic T
---@param value T
---@return Box<T>
local function makeBox(value)
	return { value = value }
end

local made = makeBox("runtime value")
local result = made.value
`)
	h.reindex()

	doc := h.server.Documents[h.server.pathToURI(h.root+"/main.lua")]
	if got := inferNamedMember(t, doc, "value"); got.Basics != TypeString || got.CustomName != "" {
		t.Fatalf("makeBox(string).value type = %+v, want string", got)
	}
}

func TestLuaDocGenericClassMultipleAndNestedArguments(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Pair<T, U>
---@field first T
---@field second U
---@field nested Map<T, Box<U>>
local Pair = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Pair<string, number>
local pair = {}
local first = pair.first
local second = pair.second
local nested = pair.nested
`)
	h.reindex()

	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "first"); got.Basics != TypeString {
		t.Errorf("Pair<string, number>.first = %+v, want string", got)
	}
	if got := inferNamedMember(t, doc, "second"); got.Basics != TypeNumber {
		t.Errorf("Pair<string, number>.second = %+v, want number", got)
	}
	if got := inferNamedMember(t, doc, "nested"); got.CustomName != "Map<string, Box<number>>" {
		t.Errorf("Pair<string, number>.nested = %+v, want Map<string, Box<number>>", got)
	}
}

func TestLuaDocGenericClassSubstitutesArrayAndCallableFields(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Box<T>
---@field values T[]
---@field transform fun(input: T[]): Map<T, Box<T[]>>
local Box = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Box<number>
local box = {}
local values = box.values
local transform = box.transform
`)
	h.reindex()

	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "values"); got.CustomName != "number[]" {
		t.Errorf("Box<number>.values type = %+v, want number[]", got)
	}
	got := inferNamedMember(t, doc, "transform")
	if got.CallSig == "" || len(got.CallParams) != 1 || got.CallParams[0].Type != "number[]" {
		t.Fatalf("Box<number>.transform signature = %+v, want input: number[]", got)
	}
	if len(got.CallRet) != 1 || got.CallRet[0].Type != "Map<number, Box<number[]>>" {
		t.Fatalf("Box<number>.transform return = %+v, want Map<number, Box<number[]>>", got.CallRet)
	}
}

var genericClassSubstitutionBenchmarkSink string

func BenchmarkLuaDocGenericClassSubstitution(b *testing.B) {
	bindings := map[string]string{"T": "number", "U": "string"}
	for _, tc := range []struct {
		name     string
		typeName string
	}{
		{name: "array", typeName: "T[]"},
		{name: "callable_nested", typeName: "fun(input: Map<T, U[]>): Box<T[]>"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				genericClassSubstitutionBenchmarkSink = substituteLuaDocGenericType(tc.typeName, bindings)
			}
		})
	}
}

func TestLuaDocGenericClassNestedParentTypesRespectResourceScope(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("a/types.lua", `---@class Base<T, U>
---@field payload Map<T, U>
local Base = {}
`)
	h.writeWorkspaceFile("b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'a'\nshared_script '@a/types.lua'\nshared_script 'types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("b/types.lua", `---@class Child: Base<Box<string>, number>
local Child = {}
`)
	h.writeWorkspaceFile("b/use.lua", `---@type Child
local child = {}
local payload = child.payload
`)
	h.writeWorkspaceFile("c/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("c/types.lua", `---@class Base<T, U>
---@field payload boolean
local Base = {}
`)
	h.reindex()

	doc := h.server.Documents[h.server.pathToURI(h.root+"/b/use.lua")]
	if got := inferNamedMember(t, doc, "payload"); got.CustomName != "Map<Box<string>, number>" {
		t.Fatalf("scoped inherited payload = %+v, want Map<Box<string>, number>", got)
	}
}

func TestLuaDocGenericClassInheritanceCycleIsBounded(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class CycleA<T>: CycleB<T>
---@field fromA T
local CycleA = {}

---@class CycleB<U>: CycleA<U>
---@field fromB U
local CycleB = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type CycleA<string>
local cycle = {}
local result = cycle.fromB
`)
	h.reindex()

	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "fromB"); got.Basics != TypeString {
		t.Fatalf("cyclic inherited generic field = %+v, want string", got)
	}
}

func BenchmarkLuaDocGenericClassIndexMemory(b *testing.B) {
	for _, generic := range []bool{false, true} {
		name := "regular"
		if generic {
			name = "generic"
		}
		b.Run(name, func(b *testing.B) {
			var source strings.Builder
			if generic {
				source.WriteString("---@class Box<T, U>\n---@field first T\n---@field nested Map<T, Box<U>>\nlocal Box = {}\n")
			} else {
				source.WriteString("---@class Box\n---@field first string\n---@field nested table\nlocal Box = {}\n")
			}
			for i := range 100 {
				if generic {
					fmt.Fprintf(&source, "---@class Child%d<T>: Box<Map<T, number>, string>\nlocal Child%d = {}\n", i, i)
				} else {
					fmt.Fprintf(&source, "---@class Child%d: Box\nlocal Child%d = {}\n", i, i)
				}
			}
			text := []byte(source.String())
			b.ReportAllocs()
			for b.Loop() {
				server := NewServer("generic-class-benchmark")
				server.updateDocument("file:///generic-classes.lua", text)
			}
		})
	}
}
