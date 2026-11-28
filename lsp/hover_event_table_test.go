package lsp

import (
	"strings"
	"testing"
)

func TestFiveMEventCallbackTableHover(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'common.lua'\nclient_scripts { 'event.lua', 'fields.lua' }\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("provider/common.lua", "ESX = {}\n")
	h.writeWorkspaceFile("provider/event.lua", "AddEventHandler('esx:getSharedObject', function(cb) cb(ESX) end)\n")
	h.writeWorkspaceFile("provider/fields.lua", "ESX.PlayerData = { ready = true }\nfunction ESX.GetPlayerData() return ESX.PlayerData end\n")
	h.writeWorkspaceFile("provider/server.lua", "ESX.ServerOnly = true\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'main.lua', 'utils.lua' }\n")
	h.writeWorkspaceFile("consumer/main.lua", "ES--[[@esx_initializer]]X = nil\nprint(ES--[[@esx_hover]]X)\n")
	h.writeWorkspaceFile("consumer/utils.lua", "local function loadESX()\n  while ESX == nil do\n    TriggerEvent('esx:getSharedObject', function(obj) ESX = obj end)\n    Wait(0)\n  end\nend\n")
	h.reindex()

	for _, marker := range []string{"esx_initializer", "esx_hover"} {
		t.Run(marker, func(t *testing.T) {
			hover := h.hover(marker)
			if hover == nil || hover.LugoTable == nil || !hover.LugoTable.More {
				t.Fatalf("expected expandable ESX hover, got %#v", hover)
			}
			if !strings.HasPrefix(hover.Contents.Value, "```lua\nglobal ESX: ") ||
				strings.Count(hover.Contents.Value, "```lua") != 1 ||
				strings.Contains(hover.Contents.Value, "Type details") {
				t.Fatalf("expected fields in the ESX definition: %s", hover.Contents.Value)
			}
			if !strings.Contains(hover.Contents.Value, "global ESX: {\n") ||
				strings.Contains(hover.Contents.Value, "any {") {
				t.Fatalf("table shape must replace table in the union: %s", hover.Contents.Value)
			}
			for _, want := range []string{"PlayerData: table", "GetPlayerData:"} {
				if !strings.Contains(hover.Contents.Value, want) {
					t.Errorf("hover missing %q: %s", want, hover.Contents.Value)
				}
			}
			if strings.Contains(hover.Contents.Value, "ServerOnly") {
				t.Errorf("client hover includes server field: %s", hover.Contents.Value)
			}
		})
	}
}
