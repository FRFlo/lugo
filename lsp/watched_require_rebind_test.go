package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestWatchedModuleCreateAndDeleteRebindsRequire(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	mainURI := s.pathToURI(filepath.Join(h.root, "main.lua"))
	targetPath := filepath.Join(h.root, "mod.lua")
	targetURI := s.pathToURI(targetPath)
	s.updateDocument(mainURI, []byte("local m = require('mod')\nprint(m.field)"))
	doc := s.Documents[mainURI]
	fieldHash := func() uint64 {
		t.Helper()
		for _, field := range doc.Resolver.PendingFields {
			if field.PropHash == ast.HashBytes([]byte("field")) {
				return field.ReceiverHash
			}
		}
		t.Fatal("missing field reference")
		return 0
	}
	send := func(kind int) {
		t.Helper()
		params, err := json.Marshal(DidChangeWatchedFilesParams{Changes: []FileEvent{{URI: targetURI, Type: kind}}})
		if err != nil {
			t.Fatal(err)
		}
		s.handleDidChangeWatchedFiles(Request{Params: params})
	}
	original := fieldHash()
	if err := os.WriteFile(targetPath, []byte("return {field = 1}"), 0644); err != nil {
		t.Fatal(err)
	}
	send(1)
	want := ast.HashBytesConcat([]byte("module:"), nil, []byte(targetURI))
	if got := fieldHash(); got != want {
		t.Fatalf("after create = %d, want %d", got, want)
	}
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	send(3)
	if got := fieldHash(); got != original {
		t.Fatalf("after delete = %d, want original %d", got, original)
	}
}
