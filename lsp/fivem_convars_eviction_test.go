package lsp

import "testing"

func TestRefreshWorkspaceAfterConvarSourceEviction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", `SetConvar("mode", "fast")\nGetConvar("mode", "")\n`)
	h.server.refreshWorkspace()

	evictClosedDocumentCaches(h.server)
	h.server.refreshWorkspace()
}
