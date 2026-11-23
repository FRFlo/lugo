package lsp

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiagnosticQuickFixesThroughCodeAction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "Client_script 'shared.lua'\nui_page 'Web/index.html'\n")
	h.writeWorkspaceFile("resource/web/index.html", "")
	source := "LocalPlayer.state:set('Health', 1)\nlocal x = LocalPlayer.state.health\nlocal function f() return ... end\n"
	h.writeWorkspaceFile("resource/shared.lua", source)
	h.reindex()
	manifestURI := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
	manifest := h.server.Documents[manifestURI]
	luaURI := h.server.pathToURI(filepath.Join(h.root, "resource", "shared.lua"))
	doc := h.server.Documents[luaURI]
	var directive, state, asset Diagnostic
	for _, d := range h.server.buildFiveMManifestDiagnostics(manifest) {
		if d.Code == "fivem-manifest-unknown-directive" {
			directive = d
			break
		}
	}
	for _, d := range h.server.buildFiveMStateBagDiagnostics(doc) {
		if d.Code == "fivem-state-bag-inconsistent-key" {
			state = d
			break
		}
	}
	for _, d := range h.diagnostics("resource/fxmanifest.lua") {
		if d.Code == "fivem-asset-case-mismatch" {
			asset = d
			break
		}
	}
	idx := strings.Index(source, "...")
	lua := Diagnostic{Code: "incorrect-vararg", Range: getRange(doc.Tree, uint32(idx), uint32(idx+3))}
	cases := []struct {
		name, path string
		diag       Diagnostic
		expected   CodeAction
		ok         bool
	}{}
	action, ok := luaDiagnosticQuickFix(doc, luaURI, lua)
	cases = append(cases, struct {
		name, path string
		diag       Diagnostic
		expected   CodeAction
		ok         bool
	}{"lua", "resource/shared.lua", lua, action, ok})
	action, ok = h.server.stateBagKeyQuickFix(doc, luaURI, state)
	cases = append(cases, struct {
		name, path string
		diag       Diagnostic
		expected   CodeAction
		ok         bool
	}{"state bag", "resource/shared.lua", state, action, ok})
	action, ok = h.server.fiveMAssetCaseFix(manifest, manifestURI, asset)
	cases = append(cases, struct {
		name, path string
		diag       Diagnostic
		expected   CodeAction
		ok         bool
	}{"asset", "resource/fxmanifest.lua", asset, action, ok})
	action, ok = h.server.manifestDirectiveQuickFix(manifest, manifestURI, directive)
	cases = append(cases, struct {
		name, path string
		diag       Diagnostic
		expected   CodeAction
		ok         bool
	}{"directive", "resource/fxmanifest.lua", directive, action, ok})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.ok {
				t.Fatal("fixture did not produce a fix")
			}
			for _, got := range h.codeActions(tc.path, tc.diag.Range, []Diagnostic{tc.diag}) {
				if got.Title == tc.expected.Title && got.Kind == "quickfix" && reflect.DeepEqual(got.Edit, tc.expected.Edit) {
					return
				}
			}
			t.Fatalf("missing action %q", tc.expected.Title)
		})
	}
}

func TestEventQuickFixThroughCodeAction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	source := "TriggerServerEvent('custom:event', 7)\n"
	h.writeWorkspaceFile("resource/server.lua", source)
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "server.lua"))
	doc := h.server.Documents[uri]
	for _, diag := range h.diagnostics("resource/server.lua") {
		if diag.Code != "fivem-event-direction" {
			continue
		}
		expected, ok := h.server.eventDirectionQuickFix(doc, uri, diag)
		if !ok {
			t.Fatal("fixture did not produce an event fix")
		}
		for _, action := range h.codeActions("resource/server.lua", diag.Range, []Diagnostic{diag}) {
			if action.Title == expected.Title && action.Kind == "quickfix" && reflect.DeepEqual(action.Edit, expected.Edit) {
				return
			}
		}
		t.Fatalf("missing event action %q with expected edit", expected.Title)
	}
	t.Fatal("missing event-direction diagnostic")
}

func TestOALVectorQuickFixThroughCodeAction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nuse_experimental_fxv2_oal 'yes'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "local pos = vector3(1, 2, 3)\nSetEntityCoords(PlayerPedId(), pos)\n")
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "client.lua"))
	doc := h.server.Documents[uri]
	for _, diag := range h.diagnostics("resource/client.lua") {
		if diag.Code != "fivem-oal-vector-argument" {
			continue
		}
		expected, ok := h.server.oalVectorQuickFix(doc, uri, diag)
		if !ok {
			t.Fatal("fixture did not produce an OAL vector fix")
		}
		for _, action := range h.codeActions("resource/client.lua", diag.Range, []Diagnostic{diag}) {
			if action.Title == expected.Title && action.Kind == "quickfix" && reflect.DeepEqual(action.Edit, expected.Edit) {
				return
			}
		}
		t.Fatalf("missing OAL vector action %q with expected edit", expected.Title)
	}
	t.Fatal("missing OAL vector diagnostic")
}

func TestManifestCleanupThroughCodeAction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'resource'\n")
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
	doc := h.server.Documents[uri]
	for _, diag := range h.diagnostics("resource/fxmanifest.lua") {
		if diag.Code != "fivem-self-dependency" {
			continue
		}
		expected, ok := h.server.manifestCleanupQuickFix(doc, uri, diag)
		if !ok {
			t.Fatal("fixture did not produce a fix")
		}
		for _, action := range h.codeActions("resource/fxmanifest.lua", diag.Range, []Diagnostic{diag}) {
			if action.Title == expected.Title && action.Kind == "quickfix" && reflect.DeepEqual(action.Edit, expected.Edit) {
				return
			}
		}
		t.Fatalf("missing action %q", expected.Title)
	}
	t.Fatal("missing self-dependency diagnostic")
}
