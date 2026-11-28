package lsp

import (
	"strings"
	"testing"
)

func TestHoverFieldRespectsLocalShadow(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_scripts { 'common.lua', 'event.lua', 'fields.lua' }\n")
	h.writeWorkspaceFile("provider/common.lua", "ESX = {}\n")
	h.writeWorkspaceFile("provider/event.lua", "AddEventHandler('esx:getSharedObject', function(cb) cb(ESX) end)\n")
	h.writeWorkspaceFile("provider/fields.lua", "ESX.PlayerData = { job = { name = 'police' } }\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'main.lua', 'utils.lua' }\n")
	h.writeWorkspaceFile("consumer/main.lua", "ESX = nil\nprint(ESX.Player--[[@global]]Data)\ndo\n local ESX = nil\n print(ESX.Player--[[@local]]Data)\nend\n")
	h.writeWorkspaceFile("consumer/utils.lua", "TriggerEvent('esx:getSharedObject', function(obj) ESX = obj end)\n")
	h.reindex()
	global := h.hover("global")
	if global == nil || !strings.Contains(global.Contents.Value, "job: table") {
		t.Fatalf("global hover = %#v, want PlayerData fields", global)
	}
	local := h.hover("local")
	if local != nil && strings.Contains(local.Contents.Value, "job: table") {
		t.Fatalf("local hover includes global PlayerData fields: %#v", local)
	}
}
