package lsp

import (
	"bytes"
	"path/filepath"
	"strings"
)

func isFiveMManifestPathDirective(name string) bool {
	switch strings.ToLower(name) {
	case "client_script", "client_scripts", "server_script", "server_scripts", "shared_script", "shared_scripts", "file", "files", "ui_page", "loadscreen", "loadscreen_manual_shutdown", "replace", "data_file", "this_is_a_map", "audio_wave", "before_level_meta", "after_level_meta", "resource_manifest_version":
		return true
	default:
		return false
	}
}

func (s *Server) manifestPathTarget(doc *Document, entry FiveMManifestEntry) string {
	if doc == nil || entry.Value == "" || strings.HasPrefix(entry.Value, "@") || !isFiveMManifestPathDirective(entry.EmittedName) {
		return ""
	}
	manifestPath := s.uriToPath(doc.URI)
	if manifestPath == "" {
		return ""
	}
	return s.pathToURI(filepath.Clean(filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(entry.Value))))
}

func (s *Server) manifestPathCompletions(doc *Document, offset uint32) []CompletionItem {
	source := doc.Source()
	if offset > uint32(len(source)) {
		return nil
	}
	start := bytes.LastIndex(source[:offset], []byte("'"))
	if double := bytes.LastIndex(source[:offset], []byte("\"")); double > start {
		start = double
	}
	if start < 0 {
		return nil
	}
	lineStart := bytes.LastIndex(source[:start], []byte("\n")) + 1
	line := string(source[lineStart:start])
	pathDirective := false
	for _, name := range []string{"client_script", "client_scripts", "server_script", "server_scripts", "shared_script", "shared_scripts", "file", "files", "ui_page", "loadscreen"} {
		if strings.Contains(line, name) {
			pathDirective = true
			break
		}
	}
	if !pathDirective {
		return nil
	}
	prefix := string(source[start+1 : offset])
	root := filepath.Dir(s.uriToPath(doc.URI))
	items := make([]CompletionItem, 0)
	seen := make(map[string]bool)
	for uri := range s.Documents {
		rel, err := filepath.Rel(root, s.uriToPath(uri))
		if err != nil || strings.HasPrefix(rel, "..") || filepath.Ext(rel) == "" {
			continue
		}
		label := filepath.ToSlash(rel)
		if strings.HasPrefix(label, prefix) && !seen[label] {
			seen[label] = true
			items = append(items, CompletionItem{Label: label, Kind: FieldCompletion, Detail: "manifest path", InsertText: label})
		}
	}
	return items
}
