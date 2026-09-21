package lsp

import "testing"

func TestFiveMServerEventUntrustedParameterBeforeValidation(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMTrustBoundary = true
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/server.lua", `RegisterNetEvent("admin:give", function(item)
	TriggerClientEvent("admin:result", -1, item)
end)
`)
	h.reindex()

	if !hasDiagnosticCode(h.diagnostics("resource/server.lua"), "fivem-event-untrusted-parameter") {
		t.Fatal("expected an untrusted event parameter diagnostic")
	}
}

func TestFiveMServerEventValidatedParameterIsAllowed(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMTrustBoundary = true
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/server.lua", `RegisterNetEvent("admin:give", function(item)
	if type(item) ~= "string" then return end
	TriggerClientEvent("admin:result", -1, item)
end)
`)
	h.reindex()

	if hasDiagnosticCode(h.diagnostics("resource/server.lua"), "fivem-event-untrusted-parameter") {
		t.Fatal("validated event parameter should not be diagnosed")
	}
}

func TestFiveMServerEventMissingTrustChecksIsWarning(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMTrustBoundary = true
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/server.lua", `RegisterNetEvent("admin:give", function(item)
	TriggerClientEvent("admin:result", -1, item)
end)
`)
	h.reindex()

	for _, diag := range h.diagnostics("resource/server.lua") {
		if diag.Code == "fivem-event-missing-source-check" || diag.Code == "fivem-event-missing-authorization" {
			if diag.Severity != SeverityWarning {
				t.Fatalf("trust-boundary diagnostic severity = %v, want warning", diag.Severity)
			}
			return
		}
	}
	t.Fatal("expected a missing source or authorization diagnostic")
}
