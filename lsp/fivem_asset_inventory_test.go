package lsp

import (
	"slices"
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
