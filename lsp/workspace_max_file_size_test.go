package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/coalaura/plain"
)

func TestHandleDidOpenRejectsOversizedDocument(t *testing.T) {
	var output bytes.Buffer
	s := NewServer("test")
	s.Writer = &output
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.MaxFileSize = 3
	uri := "file:///too-large.lua"

	params, err := json.Marshal(DidOpenTextDocumentParams{TextDocument: TextDocumentItem{
		URI:  uri,
		Text: "over",
	}})
	if err != nil {
		t.Fatal(err)
	}
	s.handleDidOpen(Request{Params: params})

	if _, ok := s.Documents[uri]; ok {
		t.Fatal("oversized document was retained")
	}
	if !s.OpenFiles[uri] {
		t.Fatal("oversized document was not marked open")
	}
	if !strings.Contains(output.String(), `"code":"file-too-large"`) {
		t.Fatalf("missing file-too-large diagnostic: %s", output.String())
	}
}

func TestHandleDidChangeRejectsOversizedDocumentWithoutReplacingSource(t *testing.T) {
	var output bytes.Buffer
	s := NewServer("test")
	s.Writer = &output
	s.Log = plain.New(plain.WithTarget(io.Discard))
	s.MaxFileSize = 3
	uri := "file:///changed.lua"
	s.updateDocument(uri, []byte("ok"))
	output.Reset()

	params, err := json.Marshal(DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: "over"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.handleDidChange(Request{Params: params})

	if got := string(s.Documents[uri].Source()); got != "ok" {
		t.Fatalf("source after rejected change = %q, want %q", got, "ok")
	}
	if !strings.Contains(output.String(), `"code":"file-too-large"`) {
		t.Fatalf("missing file-too-large diagnostic: %s", output.String())
	}
}
