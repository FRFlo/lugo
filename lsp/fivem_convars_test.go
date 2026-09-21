package lsp

import "testing"

func TestFiveMConvarWritesReadsAndDeclarations(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nconvar 'manifest_declared'\nserver_script 'server.lua'\nshared_script 'shared.lua'\n")
	h.writeWorkspaceFile("server.lua", `SetConvar("known", "one")
ExecuteCommand("set declared value")
GetConvar("known", "one")
GetConvar("declared", "value")
GetConvar("manifest_declared", "")
`)
	h.writeWorkspaceFile("shared.lua", `GetConvar("missing", "")`)
	h.reindex()
	if diags := h.diagnostics("server.lua"); hasDiagnosticCode(diags, "fivem-convar-unknown") {
		t.Fatalf("declared convars were reported unknown: %#v", diags)
	}
	if !hasDiagnosticCode(h.diagnostics("shared.lua"), "fivem-convar-unknown") {
		t.Fatal("expected unknown convar read diagnostic")
	}
}

func TestFiveMConvarConflictsAndScope(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", `SetConvar("mode", "one")
SetConvar("mode", "two")
GetConvarBool("mode", false)
GetConvar("mode", "three")
`)
	h.writeWorkspaceFile("client.lua", `SetConvar("client_only", "x")
SetConvarReplicated("replicated", "x")
`)
	h.reindex()
	server := assertFiveMDiagnosticsDeterministic(t, func() []Diagnostic { return h.diagnostics("server.lua") })
	if !hasDiagnosticCode(server, "fivem-convar-conflict") {
		t.Fatal("expected conflicting convar declaration diagnostic")
	}
	if !hasDiagnosticCode(server, "fivem-convar-type-conflict") {
		t.Fatal("expected conflicting convar type diagnostic")
	}
	if !hasDiagnosticCode(server, "fivem-convar-default-conflict") {
		t.Fatal("expected conflicting convar default diagnostic")
	}
	client := h.diagnostics("client.lua")
	if !hasDiagnosticCode(client, "fivem-convar-scope") {
		t.Fatalf("expected convar scope diagnostics, got %#v", client)
	}
}

func TestFiveMConvarDynamicNamesAreIgnored(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", `local name = "dynamic"
SetConvar(name, "value")
GetConvar(name, "")
`)
	h.reindex()
	if hasDiagnosticCode(h.diagnostics("server.lua"), "fivem-convar-unknown") {
		t.Fatal("dynamic convar name produced a diagnostic")
	}
}
