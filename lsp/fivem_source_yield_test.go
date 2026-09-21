package lsp

import (
	"strings"
	"testing"
)

func TestFiveMServerSourceAfterYieldDiagnostic(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/server.lua", `RegisterNetEvent("player:loaded", function()
	Wait(0)
	print(source)
end)
`)
	h.reindex()

	diags := h.diagnostics("resource/server.lua")
	if !hasDiagnosticCode(diags, "fivem-source-after-yield") {
		t.Fatalf("expected source-after-yield diagnostic, got: %#v", diags)
	}
}

func TestFiveMServerSourceCapturedBeforeYieldIsSafe(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/server.lua", `RegisterNetEvent("player:loaded", function()
	local playerSource = source
	Citizen.Wait(0)
	print(playerSource)
end)
`)
	h.reindex()

	for _, diag := range h.diagnostics("resource/server.lua") {
		if diag.Code == "fivem-source-after-yield" || strings.Contains(diag.Message, "source after") {
			t.Fatalf("captured source should not be diagnosed: %#v", diag)
		}
	}
}
