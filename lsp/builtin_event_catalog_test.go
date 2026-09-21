package lsp

import (
	"strings"
	"testing"
)

func TestFiveMBuiltinEventCatalogCompletionAndHover(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", "AddEventHandler(\"--[[@hover_connecting]]playerConnecting\", function() end)\nTriggerServerEvent(\"--[[@completion]]\")\nAddEventHandler(--[[@hover_unquoted]]playerDropped, function() end)\n")
	h.reindex()

	for _, tc := range []struct {
		name        string
		marker      string
		description string
		profile     string
	}{
		{"playerConnecting", "hover_connecting", "connecting to the server", "SERVER"},
		{"playerDropped", "hover_unquoted", "disconnects", "SERVER"},
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
