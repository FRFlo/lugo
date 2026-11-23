package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStateBagKeyQuickFix(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
	}{
		{"member", "LocalPlayer.state:set('Health', 1)\nlocal x = LocalPlayer.state.health\n", "Health"},
		{"string", "LocalPlayer.state:set('Health', 1)\nlocal x = LocalPlayer.state:get('health')\n", "'Health'"},
		{"ambiguous", "LocalPlayer.state:set('Health', 1)\nLocalPlayer.state:set('HEALTH', 2)\nlocal x = LocalPlayer.state.health\n", ""},
		{"different bags", "LocalPlayer.state:set('Health', 1)\nlocal x = GlobalState.health\n", ""},
		{"keyword", "LocalPlayer.state:set('end', 1)\nlocal x = LocalPlayer.state.End\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\n")
			h.writeWorkspaceFile("resource/shared.lua", tc.source)
			h.reindex()
			uri := h.server.pathToURI(filepath.Join(h.root, "resource", "shared.lua"))
			doc := h.server.Documents[uri]
			for _, d := range h.server.buildFiveMStateBagDiagnostics(doc) {
				if d.Code != "fivem-state-bag-inconsistent-key" {
					continue
				}
				action, ok := h.server.stateBagKeyQuickFix(doc, uri, d)
				if ok != (tc.want != "") {
					t.Fatalf("action = %#v, ok = %v, want %q", action, ok, tc.want)
				}
				if ok && action.Edit.Changes[uri][0].NewText != tc.want {
					t.Fatalf("edit = %#v", action.Edit)
				}
				if ok {
					d.Message += " stale"
					if _, stale := h.server.stateBagKeyQuickFix(doc, uri, d); stale {
						t.Fatal("accepted stale diagnostic")
					}
				}
				return
			}
			t.Fatal("missing inconsistent-key diagnostic")
		})
	}
}

func TestStateBagMissingKeySuggestion(t *testing.T) {
	for _, tc := range []struct {
		name, source, diagCode, query, want string
	}{
		{"same stable receiver", "LocalPlayer.state:set('health', 1)\nlocal x = LocalPlayer.state.healt\n", "fivem-state-bag-missing-key", "healt", "health"},
		{"ambiguous candidates", "LocalPlayer.state:set('healt', 1)\nLocalPlayer.state:set('helth', 2)\nlocal x = LocalPlayer.state.health\n", "fivem-state-bag-missing-key", "health", ""},
		{"different receiver", "GlobalState.health = 1\nlocal x = LocalPlayer.state.healt\n", "fivem-state-bag-missing-key", "healt", ""},
		{"dynamic receiver", "Entity(id).state:set('health', 1)\nlocal x = Entity(id).state.healt\n", "fivem-state-bag-missing-key", "healt", ""},
		{"global handler has no stable receiver", "GlobalState.health = 1\nAddStateBagChangeHandler('healt', nil, function() end)\n", "fivem-state-bag-unknown-key", "healt", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\n")
			h.writeWorkspaceFile("resource/shared.lua", tc.source)
			h.reindex()
			uri := h.server.pathToURI(filepath.Join(h.root, "resource", "shared.lua"))
			doc := h.server.Documents[uri]
			for _, d := range h.server.buildFiveMStateBagDiagnostics(doc) {
				if d.Code != tc.diagCode {
					continue
				}
				if tc.query != "" && !strings.Contains(d.Message, "'"+tc.query+"'") {
					continue
				}
				action, ok := h.server.stateBagKeyQuickFix(doc, uri, d)
				if ok != (tc.want != "") {
					t.Fatalf("action = %#v, ok = %v, want %q", action, ok, tc.want)
				}
				if ok && action.Edit.Changes[uri][0].NewText != tc.want {
					t.Fatalf("edit = %#v, want %q", action.Edit, tc.want)
				}
				return
			}
			t.Fatalf("missing diagnostic %q", tc.diagCode)
		})
	}
}
