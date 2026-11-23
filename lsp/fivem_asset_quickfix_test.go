package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFiveMAssetCaseFix(t *testing.T) {
	for _, tc := range []struct {
		name, literal string
		files         []string
		want          string
	}{
		{"single quote", "'Web/index.html'", []string{"web/index.html"}, "'web/index.html'"},
		{"double quote", `"Web/index.html"`, []string{"web/index.html"}, `"web/index.html"`},
		{"ambiguous", "'WEB/index.html'", []string{"web/index.html", "Web/index.html"}, ""},
		{"glob", "'Web/*.HTML'", []string{"web/index.html"}, ""},
		{"external", "'@shared/file.lua'", []string{"shared/file.lua"}, ""},
		{"traversal", "'../Web/index.html'", []string{"web/index.html"}, ""},
		{"escaped literal", `'Web/\x69ndex.html'`, []string{"web/index.html"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\nui_page "+tc.literal+"\n")
			for _, file := range tc.files {
				h.writeWorkspaceFile("resource/"+file, "")
			}
			h.reindex()
			uri := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
			doc := h.server.Documents[uri]
			res := h.server.parseFiveMManifest(doc)
			var diag Diagnostic
			for _, e := range res.Manifest.Entries {
				if e.EmittedName == "ui_page" {
					diag = Diagnostic{Code: "fivem-asset-case-mismatch", Range: e.ValueRange}
				}
			}
			action, ok := h.server.fiveMAssetCaseFix(doc, uri, diag)
			if ok != (tc.want != "") {
				t.Fatalf("fix available = %v, want %v", ok, tc.want != "")
			}
			if ok && action.Edit.Changes[uri][0].NewText != tc.want {
				t.Fatalf("replacement = %q, want %q", action.Edit.Changes[uri][0].NewText, tc.want)
			}
			if ok {
				diag.Range.Start.Character++
				if _, stale := h.server.fiveMAssetCaseFix(doc, uri, diag); stale {
					t.Fatal("accepted stale range")
				}
			}
		})
	}
}

func TestFiveMAssetCaseFixRejectsIncompleteInventory(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "ui_page 'Web/index.html'\n")
	h.writeWorkspaceFile("resource/web/index.html", "")
	if err := os.Symlink("web", filepath.Join(h.root, "resource", "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
	doc := h.server.Documents[uri]
	res := h.server.parseFiveMManifest(doc)
	for _, e := range res.Manifest.Entries {
		if e.EmittedName == "ui_page" {
			if _, ok := h.server.fiveMAssetCaseFix(doc, uri, Diagnostic{Code: "fivem-asset-case-mismatch", Range: e.ValueRange}); ok {
				t.Fatal("accepted incomplete inventory")
			}
			return
		}
	}
	t.Fatal("missing manifest entry")
}

func TestFiveMAssetTypoSuggestion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		path  string
		want  string
	}{
		{name: "unique near path", files: []string{"web/index.html"}, path: "web/index.htm", want: "'web/index.html'"},
		{name: "ambiguous near paths", files: []string{"web/index.htm", "web/index.html"}, path: "web/index.ht", want: ""},
		{name: "empty glob", files: []string{"web/index.html"}, path: "web/*.js", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\nui_page '"+tc.path+"'\n")
			for _, file := range tc.files {
				h.writeWorkspaceFile("resource/"+file, "")
			}
			h.reindex()
			uri := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
			doc := h.server.Documents[uri]
			var diag Diagnostic
			for _, d := range h.server.buildFiveMAssetInventoryDiagnostics(doc) {
				if d.Code == "fivem-asset-missing" || d.Code == "fivem-asset-empty-glob" {
					diag = d
					break
				}
			}
			if diag.Code == "" {
				t.Fatal("missing asset diagnostic")
			}
			action, ok := h.server.fiveMAssetCaseFix(doc, uri, diag)
			if ok != (tc.want != "") {
				t.Fatalf("fix available = %v, want %v", ok, tc.want != "")
			}
			if ok && action.Edit.Changes[uri][0].NewText != tc.want {
				t.Fatalf("replacement = %q, want %q", action.Edit.Changes[uri][0].NewText, tc.want)
			}
		})
	}
}

func TestFiveMAssetTypoSuggestionRejectsIncompleteInventory(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "ui_page 'web/index.htm'\n")
	h.writeWorkspaceFile("resource/web/index.html", "")
	if err := os.Symlink("web", filepath.Join(h.root, "resource", "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
	doc := h.server.Documents[uri]
	for _, d := range h.server.buildFiveMAssetInventoryDiagnostics(doc) {
		if d.Code == "fivem-asset-missing" {
			if _, ok := h.server.fiveMAssetCaseFix(doc, uri, d); ok {
				t.Fatal("suggested path from incomplete inventory")
			}
			return
		}
	}
	t.Fatal("missing asset diagnostic")
}
