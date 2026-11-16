package lsp

import "testing"

func TestNewIndexCrossDocumentWritesAreMergedWithoutOffsetOrdering(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("setup.lua", `sink = {}
object = setmetatable({}, { __newindex = sink })
`)
	h.writeWorkspaceFile("write.lua", `object.answer = "from another file"
`)
	h.writeWorkspaceFile("another_write.lua", `object.answer = true
`)
	h.writeWorkspaceFile("read.lua", `local result = sink.--[[@cross_document_write]]answer
`)
	h.reindex()

	doc := h.docForMarker("cross_document_write")
	got := inferLastNamedMember(t, doc, "answer")
	if got.Basics&(TypeString|TypeBoolean) != TypeString|TypeBoolean {
		t.Fatalf("cross-document fallback field = %#v, want string and boolean possibilities", got)
	}
}

func TestNewIndexCrossDocumentWritesRespectFiveMResourceScope(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource_a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'shared.lua', 'client.lua' }\n")
	h.writeWorkspaceFile("resource_a/shared.lua", `sink = {}
object = setmetatable({}, { __newindex = sink })
`)
	h.writeWorkspaceFile("resource_a/client.lua", `object.answer = "visible"
local result = sink.--[[@fivem_visible_write]]answer
`)
	h.writeWorkspaceFile("resource_b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'shared.lua', 'client.lua' }\n")
	h.writeWorkspaceFile("resource_b/shared.lua", `sink = {}
object = setmetatable({}, { __newindex = sink })
`)
	h.writeWorkspaceFile("resource_b/client.lua", `object.answer = true
`)
	h.reindex()

	doc := h.docForMarker("fivem_visible_write")
	got := inferLastNamedMember(t, doc, "answer")
	if got.Basics&TypeString == 0 || got.Basics&TypeBoolean != 0 {
		t.Fatalf("FiveM scoped fallback field = %#v, want only the visible resource's string", got)
	}
}
