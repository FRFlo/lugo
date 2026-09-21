package lsp

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestUpdateDocumentRejectsOversizedSource(t *testing.T) {
	s := NewServer("test")
	s.MaxFileSize = 3

	if s.updateDocument("file:///test.lua", []byte("a=1")) {
		t.Fatal("initial update unexpectedly requested workspace republish")
	}
	doc := s.Documents["file:///test.lua"]
	if doc == nil {
		t.Fatal("initial document was not stored")
	}
	tree := doc.Tree

	if s.updateDocument("file:///test.lua", []byte("a=12")) {
		t.Fatal("oversized update unexpectedly requested workspace republish")
	}
	if s.Documents["file:///test.lua"].Tree != tree {
		t.Fatal("oversized update replaced the existing document")
	}
}

func TestHandleDidOpenRejectsOversizedSource(t *testing.T) {
	s := NewServer("test")
	s.MaxFileSize = 3
	s.Writer = &bytes.Buffer{}
	params, _ := json.Marshal(DidOpenTextDocumentParams{TextDocument: TextDocumentItem{
		URI:  "file:///test.lua",
		Text: "a=12",
	}})

	s.handleDidOpen(Request{Params: params})

	if !s.OpenFiles["file:///test.lua"] {
		t.Fatal("oversized document was not marked open")
	}
	if _, ok := s.Documents["file:///test.lua"]; ok {
		t.Fatal("oversized document was parsed and stored")
	}
}

func TestHandleDidChangeRejectsOversizedSource(t *testing.T) {
	s := NewServer("test")
	s.MaxFileSize = 3
	s.Writer = &bytes.Buffer{}
	uri := "file:///test.lua"
	s.updateDocument(uri, []byte("a=1"))
	s.OpenFiles[uri] = true
	tree := s.Documents[uri].Tree

	params, _ := json.Marshal(DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: "a=12"}},
	})
	s.handleDidChange(Request{Params: params})

	if s.Documents[uri].Tree != tree {
		t.Fatal("oversized change replaced the existing document")
	}
}
