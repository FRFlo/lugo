package lsp

import (
	"encoding/json"
	"testing"
)

func TestDebugExportEmptyGlobalIndexMatchesOriginalJSON(t *testing.T) {
	s := NewServer("test")
	s.GlobalIndex = NewGlobalIndex()
	categories := []string{debugExportCategoryGlobalIndex}
	got, err := s.buildDebugExport(DebugExportParams{Categories: categories})
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.MarshalIndent(debugExportPayload{
		Metadata:    debugExportMetadata{ServerVersion: s.Version, RootURI: s.RootURI, WorkspaceFolders: s.WorkspaceFolders, Categories: categories, DocumentCount: 0, IsIndexing: s.IsIndexing},
		Documents:   []debugExportDocument{},
		GlobalIndex: s.exportDebugGlobalIndex(),
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("empty index export differs from original JSON\ngot: %s\nwant: %s", got, want)
	}
}

func TestDebugExportGlobalIndexIgnoresNilEntries(t *testing.T) {
	s := NewServer("test")
	s.GlobalIndex = NewGlobalIndex()
	s.GlobalIndex.Resources["resource"] = &ResourceScope{
		Client: SymbolTable{"missing": nil, "present": &SymbolEntry{}},
		Server: SymbolTable{"alsoMissing": nil},
	}
	categories := []string{debugExportCategoryGlobalIndex}
	want, err := json.MarshalIndent(debugExportPayload{
		Metadata:  debugExportMetadata{ServerVersion: s.Version, RootURI: s.RootURI, WorkspaceFolders: s.WorkspaceFolders, Categories: categories, DocumentCount: 0, IsIndexing: s.IsIndexing},
		Documents: []debugExportDocument{}, GlobalIndex: s.exportDebugGlobalIndex(),
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	s.MaxFileSize = int64(len(want))
	got, err := s.buildDebugExport(DebugExportParams{Categories: categories})
	if err != nil || got != string(want) {
		t.Fatalf("export with nil entries: err=%v\ngot: %s\nwant: %s", err, got, want)
	}
}
