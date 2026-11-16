package lsp

import "testing"

var luaDocOperatorMemorySink TypeSet
var luaDocOperatorMemoryOK bool

func BenchmarkLuaDocOperatorInferenceMemory(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("types.lua", `---@class Vector
---@operator add(Vector): Vector
local Vector = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Vector
local value = {}
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	left := TypeSet{CustomName: "Vector"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		luaDocOperatorMemorySink, luaDocOperatorMemoryOK = doc.inferLuaDocOperator(left, left, "__add")
	}
	if !luaDocOperatorMemoryOK {
		b.Fatal("operator lookup failed")
	}
}

func TestLuaDocOperatorInferenceAllocationBudget(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Vector
---@operator add(Vector): Vector
local Vector = {}
`)
	h.writeWorkspaceFile("use.lua", "---@type Vector\nlocal value = {}\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	typ := TypeSet{CustomName: "Vector"}
	allocs := testing.AllocsPerRun(100, func() { luaDocOperatorMemorySink, luaDocOperatorMemoryOK = doc.inferLuaDocOperator(typ, typ, "__add") })
	if !luaDocOperatorMemoryOK || allocs > 1 {
		t.Fatalf("operator lookup allocated %.1f times, want at most 1", allocs)
	}
}
