package lsp

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// fiveMAssetCaseFix rechecks both the current manifest and the filesystem. A
// partial walk or an ambiguous case-insensitive match cannot justify an edit.
func (s *Server) fiveMAssetCaseFix(doc *Document, uri string, diag Diagnostic) (CodeAction, bool) {
	if s == nil || doc == nil || !doc.IsFiveMManifest || (diag.Code != "fivem-asset-case-mismatch" && diag.Code != "fivem-asset-missing" && diag.Code != "fivem-asset-empty-glob") {
		return CodeAction{}, false
	}
	// Recompute the diagnostic so old client ranges cannot produce an edit.
	current := false
	for _, d := range s.buildFiveMAssetInventoryDiagnostics(doc) {
		if d.Code == diag.Code && d.Range == diag.Range {
			current = true
			break
		}
	}
	if !current {
		return CodeAction{}, false
	}
	res := s.parseFiveMManifest(doc)
	if res == nil || res.Manifest == nil {
		return CodeAction{}, false
	}
	var entry *FiveMManifestEntry
	for i := range res.Manifest.Entries {
		e := &res.Manifest.Entries[i]
		if e.ValueRange != diag.Range || e.LoaderInjected || e.ReservedKey {
			continue
		}
		switch e.EmittedName {
		case "ui_page", "client_script", "server_script", "shared_script", "file":
		default:
			continue
		}
		if entry != nil {
			return CodeAction{}, false
		}
		entry = e
	}
	if entry == nil || len(entry.RawValue) < 2 {
		return CodeAction{}, false
	}
	// Accept only plain quoted literals: preserving escaped or long-bracket
	// strings requires a complete Lua string encoder.
	quote := entry.RawValue[0]
	if (quote != '\'' && quote != '"') || entry.RawValue[len(entry.RawValue)-1] != quote || entry.RawValue[1:len(entry.RawValue)-1] != entry.Value || strings.ContainsAny(entry.Value, "\\\r\n") {
		return CodeAction{}, false
	}
	path := entry.Value
	if path == "" || strings.HasPrefix(path, "@") || fiveMAssetPathTraversal(path) {
		return CodeAction{}, false
	}
	root := s.uriToPath(res.RootURI)
	if root == "" {
		return CodeAction{}, false
	}
	var paths []string
	err := filepath.WalkDir(root, func(candidate string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// WalkDir does not follow symlinked directories, so this inventory
		// cannot establish uniqueness if a symlink is present.
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("incomplete asset inventory")
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, candidate)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return CodeAction{}, false
	}
	match := ""
	if diag.Code == "fivem-asset-case-mismatch" {
		for _, candidate := range paths {
			if !strings.EqualFold(path, candidate) {
				continue
			}
			if match != "" && match != candidate {
				return CodeAction{}, false
			}
			match = candidate
		}
	} else {
		// Empty globs have no reliably inferable intent: replacing one with a
		// concrete file would silently narrow the manifest. Missing literals
		// get a suggestion only when exactly one path is within edit distance 2.
		if diag.Code != "fivem-asset-missing" || fiveMAssetIsGlob(path) {
			return CodeAction{}, false
		}
		for _, candidate := range paths {
			if levenshteinFast(strings.ToLower(path), strings.ToLower(candidate), 2) > 2 {
				continue
			}
			if match != "" {
				return CodeAction{}, false
			}
			match = candidate
		}
	}
	if match == "" || match == path || strings.ContainsAny(match, string(quote)+"\\\r\n") {
		return CodeAction{}, false
	}
	return CodeAction{
		Title: fmt.Sprintf("Change manifest path to '%s'", match), Kind: "quickfix",
		Diagnostics: []Diagnostic{diag},
		Edit:        &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: diag.Range, NewText: string(quote) + match + string(quote)}}}},
	}, true
}
