package lsp

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLibraryPathsDeduplicateResolvedPathsPreservingOrder(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	for _, dir := range []string{first, second} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(first, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var s Server
	if !s.setLibraryPaths([]string{first, second}) {
		t.Fatal("initial paths must change")
	}
	if s.setLibraryPaths([]string{alias, first, second, alias}) {
		t.Fatal("duplicate aliases must not trigger refresh")
	}
	if !slices.Equal(s.LibraryPaths, []string{first, second}) {
		t.Fatalf("paths = %v", s.LibraryPaths)
	}
	if !s.setLibraryPaths([]string{second, alias, first}) {
		t.Fatal("a real priority change must trigger refresh")
	}
	if !slices.Equal(s.LibraryPaths, []string{second, first}) {
		t.Fatalf("reordered paths = %v", s.LibraryPaths)
	}
}
