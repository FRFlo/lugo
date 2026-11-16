package lsp

import (
	"fmt"
	"testing"
)

func TestLuaDocClassFieldReferencesIncludeConstantIndexUses(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Foo
---@field bar number
local Foo = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Foo
local item = {}
local dotted = item.b--[[@field_refs]]ar
local indexed = item["bar"]
`)
	h.reindex()
	refs := h.references("field_refs", false)
	if len(refs) != 2 {
		t.Fatalf("field references = %#v, want dotted and constant-index uses", refs)
	}
}

func BenchmarkLuaDocFieldReferencesManyUnrelatedDocuments(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("types.lua", `---@class Foo
---@field bar number
local Foo = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Foo
local item = {}
local value = item.b--[[@bench_field_refs]]ar
`)
	for i := 0; i < 64; i++ {
		h.writeWorkspaceFile(fmt.Sprintf("unrelated_%02d.lua", i), fmt.Sprintf("local value = %d\n", i))
	}
	h.reindex()
	ctx := h.resolve("bench_field_refs")
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		h.server.getReferences(ctx, false)
	}
}

func TestLuaDocClassFieldReferencesWithWhitespaceAndLongKeys(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Foo
---@field bar number
local Foo = {}
`)
	h.writeWorkspaceFile("origin.lua", `---@type Foo
local item = {}
local value = item.b--[[@field_refs_spacing]]ar
`)
	h.writeWorkspaceFile("other.lua", `---@type Foo
local item = {}
local spaced = item . bar
local indexed = item[ [[bar]] ]
`)
	h.reindex()
	refs := h.references("field_refs_spacing", false)
	if len(refs) != 3 {
		t.Fatalf("field references = %#v, want original, spaced and long-string accesses", refs)
	}
}

func TestLuaDocClassFieldBracketReferencesUseCandidateProfile(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
shared_script 'shared.lua'
client_scripts { 'client_types.lua', 'client.lua' }
`)
	h.writeWorkspaceFile("resource/shared.lua", `---@class Base
---@field bar number
local Base = {}
`)
	h.writeWorkspaceFile("resource/client_types.lua", `---@class Child: Base
local Child = {}
`)
	h.writeWorkspaceFile("resource/client.lua", `---@type Base
local parent = {}
local first = parent.b--[[@profile_refs]]ar
---@type Child
local child = {}
local second = child["bar"]
`)
	h.reindex()
	refs := h.references("profile_refs", false)
	if len(refs) != 2 {
		t.Fatalf("field references = %#v, want base and client subclass accesses", refs)
	}
}

func TestMetatableStringKeyFieldReferences(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { ["run"] = function() end }
local obj = setmetatable({}, { __index = base })
obj.r--[[@string_key_refs]]un()
obj.run()
`)
	h.reindex()

	refs := h.references("string_key_refs", false)
	if len(refs) != 2 {
		t.Fatalf("references = %#v, want both uses", refs)
	}
}
