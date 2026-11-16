package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetLibraryPathsComparesResolvedPaths(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "library")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	var s Server
	if !s.setLibraryPaths([]string{link}) {
		t.Fatal("initial path should change configuration")
	}
	if s.setLibraryPaths([]string{link}) {
		t.Fatal("unchanged symlink should not change configuration")
	}
	if s.setLibraryPaths([]string{target}) {
		t.Fatal("resolved equivalent should not change configuration")
	}
	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	if !s.setLibraryPaths([]string{link}) || s.setLibraryPaths([]string{link}) {
		t.Fatal("retargeted symlink should change once, then remain stable")
	}
	missing := filepath.Join(root, "missing")
	if !s.setLibraryPaths([]string{missing}) || s.setLibraryPaths([]string{missing}) {
		t.Fatal("missing path should change once, then remain stable")
	}
	if err := os.Mkdir(missing, 0755); err != nil {
		t.Fatal(err)
	}
	if !s.setLibraryPaths([]string{link}) {
		t.Fatal("different path should change configuration")
	}
}

func TestKnownGlobalsChangeOnlyRepublishes(t *testing.T) {
	s := NewServer("test")
	opts := defaultInitializationOptions()
	s.applyInitializationOptions(opts)
	opts.KnownGlobals = []string{"MyGlobal", "framework.*"}
	reindex, republish := s.applyInitializationOptions(opts)
	if reindex || !republish {
		t.Fatalf("known globals change: reindex=%v republish=%v", reindex, republish)
	}
	if !s.isKnownGlobal(nil, []byte("MyGlobal")) || !s.isKnownGlobal(nil, []byte("framework.foo")) {
		t.Fatal("known globals not applied immediately")
	}
	reindex, republish = s.applyInitializationOptions(opts)
	if reindex || republish {
		t.Fatalf("unchanged configuration: reindex=%v republish=%v", reindex, republish)
	}
}
