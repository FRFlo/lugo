package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchedModuleChangesRepublishConsumers(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.IsIndexing = false
	mainURI := s.pathToURI(filepath.Join(h.root, "main.lua"))
	targetPath := filepath.Join(h.root, "mod.lua")
	targetURI := s.pathToURI(targetPath)
	s.updateDocument(mainURI, []byte("local m = require('mod')\nprint(m.field)"))
	send := func(uri string, kind int) string {
		t.Helper()
		params, err := json.Marshal(DidChangeWatchedFilesParams{Changes: []FileEvent{{URI: uri, Type: kind}}})
		if err != nil {
			t.Fatal(err)
		}
		h.rpcOut.Reset()
		s.handleDidChangeWatchedFiles(Request{Params: params})
		return h.rpcOut.String()
	}
	if err := os.WriteFile(targetPath, []byte("return {field = 1}"), 0644); err != nil {
		t.Fatal(err)
	}
	if out := send(targetURI, 1); !strings.Contains(out, mainURI) {
		t.Fatal("creation did not republish the consumer")
	}
	if out := send(targetURI, 2); strings.Contains(out, mainURI) {
		t.Fatal("unchanged module unnecessarily republished the consumer")
	}
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	if out := send(targetURI, 3); !strings.Contains(out, mainURI) {
		t.Fatal("deletion did not republish the consumer")
	}
}
