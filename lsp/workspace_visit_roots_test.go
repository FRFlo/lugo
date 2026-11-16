package lsp

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/coalaura/plain"
)

func TestWorkspaceTraversalSharesVisitedDirectoriesAcrossRoots(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "mod.lua")
	if err := os.WriteFile(file, []byte("return 1"), 0644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(nested, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.Writer = io.Discard
	s.activeURIs = make(map[string]bool)
	var jobs []*IndexJob
	var unchanged, failed int
	for _, dir := range []string{alias, root, nested} {
		s.indexWorkspace(dir, &jobs, &unchanged, &failed)
	}
	if len(jobs) != 1 || jobs[0].Uri != s.pathToURI(file) || len(s.visitedDirs) != 2 || failed != 0 {
		t.Fatalf("jobs=%d visited=%v failures=%d", len(jobs), s.visitedDirs, failed)
	}
}

func TestRefreshWorkspaceResetsVisitedDirectories(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("first.lua", "return 1")
	h.server.refreshWorkspace()
	h.writeWorkspaceFile("second.lua", "return 2")
	h.server.refreshWorkspace()
	uri := h.server.pathToURI(filepath.Join(h.root, "second.lua"))
	if h.server.Documents[uri] == nil {
		t.Fatal("second refresh did not visit the workspace again")
	}
}
