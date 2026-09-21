package lsp

import "testing"

func TestStateBagDiagnosticsIndexLiteralKeys(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\n")
	h.writeWorkspaceFile("resource/shared.lua", `
GlobalState.health = 1
Entity(1).state:set('health', 2)
LocalPlayer.state:get('health')
local missing = GlobalState.missing
local function changed() end
AddStateBagChangeHandler('unknown', nil, changed)
`)
	h.reindex()

	diags := h.diagnostics("resource/shared.lua")
	if !hasDiagnosticCode(diags, "fivem-state-bag-missing-key") {
		t.Fatalf("expected missing state bag key diagnostic, got %#v", diags)
	}
	if !hasDiagnosticCode(diags, "fivem-state-bag-unknown-key") {
		t.Fatalf("expected unknown handler key diagnostic, got %#v", diags)
	}
}

func TestStateBagDiagnosticsDoNotGuessComputedKeys(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\n")
	h.writeWorkspaceFile("resource/shared.lua", `
local key = 'dynamic'
GlobalState[key] = true
GlobalState[key]
`)
	h.reindex()
	if diags := h.diagnostics("resource/shared.lua"); hasDiagnosticCode(diags, "fivem-state-bag-missing-key") {
		t.Fatalf("computed keys should be ignored, got %#v", diags)
	}
}
