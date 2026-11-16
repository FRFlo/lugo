package lsp

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coalaura/plain"
)

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
