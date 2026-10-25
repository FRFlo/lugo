package lsp

import "testing"

func TestFiveMResourceManagementStubsByRuntimeProfile(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_script 'client.lua'
server_script 'server.lua'
`)
	h.writeWorkspaceFile("client.lua", `local current = GetCurrentResourceName()
local count = GetNumResources()
local first = GetResourceByFindIndex(0)
local state = GetResourceState(current)
local metadataCount = GetNumResourceMetadata(current, 'client_script')
local metadata = GetResourceMetadata(current, 'client_script', 0)
local firstMetadata = GetResourceMetadata(current, 'client_script')
local invoking = GetInvokingResource()
local isServer = IsDuplicityVersion()
return count, first, state, metadataCount, metadata, firstMetadata, invoking, isServer
`)
	h.writeWorkspaceFile("server.lua", `local current = GetCurrentResourceName()
local first = GetResourceByFindIndex(0)
SetMapName('test map')
SetGameType('test type')
StartResource(current)
StopResource(current)
return first
`)
	h.reindex()

	for _, relPath := range []string{"client.lua", "server.lua"} {
		diags := h.diagnostics(relPath)
		for _, diag := range diags {
			if diag.Code == "undefined-global" || diag.Code == "redundant-parameter" {
				t.Errorf("%s reports missing or incorrect FiveM resource API signature: %#v", relPath, diag)
			}
		}
	}
}
