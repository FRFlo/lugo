package lsp

import (
	"path/filepath"
	"testing"
)

func TestManifestDirectiveQuickFix(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"Client_script 'main.lua'\n", "client_script"},
		{"CreateThread 'main.lua'\n", ""},
	} {
		t.Run(tc.source, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("resource/fxmanifest.lua", tc.source)
			h.reindex()
			uri := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
			doc := h.server.Documents[uri]
			for _, d := range h.server.buildFiveMManifestDiagnostics(doc) {
				if d.Code != "fivem-manifest-unknown-directive" {
					continue
				}
				action, ok := h.server.manifestDirectiveQuickFix(doc, uri, d)
				if ok != (tc.want != "") {
					t.Fatalf("action = %#v, ok = %v", action, ok)
				}
				if ok && action.Edit.Changes[uri][0].NewText != tc.want {
					t.Fatalf("edit = %#v", action.Edit)
				}
				return
			}
			t.Fatal("missing unknown directive diagnostic")
		})
	}
}
