package lsp

import "testing"

func TestFiveMTriggerServerEventResolvesOnlyServerOrSharedRegistrations(t *testing.T) {
	tests := []struct {
		name         string
		registration string
		wantWarning  bool
	}{
		{name: "server", registration: "server.lua", wantWarning: false},
		{name: "shared", registration: "shared.lua", wantWarning: false},
		{name: "client", registration: "client.lua", wantWarning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
			h.writeWorkspaceFile("consumer/client.lua", "TriggerServerEvent('contract:event')\n")
			h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n"+tt.name+"_script '"+tt.registration+"'\n")
			h.writeWorkspaceFile("provider/"+tt.registration, "RegisterNetEvent('contract:event', function() end)\n")
			h.reindex()

			diags := h.diagnostics("consumer/client.lua")
			if hasDiagnosticCode(diags, "fivem-unregistered-net-event") != tt.wantWarning {
				t.Fatalf("TriggerServerEvent registration in %s: diagnostics = %#v, want warning=%v", tt.registration, diags, tt.wantWarning)
			}
		})
	}
}

func TestFiveMTriggerClientEventResolvesOnlyClientOrSharedRegistrationsAcrossResources(t *testing.T) {
	tests := []struct {
		name         string
		registration string
		wantWarning  bool
	}{
		{name: "client", registration: "client.lua", wantWarning: false},
		{name: "shared", registration: "shared.lua", wantWarning: false},
		{name: "server", registration: "server.lua", wantWarning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
			h.writeWorkspaceFile("consumer/server.lua", "TriggerClientEvent('contract:event', -1)\n")
			h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n"+tt.name+"_script '"+tt.registration+"'\n")
			h.writeWorkspaceFile("provider/"+tt.registration, "RegisterNetEvent('contract:event', function() end)\n")
			h.reindex()

			diags := h.diagnostics("consumer/server.lua")
			if hasDiagnosticCode(diags, "fivem-unregistered-net-event") != tt.wantWarning {
				t.Fatalf("TriggerClientEvent registration in %s: diagnostics = %#v, want warning=%v", tt.registration, diags, tt.wantWarning)
			}
		})
	}
}
