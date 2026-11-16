package lsp

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWorkspaceFolderChangesOnlyRefreshWhenFoldersChange(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.workDoneProgressSupport = true
	root := WorkspaceFolder{URI: s.RootURI}
	missing := WorkspaceFolder{URI: s.pathToURI(t.TempDir())}
	send := func(event WorkspaceFoldersChangeEvent) {
		t.Helper()
		data, err := json.Marshal(DidChangeWorkspaceFoldersParams{Event: event})
		if err != nil {
			t.Fatal(err)
		}
		s.handleDidChangeWorkspaceFolders(Request{Params: data})
	}
	for _, event := range []WorkspaceFoldersChangeEvent{
		{},
		{Added: []WorkspaceFolder{root}},
		{Removed: []WorkspaceFolder{missing}},
		{Removed: []WorkspaceFolder{root}, Added: []WorkspaceFolder{root}},
	} {
		send(event)
		if h.rpcOut.Len() != 0 {
			t.Fatal("unchanged folders caused a workspace refresh")
		}
	}
	send(WorkspaceFoldersChangeEvent{Added: []WorkspaceFolder{missing}})
	if h.rpcOut.Len() == 0 {
		t.Fatal("adding a folder did not refresh the workspace")
	}
	if len(s.WorkspaceFolders) != 2 {
		t.Fatalf("folders = %v", s.WorkspaceFolders)
	}
}

func TestWorkspaceFoldersAndLibrariesRefreshTogether(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.workDoneProgressSupport = true
	folder := WorkspaceFolder{URI: s.pathToURI(t.TempDir())}
	libraries := []string{t.TempDir()}
	params, err := json.Marshal(DidChangeWorkspaceFoldersParams{
		Event: WorkspaceFoldersChangeEvent{Added: []WorkspaceFolder{folder}}, LibraryPaths: &libraries,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.handleDidChangeWorkspaceFolders(Request{Params: params})
	if count := bytes.Count(h.rpcOut.Bytes(), []byte("window/workDoneProgress/create")); count != 1 {
		t.Fatalf("workspace refresh count = %d, want 1", count)
	}
	if len(s.LibraryPaths) != 1 || s.LibraryPaths[0] != libraries[0] {
		t.Fatalf("library paths = %v, want %v", s.LibraryPaths, libraries)
	}
	h.rpcOut.Reset()
	s.handleDidChangeWorkspaceFolders(Request{Params: params})
	if h.rpcOut.Len() != 0 {
		t.Fatal("unchanged folders and libraries refreshed workspace")
	}
}
