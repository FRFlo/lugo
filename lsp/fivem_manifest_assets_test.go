package lsp

import "testing"

func TestFiveMManifestFilesDoNotClassifyLuaAssetsAsScripts(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, root+"/resource/fxmanifest.lua", `
fx_version "cerulean"
files { "asset.lua" }
shared_script "shared.lua"
`)

	asset := addFiveMTestDocument(t, s, root+"/resource/asset.lua", "return {}")
	shared := addFiveMTestDocument(t, s, root+"/resource/shared.lua", "return {}")

	if profile := s.getDocumentFiveMProfile(asset); profile.Kind != FiveMProfilePlainLua {
		t.Fatalf("files Lua asset profile = %s, want %s", profile.Kind.String(), FiveMProfilePlainLua.String())
	}
	if profile := s.getDocumentFiveMProfile(shared); profile.Kind != FiveMProfileShared {
		t.Fatalf("shared_script profile = %s, want %s", profile.Kind.String(), FiveMProfileShared.String())
	}
}
