package lsp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coalaura/plain"
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

func TestIndexWorkspaceOverlappingRoots(t *testing.T) {
	root := t.TempDir()
	library := filepath.Join(root, "library")
	if err := os.Mkdir(library, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(library, "module.lua")
	if err := os.WriteFile(file, []byte("return 1"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.Writer = io.Discard
	uri := s.pathToURI(file)
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold", true: "changed existing document"}[warm], func(t *testing.T) {
			if warm {
				s.updateDocument(uri, []byte("return 0"))
				s.Documents[uri].ModTime = time.Time{}
			}
			s.activeURIs = make(map[string]bool)
			s.visitedDirs = make(map[string]bool)
			var jobs []*IndexJob
			var unchanged, failed int
			for _, dir := range []string{library, root, library} {
				s.indexWorkspace(dir, &jobs, &unchanged, &failed)
			}
			if len(jobs) != 1 || jobs[0].Uri != uri {
				t.Fatalf("expected one job for %s, got %#v", uri, jobs)
			}
			if !s.activeURIs[uri] || failed != 0 {
				t.Fatalf("active = %t, failures = %d", s.activeURIs[uri], failed)
			}
		})
	}
}
