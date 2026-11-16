package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceClassificationRespectsDirectoryBoundaries(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "app")
	library := filepath.Join(workspace, "lib")
	s := NewServer("test")
	s.WorkspaceFolders = []string{s.pathToURI(workspace)}
	s.lowerWorkspaceFolders = []string{strings.ToLower(workspace)}
	s.LibraryPaths = []string{library}
	s.lowerLibraryPaths = []string{strings.ToLower(library)}
	for _, tc := range []struct {
		name               string
		path               string
		library, workspace bool
		module             string
	}{
		{"library child", filepath.Join(library, "mod.lua"), true, false, "mod"},
		{"library sibling prefix", filepath.Join(workspace, "library", "mod.lua"), false, true, "library.mod"},
		{"workspace sibling prefix", filepath.Join(root, "application", "mod.lua"), false, false, "mod"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uri, lower := s.pathToURI(tc.path), strings.ToLower(tc.path)
			if got := s.checkIsLibrary(uri, lower); got != tc.library {
				t.Errorf("library = %v, want %v", got, tc.library)
			}
			if got := s.checkIsWorkspace(uri, lower); got != tc.workspace {
				t.Errorf("workspace = %v, want %v", got, tc.workspace)
			}
			if got := s.computeModuleName(uri, tc.path, lower); got != tc.module {
				t.Errorf("module = %q, want %q", got, tc.module)
			}
		})
	}
	volumeRoot := filepath.VolumeName(workspace) + string(filepath.Separator)
	if !pathWithinDirectory(filepath.Join(volumeRoot, "child"), volumeRoot) {
		t.Fatalf("filesystem root %q must contain child", volumeRoot)
	}
	if !pathWithinDirectory(workspace, workspace) {
		t.Fatal("a directory must contain itself")
	}
}

func TestResolveModuleDoesNotPreferSiblingDirectoryPrefix(t *testing.T) {
	root := t.TempDir()
	currentDir := filepath.Join(root, "app")
	workspace := filepath.Join(root, "workspace")
	s := NewServer("test")
	s.WorkspaceFolders = []string{s.pathToURI(workspace)}
	current := s.pathToURI(filepath.Join(currentDir, "main.lua"))
	wrongPath := filepath.Join(root, "application", "nested", "mod.lua")
	correctPath := filepath.Join(workspace, "nested", "mod.lua")
	wrong := &Document{URI: s.pathToURI(wrongPath), Path: wrongPath}
	correct := &Document{URI: s.pathToURI(correctPath), Path: correctPath}
	s.Documents[current] = &Document{URI: current, Dir: currentDir}
	s.Documents[wrong.URI] = wrong
	s.Documents[correct.URI] = correct
	if got := s.resolveModule(current, "nested.mod"); got != correct {
		t.Fatalf("resolved %v, want %v", got, correct.URI)
	}
}
