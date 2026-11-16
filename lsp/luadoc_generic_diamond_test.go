package lsp

import "testing"

func TestLuaDocGenericDiamondUsesFirstParentBinding(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base<T>
---@field payload T
local Base = {}
---@class Left: Base<string>
local Left = {}
---@class Right: Base<number>
local Right = {}
---@class Child: Left, Right
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local child = {}
local result = child.payload
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "payload"); got.Basics != TypeString {
		t.Fatalf("diamond field type = %+v, want first parent's string binding", got)
	}
}

func TestLuaDocGenericDiamondAcrossFiveMResources(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("base/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("base/types.lua", "---@class Base<T>\n---@field payload T\nlocal Base = {}\n")
	for _, branch := range []struct{ resource, class, argument string }{
		{"left", "Left", "string"}, {"right", "Right", "number"},
	} {
		h.writeWorkspaceFile(branch.resource+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'base'\nshared_script '@base/types.lua'\nshared_script 'types.lua'\n")
		h.writeWorkspaceFile(branch.resource+"/types.lua", "---@class "+branch.class+": Base<"+branch.argument+">\nlocal "+branch.class+" = {}\n")
	}
	h.writeWorkspaceFile("child/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependencies { 'left', 'right' }\nshared_scripts { '@left/types.lua', '@right/types.lua', 'types.lua' }\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("child/types.lua", "---@class Child: Left, Right\nlocal Child = {}\n")
	h.writeWorkspaceFile("child/use.lua", "---@type Child\nlocal child = {}\nlocal result = child.payload\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/child/use.lua")]
	if got := inferNamedMember(t, doc, "payload"); got.Basics != TypeString {
		t.Fatalf("scoped diamond field type = %+v, want first parent's string binding", got)
	}
}
