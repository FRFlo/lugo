package lsp

import (
	"path/filepath"
	"testing"
)

func TestFiveMCommandAndConvarQuickFixesThroughCodeAction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\nconvar 'feature_mode'\n")
	h.writeWorkspaceFile("resource/server.lua", "RegisterKeyMapping('inspect', 'Inspect', 'keyboard', 'I')\nGetConvar('feature_mdoe', '')\n")
	h.reindex()

	var diagnostics []Diagnostic
	for _, diag := range h.diagnostics("resource/server.lua") {
		if diag.Code == "fivem-command-missing-declaration" || diag.Code == "fivem-convar-unknown" {
			diagnostics = append(diagnostics, diag)
		}
	}
	if len(diagnostics) != 2 {
		t.Fatalf("got %d target diagnostics, want command and convar: %#v", len(diagnostics), diagnostics)
	}
	actions := h.codeActions("resource/server.lua", Range{}, diagnostics)
	var command, convar bool
	for _, action := range actions {
		switch action.Title {
		case "Declare 'inspect' as restricted (ACE permission required)", "Declare 'inspect' as unrestricted":
			command = action.Edit != nil
		case "Use existing convar 'feature_mode'", "Declare convar 'feature_mdoe' in fxmanifest.lua":
			convar = action.Edit != nil
		}
	}
	if !command || !convar {
		t.Fatalf("code action route did not return both helpers: command=%v convar=%v actions=%#v", command, convar, actions)
	}
	manifestURI := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
	foundManifestEdit := false
	for _, action := range actions {
		if action.Title == "Declare convar 'feature_mdoe' in fxmanifest.lua" {
			foundManifestEdit = len(action.Edit.Changes[manifestURI]) == 1
		}
	}
	if !foundManifestEdit {
		t.Fatal("convar declaration code action should edit the manifest through the LSP route")
	}
}
