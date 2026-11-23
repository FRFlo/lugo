package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestCleanupQuickFix(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, code string
		want                 bool
	}{
		{"self", "dependency 'resource'\n", "fivem-self-dependency", true},
		{"duplicate", "export 'foo'\nexport 'foo'\n", "fivem-duplicate-export", true},
		{"sides", "client_export 'foo'\nserver_export 'foo'\n", "fivem-duplicate-export", false},
		{"list", "dependencies { 'resource' }\n", "fivem-self-dependency", true},
		{"list preserves comment", "dependencies { 'resource', -- keep this\n 'other' }\n", "fivem-self-dependency", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n"+tc.manifest)
			h.reindex()
			uri := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
			doc := h.server.Documents[uri]
			for _, d := range h.diagnostics("resource/fxmanifest.lua") {
				if d.Code != tc.code {
					continue
				}
				a, ok := h.server.manifestCleanupQuickFix(doc, uri, d)
				if ok != tc.want {
					t.Fatalf("action=%#v ok=%v diag=%#v", a, ok, d)
				}
				if ok && strings.HasPrefix(tc.name, "list") {
					edit := a.Edit.Changes[uri][0]
					start := doc.Tree.Offset(edit.Range.Start.Line, edit.Range.Start.Character)
					end := doc.Tree.Offset(edit.Range.End.Line, edit.Range.End.Character)
					updated := string(doc.Source()[:start]) + edit.NewText + string(doc.Source()[end:])
					if strings.Contains(updated, "'resource'") {
						t.Fatalf("self dependency remains after edit: %q", updated)
					}
					if tc.name == "list preserves comment" && !strings.Contains(updated, "-- keep this") {
						t.Fatalf("comment was not preserved: %q", updated)
					}
				}
				return
			}
			t.Fatal("missing diagnostic")
		})
	}
}
