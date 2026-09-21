package lsp

import "testing"

func TestFiveMEventPayloadContractValidation(t *testing.T) {
	tests := []struct {
		name       string
		trigger    string
		want       bool
		wantPhrase string
	}{
		{name: "too few", trigger: `TriggerServerEvent("payload:event", 1)`, want: true, wantPhrase: "expected 2 payload arguments, got 1"},
		{name: "too many", trigger: `TriggerServerEvent("payload:event", 1, 2, 3)`, want: true, wantPhrase: "expected 2 payload arguments, got 3"},
		{name: "matching", trigger: `TriggerServerEvent("payload:event", 1, 2)`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
			h.writeWorkspaceFile("provider/server.lua", `RegisterNetEvent("payload:event", function(first, second) end)
`)
			h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
			h.writeWorkspaceFile("consumer/client.lua", tt.trigger+"\n")
			h.reindex()

			diags := h.diagnostics("consumer/client.lua")
			var payload *Diagnostic
			for i := range diags {
				if diags[i].Code == "fivem-event-payload" {
					payload = &diags[i]
					break
				}
			}
			if (payload != nil) != tt.want {
				t.Fatalf("payload diagnostics = %#v, want present=%v", diags, tt.want)
			}
			if payload != nil && tt.wantPhrase != "" && !contains(payload.Message, tt.wantPhrase) {
				t.Fatalf("payload diagnostic = %q, want phrase %q", payload.Message, tt.wantPhrase)
			}
		})
	}
}

func TestFiveMEventPayloadUnknownContractIsIgnored(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("provider/server.lua", `RegisterNetEvent("payload:unknown")
`)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("consumer/client.lua", `TriggerServerEvent("payload:unknown", value)
`)
	h.reindex()

	if diags := h.diagnostics("consumer/client.lua"); hasDiagnosticCode(diags, "fivem-event-payload") {
		t.Fatalf("unknown payload contract produced diagnostic: %#v", diags)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
