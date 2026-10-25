package lsp

import "testing"

func TestFiveMVectorConstructorsAcceptComponents(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'client.lua'
`)
	h.writeWorkspaceFile("resource/client.lua", `local a = vector2(1, 2)
local b = vector3(1, 2, 3)
local c = vector4(1, 2, 3, 4)
return a, b, c
`)
	h.reindex()

	diags := h.diagnostics("resource/client.lua")
	if hasDiagnosticCode(diags, "redundant-parameter") {
		t.Fatalf("valid vector component constructors produced arity diagnostics: %#v", diags)
	}
}
