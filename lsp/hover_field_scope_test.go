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
	h.writeWorkspaceFile("provider/fields.lua", "ESX.PlayerData = { job = { grade = { name = 'police' } } }\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'main.lua', 'utils.lua' }\n")
	h.writeWorkspaceFile("consumer/main.lua", "ESX = nil\nprint(ESX.Player--[[@global]]Data)\nprint(ESX.PlayerData.j--[[@job]]ob.grade)\nprint(ESX.PlayerData.job.gr--[[@grade]]ade.name)\ndo\n local ESX = nil\n print(ESX.Player--[[@local]]Data)\nend\nlocal data = { job = { grade = { name = 'police' } } }\nprint(data.j--[[@local_job]]ob.grade)\nprint(data.job.gr--[[@local_grade]]ade.name)\n")
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
	for marker, want := range map[string]string{"job": "grade: table", "grade": "name: string", "local_job": "grade: table", "local_grade": "name: string"} {
		hover := h.hover(marker)
		if hover == nil || !strings.Contains(hover.Contents.Value, want) {
			t.Errorf("%s hover = %#v, want %q", marker, hover, want)
		}
	}
}
