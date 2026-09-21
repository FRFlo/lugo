package lsp

import "testing"

func TestFiveMCommandAndKeyMappingDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", `RegisterCommand("kick", function() end, true)
RegisterKeyMapping("kick", "Kick", "keyboard", "K")
RegisterKeyMapping("other", "Other", "keyboard", "K")
ExecuteCommand("missing")
`)
	h.reindex()
	diags := h.diagnostics("server.lua")
	for _, code := range []string{"fivem-command-missing-ace", "fivem-command-missing-declaration", "fivem-key-mapping-conflict"} {
		if !hasDiagnosticCode(diags, code) {
			t.Errorf("expected %s diagnostic, got %#v", code, diags)
		}
	}
}

func TestFiveMCommandDynamicNamesAreIgnored(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", "local name = 'dynamic'\nRegisterCommand(name, function() end)\nExecuteCommand(name)\n")
	h.reindex()
	for _, diag := range h.diagnostics("server.lua") {
		if diag.Code == "fivem-command-missing-declaration" || diag.Code == "fivem-command-missing-ace" {
			t.Fatalf("dynamic command produced diagnostic: %#v", diag)
		}
	}
}
