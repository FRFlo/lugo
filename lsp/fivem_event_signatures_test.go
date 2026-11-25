package lsp

import (
	"strings"
	"testing"
)

func TestFiveMTriggerReceiverSignatures(t *testing.T) {
	tests := []struct{ name, source, target, extra string }{
		{"TriggerEvent", "client", "client", ""},
		{"TriggerServerEvent", "client", "server", ""},
		{"TriggerClientEvent", "server", "client", ", -1"},
		{"TriggerLatentServerEvent", "client", "server", ", 1000"},
		{"TriggerLatentClientEvent", "server", "client", ", -1, 1000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
			call := "--[[@callee]]" + tt.name + "('--[[@event]]example'" + tt.extra + ")\n"
			handler := "RegisterNetEvent('example', function(payload) end)\n"
			if tt.source == tt.target {
				h.writeWorkspaceFile("resource/"+tt.source+".lua", call+handler)
			} else {
				h.writeWorkspaceFile("resource/"+tt.source+".lua", call)
				h.writeWorkspaceFile("resource/"+tt.target+".lua", handler)
			}
			h.reindex()
			for _, marker := range []string{"callee", "event"} {
				hover := h.hover(marker)
				if hover == nil || !strings.Contains(hover.Contents.Value, "payload") {
					t.Fatalf("hover %s = %#v", marker, hover)
				}
			}
			help := h.signatureHelp("event")
			if help == nil || len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "payload") {
				t.Fatalf("signature help = %#v", help)
			}
			item := completionItemByLabel(h.completion("event"), "example")
			if item == nil || !strings.Contains(item.Detail, "payload") {
				t.Fatalf("completion = %#v", item)
			}
		})
	}
}
