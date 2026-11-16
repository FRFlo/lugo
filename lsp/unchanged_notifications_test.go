package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUnchangedWatchedFileDoesNotRepublish(t *testing.T) {
	for _, name := range []string{"file.lua", "fxmanifest.lua"} {
		t.Run(name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			s := h.server
			s.IsIndexing = false
			filename := filepath.Join(h.root, name)
			source := []byte("local item = 1\n")
			if err := os.WriteFile(filename, source, 0644); err != nil {
				t.Fatal(err)
			}
			uri := s.pathToURI(filename)
			s.updateDocument(uri, source)
			send := func() {
				t.Helper()
				p, err := json.Marshal(DidChangeWatchedFilesParams{Changes: []FileEvent{{URI: uri, Type: 2}}})
				if err != nil {
					t.Fatal(err)
				}
				h.rpcOut.Reset()
				s.handleDidChangeWatchedFiles(Request{Params: p})
			}
			send()
			if h.rpcOut.Len() != 0 {
				t.Fatal("unchanged file republished diagnostics")
			}
			stat, err := os.Stat(filename)
			if err != nil {
				t.Fatal(err)
			}
			if !s.Documents[uri].ModTime.Equal(stat.ModTime()) {
				t.Fatal("unchanged file modtime not updated")
			}
			if err := os.WriteFile(filename, []byte("local item = 2\n"), 0644); err != nil {
				t.Fatal(err)
			}
			send()
			if h.rpcOut.Len() == 0 {
				t.Fatal("changed file failed to publish diagnostics")
			}
		})
	}
}

func TestUnchangedOpenChangeAndOversizeRecovery(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.IsIndexing = false
	uri := s.pathToURI(filepath.Join(h.root, "file.lua"))
	text := "local item = 1\n"
	s.OpenFiles[uri] = true
	s.updateDocument(uri, []byte(text))
	send := func(version int, value string) {
		t.Helper()
		p, err := json.Marshal(map[string]any{"textDocument": map[string]any{"uri": uri, "version": version}, "contentChanges": []any{map[string]any{"text": value}}})
		if err != nil {
			t.Fatal(err)
		}
		h.rpcOut.Reset()
		s.handleDidChange(Request{Params: p})
	}
	send(1, text)
	if h.rpcOut.Len() != 0 {
		t.Fatal("unchanged edit republished diagnostics")
	}
	if s.documentVersions[uri] != 1 {
		t.Fatal("unchanged edit did not advance version")
	}
	s.MaxFileSize = int64(len(text))
	send(2, text+"x")
	if h.rpcOut.Len() == 0 || !s.oversizedOpenFiles[uri] {
		t.Fatal("oversized edit did not publish size diagnostic")
	}
	send(3, text)
	if h.rpcOut.Len() == 0 || s.oversizedOpenFiles[uri] {
		t.Fatal("restored size must replace size diagnostic")
	}
	send(4, "local item = 2\n")
	if h.rpcOut.Len() == 0 {
		t.Fatal("changed edit failed to publish diagnostics")
	}
}

func TestEvictedEmptyWatchedFileIsRestored(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.IsIndexing = false
	filename := filepath.Join(h.root, "empty.lua")
	if err := os.WriteFile(filename, nil, 0644); err != nil {
		t.Fatal(err)
	}
	uri := s.pathToURI(filename)
	s.updateDocument(uri, []byte("local item = 1"))
	s.Documents[uri].Tree.Source = nil
	p, err := json.Marshal(DidChangeWatchedFilesParams{Changes: []FileEvent{{URI: uri, Type: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	s.handleDidChangeWatchedFiles(Request{Params: p})
	if s.Documents[uri].Tree.Source == nil || len(s.Documents[uri].Tree.Source) != 0 {
		t.Fatal("evicted source was not restored as an empty file")
	}
}
