package lsp

import (
	"strings"
	"testing"
)

func BenchmarkHoverFieldDetail(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_scripts { 'common.lua', 'event.lua', 'fields.lua' }\n")
	h.writeWorkspaceFile("provider/common.lua", "ESX = {}\n")
	h.writeWorkspaceFile("provider/event.lua", "AddEventHandler('esx:getSharedObject', function(cb) cb(ESX) end)\n")
	h.writeWorkspaceFile("provider/fields.lua", "ESX.PlayerData = { job = { name = 'police' } }\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'main.lua', 'utils.lua' }\n")
	h.writeWorkspaceFile("consumer/main.lua", "ESX = nil\nprint(ESX.Player--[[@global]]Data)\nlocal localValue = {}\nprint(localValue.mis--[[@local]]sing)\n")
	h.writeWorkspaceFile("consumer/utils.lua", "TriggerEvent('esx:getSharedObject', function(obj) ESX = obj end)\n")
	h.reindex()
	for _, markerName := range []string{"global", "local"} {
		b.Run(markerName, func(b *testing.B) {
			marker := h.requireMarker(markerName)
			doc := h.docForMarker(markerName)
			ctx := h.server.resolveSymbolAt(marker.URI, uint32(marker.Offset))
			detail, _ := h.server.hoverTableDetail(doc, ctx, uint32(marker.Offset), 1, TypeSet{})
			if markerName == "global" && !strings.Contains(detail, "job: table") {
				b.Fatalf("missing PlayerData fields: %q", detail)
			}
			b.ReportAllocs()
			for b.Loop() {
				h.server.hoverTableDetail(doc, ctx, uint32(marker.Offset), 1, TypeSet{})
			}
		})
	}
}
