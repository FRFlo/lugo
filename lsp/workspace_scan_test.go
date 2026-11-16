package lsp

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/coalaura/plain"
)

func TestIndexWorkspaceReportsIncompleteScanErrors(t *testing.T) {
	root := t.TempDir()
	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.workspaceReadDir = func(string) ([]os.DirEntry, error) {
		return nil, os.ErrPermission
	}
	var jobs []*IndexJob
	var unchanged, failed int

	if complete := s.indexWorkspace(root, &jobs, &unchanged, &failed); complete {
		t.Fatal("indexWorkspace reported a complete scan after ReadDir failed")
	}
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
}

func TestIndexWorkspaceRootStatErrorsAreNotTreatedAsDeletion(t *testing.T) {
	root := t.TempDir()
	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	wantErr := errors.New("stat denied")
	s.workspaceStat = func(string) (os.FileInfo, error) {
		return nil, wantErr
	}
	var jobs []*IndexJob
	var unchanged, failed int

	if complete := s.indexWorkspace(root, &jobs, &unchanged, &failed); complete {
		t.Fatal("indexWorkspace reported a complete scan after Stat failed")
	}
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
}

func TestIndexWorkspaceMissingRootIsCompleteRemoval(t *testing.T) {
	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	var jobs []*IndexJob
	var unchanged, failed int

	if complete := s.indexWorkspace(filepath.Join(t.TempDir(), "deleted"), &jobs, &unchanged, &failed); !complete {
		t.Fatal("missing workspace root should be treated as a completed removal")
	}
	if failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}
}

func TestRefreshWorkspaceKeepsDocumentsAfterIncompleteScan(t *testing.T) {
	root := t.TempDir()
	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.IsCI = true
	s.WorkspaceFolders = []string{root}
	s.workspaceReadDir = func(string) ([]os.DirEntry, error) {
		return nil, os.ErrPermission
	}
	doc := &Document{URI: "file:///workspace/still-here.lua"}
	s.Documents[doc.URI] = doc

	s.refreshWorkspace()

	if s.Documents[doc.URI] != doc {
		t.Fatal("document was removed after an incomplete workspace scan")
	}
}

func TestRefreshWorkspaceRemovesDocumentsAfterRootDeletion(t *testing.T) {
	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.IsCI = true
	s.WorkspaceFolders = []string{filepath.Join(t.TempDir(), "deleted")}
	doc := &Document{URI: "file:///workspace/removed.lua"}
	s.Documents[doc.URI] = doc

	s.refreshWorkspace()

	if _, exists := s.Documents[doc.URI]; exists {
		t.Fatal("document remained after its workspace root was actually removed")
	}
}

func TestIndexWorkspaceFollowsSymlinkDirectories(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	file := filepath.Join(target, "linked.lua")
	if err := os.WriteFile(file, []byte("return true"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	var jobs []*IndexJob
	var unchanged, failed int
	if complete := s.indexWorkspace(root, &jobs, &unchanged, &failed); !complete {
		t.Fatal("symlink workspace scan was incomplete")
	}
	if len(jobs) != 1 || jobs[0].Path != file {
		t.Fatalf("jobs = %#v, want linked file %q", jobs, file)
	}
}

func TestRefreshWorkspaceDoesNotReregisterRemovedManifest(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "return true\n")
	h.reindex()

	rootURI := h.server.pathToURI(filepath.Join(h.root, "resource"))
	if h.server.FiveMResourceGraph.ResourceByRoot(rootURI) == nil {
		t.Fatal("expected resource to be registered before manifest deletion")
	}
	if err := os.Remove(filepath.Join(h.root, "resource", "fxmanifest.lua")); err != nil {
		t.Fatal(err)
	}
	h.reindex()

	if h.server.FiveMResourceGraph.ResourceByRoot(rootURI) != nil {
		t.Fatal("removed manifest left a phantom resource in the graph")
	}
}

func TestRefreshWorkspaceKeepsOpenManifestWhenDiskFileIsRemoved(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "return true\n")
	h.reindex()

	manifestURI := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
	rootURI := h.server.pathToURI(filepath.Join(h.root, "resource"))
	h.server.OpenFiles[manifestURI] = true
	if err := os.Remove(filepath.Join(h.root, "resource", "fxmanifest.lua")); err != nil {
		t.Fatal(err)
	}
	h.reindex()

	if h.server.FiveMResourceGraph.ResourceByRoot(rootURI) == nil || h.server.Documents[manifestURI] == nil {
		t.Fatal("open manifest buffer was not retained after its disk file was removed")
	}
}

func TestRefreshWorkspaceKeepsManifestOnIncompleteScan(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "return true\n")
	h.reindex()

	manifestURI := h.server.pathToURI(filepath.Join(h.root, "resource", "fxmanifest.lua"))
	rootURI := h.server.pathToURI(filepath.Join(h.root, "resource"))
	h.server.workspaceReadDir = func(string) ([]os.DirEntry, error) {
		return nil, os.ErrPermission
	}
	h.reindex()

	if h.server.FiveMResourceGraph.ResourceByRoot(rootURI) == nil || h.server.Documents[manifestURI] == nil {
		t.Fatal("manifest was not retained after an incomplete workspace scan")
	}
}

func TestRefreshWorkspacePreservesManifestTopologyOnIncompleteScan(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'base'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "return true\n")
	h.writeWorkspaceFile("base/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n")
	h.reindex()

	resourceURI := h.server.pathToURI(filepath.Join(h.root, "resource"))
	baseURI := h.server.pathToURI(filepath.Join(h.root, "base"))
	if got := h.server.FiveMResourceGraph.ResourceByRoot(resourceURI); got == nil {
		t.Fatal("expected resource before incomplete scan")
	}
	if got := h.server.FiveMResourceGraph.ResourceByRoot(baseURI); got == nil {
		t.Fatal("expected dependency resource before incomplete scan")
	}

	h.server.workspaceReadDir = func(string) ([]os.DirEntry, error) { return nil, os.ErrPermission }
	h.reindex()

	resource := h.server.FiveMResourceGraph.ResourceByRoot(resourceURI)
	if resource == nil {
		t.Fatal("resource topology was lost after incomplete scan")
	}
	if got := h.server.FiveMResourceGraph.ResourceByRoot(baseURI); got == nil {
		t.Fatal("dependency topology was lost after incomplete scan")
	}
	if len(resource.Dependencies) != 1 || resource.Dependencies[0] != "base" {
		t.Fatalf("dependencies = %v, want [base]", resource.Dependencies)
	}
}
