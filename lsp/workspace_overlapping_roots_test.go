package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRefreshWorkspaceDeduplicatesOverlappingRoots(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	var source strings.Builder
	for i := 0; i < 1500; i++ {
		fmt.Fprintf(&source, "local value%d = { key = %d, nested = { flag = true } }\n", i, i)
	}
	rel := "nested/overlap.lua"
	h.writeWorkspaceFile(rel, source.String()+"-- generation 0\n")
	h.reindex()

	path := filepath.Join(h.root, filepath.FromSlash(rel))
	if err := os.WriteFile(path, []byte(source.String()+"-- generation 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	nestedRoot := filepath.Join(h.root, "nested")
	h.server.WorkspaceFolders = []string{h.server.pathToURI(h.root), h.server.pathToURI(nestedRoot)}
	h.server.lowerWorkspaceFolders = []string{strings.ToLower(filepath.Clean(h.root)), strings.ToLower(filepath.Clean(nestedRoot))}

	h.server.refreshWorkspace()

	uri := h.server.pathToURI(path)
	doc := h.server.Documents[uri]
	if doc == nil || !strings.Contains(string(doc.Source()), "generation 1") {
		t.Fatalf("overlapping-root refresh did not publish new source: doc=%v", doc)
	}
}
