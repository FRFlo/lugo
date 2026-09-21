package lsp

import "testing"

func TestDiagnosticsAggregateFiveMAndOrdinaryDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/server.lua", "TriggerServerEvent('demo:event')\nprint(missing_global)\n")
	h.server.DiagFiveMEventDirection = true
	h.server.DiagFiveMUnregisteredNetEvent = false
	h.server.DiagFiveMUnknownEvent = false
	h.server.DiagUndefinedGlobals = true
	h.reindex()

	diags := h.diagnostics("resource/server.lua")
	if !hasDiagnosticCode(diags, "fivem-event-direction") {
		t.Fatalf("expected FiveM event diagnostic, got: %#v", diags)
	}
	if !hasDiagnosticCode(diags, "undefined-global") {
		t.Fatalf("expected ordinary diagnostic alongside FiveM diagnostic, got: %#v", diags)
	}
}

func TestDiagnosticsApplyPragmaSuppressionToFiveMDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/server.lua", "---@diagnostic disable-file fivem-event-direction,undefined-global\nTriggerServerEvent('demo:event')\nprint(missing_global)\n")
	h.server.DiagFiveMEventDirection = true
	h.server.DiagFiveMUnregisteredNetEvent = false
	h.server.DiagFiveMUnknownEvent = false
	h.server.DiagUndefinedGlobals = true
	h.reindex()

	diags := h.diagnostics("resource/server.lua")
	if hasDiagnosticCode(diags, "fivem-event-direction") || hasDiagnosticCode(diags, "undefined-global") {
		t.Fatalf("pragma-suppressed diagnostics were published: %#v", diags)
	}
}
