package lsp

import (
	"slices"
	"strings"
	"testing"
)

func TestFiveMManifestAssetInventoryValidatesLocalFilesAndPaths(t *testing.T) {
	res := &FiveMResource{
		UIPage:      "web/index.html",
		SharedGlobs: []string{"asset.lua", "missing.lua", "web/*.css", "missing/*.js", ""},
	}
	inventory := NewFiveMAssetInventory([]string{
		"asset.lua",
		"web/index.html",
		"web/site.css",
	})

	issues := inventory.Validate(res)
	got := make([]string, len(issues))
	for i, issue := range issues {
		got[i] = string(issue.Kind) + ":" + issue.Path
	}
	want := []string{
		string(FiveMAssetIssueMissing) + ":missing.lua",
		string(FiveMAssetIssueEmptyGlob) + ":missing/*.js",
		string(FiveMAssetIssueEmptyGlob) + ":",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("asset issues = %#v, want %#v", got, want)
	}
}

func TestFiveMAssetMatchGlobHandlesAdversarialStarPattern(t *testing.T) {
	pattern := strings.Repeat("*a", 24) + "b"
	candidate := strings.Repeat("a", 24) + "c"

	if fiveMAssetMatchGlob(pattern, candidate) {
		t.Fatalf("fiveMAssetMatchGlob(%q, %q) = true, want false", pattern, candidate)
	}
}

func TestFiveMManifestAssetInventoryDiagnosticsUseManifestValueRanges(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
ui_page 'web/missing.html'
files { '', '../secret.lua', 'web/*.CSS', '@shared/common.lua' }
client_script 'client.lua'
`)
	h.writeWorkspaceFile("resource/client.lua", "return {}\n")
	h.writeWorkspaceFile("resource/web/site.css", "")
	h.reindex()

	diags := h.diagnostics("resource/fxmanifest.lua")
	want := map[string]int{
		"fivem-asset-missing":        1,
		"fivem-asset-empty-glob":     2,
		"fivem-asset-path-traversal": 2,
		"fivem-asset-case-mismatch":  2,
	}
	for _, diag := range diags {
		line, ok := want[diag.Code]
		if !ok {
			continue
		}
		if line == -1 {
			t.Errorf("duplicate %s diagnostic: %+v", diag.Code, diag)
			continue
		}
		if diag.Range.Start.Line != uint32(line) {
			t.Errorf("%s location line = %d, want %d (diagnostic: %+v)", diag.Code, diag.Range.Start.Line, line, diag)
		}
		want[diag.Code] = -1
	}
	for code, line := range want {
		if line != -1 {
			t.Errorf("missing %s diagnostic (all diagnostics: %#v)", code, diags)
		}
	}
}

func TestFiveMManifestAssetInventoryRejectsTraversalAndWrongCase(t *testing.T) {
	res := &FiveMResource{
		UIPage:      "Web/index.html",
		SharedGlobs: []string{"../secret.lua", "web/*.CSS"},
	}
	inventory := NewFiveMAssetInventory([]string{"web/index.html", "web/site.css", "secret.lua"})

	issues := inventory.Validate(res)
	got := make([]string, len(issues))
	for i, issue := range issues {
		got[i] = string(issue.Kind) + ":" + issue.Path
	}
	want := []string{
		string(FiveMAssetIssueCaseMismatch) + ":Web/index.html",
		string(FiveMAssetIssuePathTraversal) + ":../secret.lua",
		string(FiveMAssetIssueCaseMismatch) + ":web/*.CSS",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("asset issues = %#v, want %#v", got, want)
	}
}
