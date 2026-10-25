package lsp

import (
	"strings"
	"testing"
)

func TestFiveMBuiltinEventCatalogCompletionAndHover(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("server.lua", "AddEventHandler(\"--[[@hover_connecting]]playerConnecting\", function() end)\nTriggerServerEvent(\"--[[@completion]]\")\nAddEventHandler(--[[@hover_unquoted]]playerDropped, function() end)\nAddEventHandler(\"--[[@hover_refresh]]onResourceListRefresh\", function() end)\nTriggerClientEvent('sessionHostResult', source, 'go')\n")
	h.writeWorkspaceFile("client.lua", "AddEventHandler(\"--[[@hover_chat_template]]chat:addTemplate\", function() end)\nAddEventHandler('__cfx_internal:serverPrint', function() end)\n")
	h.reindex()

	for _, tc := range []struct {
		name        string
		marker      string
		description string
		profile     string
	}{
		{"playerConnecting", "hover_connecting", "connecting to the server", "SERVER"},
		{"playerDropped", "hover_unquoted", "disconnects", "SERVER"},
		{"onResourceListRefresh", "hover_refresh", "resource list is refreshed", "SERVER"},
		{"chat:addTemplate", "hover_chat_template", "message template", "CLIENT"},
	} {
		hover := h.hover(tc.marker)
		if hover == nil {
			t.Fatalf("%s hover is nil", tc.name)
		}
		value := hover.Contents.Value
		for _, want := range []string{tc.name, tc.description, "Profile: " + tc.profile, "Direction:"} {
			if !strings.Contains(value, want) {
				t.Errorf("%s hover = %q, want %q", tc.name, value, want)
			}
		}
	}

	item := completionItemByLabel(h.completion("completion"), "playerConnecting")
	if item == nil || item.Documentation == nil {
		t.Fatalf("playerConnecting completion lacks documentation: %#v", item)
	}
	if !strings.Contains(item.Documentation.Value, "connecting to the server") || !strings.Contains(item.Documentation.Value, "Profile: SERVER") || !strings.Contains(item.Documentation.Value, "Direction:") {
		t.Fatalf("playerConnecting completion documentation = %#v", item.Documentation)
	}
}

func TestFiveMBuiltinSystemEventsSuppressFalsePositives(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("server.lua", "AddEventHandler('onResourceListRefresh', function() end)\nTriggerClientEvent('sessionHostResult', source, 'go')\n")
	h.writeWorkspaceFile("client.lua", "AddEventHandler('__cfx_internal:serverPrint', function() end)\nAddEventHandler('chat:addTemplate', function() end)\n")
	h.reindex()

	for _, relPath := range []string{"server.lua", "client.lua"} {
		diags := h.diagnostics(relPath)
		if hasDiagnosticCode(diags, "fivem-unknown-event") {
			t.Errorf("%s produced unknown-event warning for a host/system event: %#v", relPath, diags)
		}
		if hasDiagnosticCode(diags, "fivem-unregistered-net-event") {
			t.Errorf("%s produced unregistered-network warning for a built-in event: %#v", relPath, diags)
		}
	}
}
