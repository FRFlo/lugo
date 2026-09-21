package lsp

import "testing"

func TestFiveMExportContractManifestMissingImplementation(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nexport 'ping'\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\ndependency 'provider'\n")
	h.writeWorkspaceFile("consumer/client.lua", "return exports.provider:ping()\n")
	h.reindex()

	if !hasDiagnosticCode(h.diagnostics("provider/fxmanifest.lua"), "fivem-export-missing-implementation") {
		t.Fatal("manifest export without implementation should be diagnosed")
	}
}

func TestFiveMExportContractImplementationNotDeclared(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("provider/server.lua", "exports('ping', function() end)\n")
	h.reindex()

	if !hasDiagnosticCode(h.diagnostics("provider/server.lua"), "fivem-export-not-declared") {
		t.Fatal("export implementation absent from manifest should be diagnosed")
	}
}

func TestFiveMExportContractDuplicateNames(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nexport 'ping'\nexport 'ping'\n")
	h.writeWorkspaceFile("provider/server.lua", "exports('ping', function() end)\nexports('ping', function() end)\n")
	h.reindex()

	if !hasDiagnosticCode(h.diagnostics("provider/fxmanifest.lua"), "fivem-duplicate-export") {
		t.Fatal("duplicate manifest export should be diagnosed")
	}
	if !hasDiagnosticCode(h.diagnostics("provider/server.lua"), "fivem-duplicate-export") {
		t.Fatal("duplicate export implementation should be diagnosed")
	}
}

func TestFiveMExportContractUnknownConsumerTarget(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_export 'ping'\nclient_script 'server.lua'\n")
	h.writeWorkspaceFile("provider/server.lua", "exports('ping', function() end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("consumer/client.lua", "local a = exports.unknown:ping()\nlocal b = exports.provider:missing()\n")
	h.reindex()

	diags := h.diagnostics("consumer/client.lua")
	if !hasDiagnosticCode(diags, "fivem-unknown-resource") {
		t.Fatal("consumer targeting unknown resource should be diagnosed")
	}
	if !hasDiagnosticCode(diags, "fivem-unknown-export") {
		t.Fatal("consumer targeting unknown export should be diagnosed")
	}
}
