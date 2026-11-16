package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestClosedDocumentSameTextOpenCloseCycle(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagUnusedLocal = true
	const source = "local unused = 1\n"
	h.writeWorkspaceFile("cycle.lua", source)
	uri := h.server.pathToURI(filepath.Join(h.root, "cycle.lua"))

	for version := 1; version <= 3; version++ {
		openParams, err := json.Marshal(DidOpenTextDocumentParams{TextDocument: TextDocumentItem{
			URI: uri, Version: version, Text: source,
		}})
		if err != nil {
			t.Fatal(err)
		}
		h.server.handleDidOpen(Request{Params: openParams})

		closeParams, err := json.Marshal(DidCloseTextDocumentParams{TextDocument: TextDocumentIdentifier{URI: uri}})
		if err != nil {
			t.Fatal(err)
		}
		h.server.handleDidClose(Request{Params: closeParams})
	}
}

func TestClosedDocumentSemanticFactsSurviveEviction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	uri := h.server.pathToURI(filepath.Join(h.root, "mutated.lua"))
	h.server.updateDocument(uri, []byte("local x=1\nx=2\nprint(x)\n"))
	doc := h.server.Documents[uri]
	if doc == nil || doc.Tree == nil || doc.Resolver == nil {
		t.Fatal("document should have its tree and resolver")
	}
	reads := append([]uint16(nil), doc.ActualReads...)
	mutated := append([]bool(nil), doc.MutatedLocals...)
	var target ast.NodeID = ast.InvalidNode
	for id, def := range doc.Resolver.References {
		if def != ast.InvalidNode && ast.NodeID(id) != def &&
			string(doc.Tree.Source[doc.Tree.Nodes[id].Start:doc.Tree.Nodes[id].End]) == "x" {
			target = ast.NodeID(id)
			break
		}
	}
	if target == ast.InvalidNode {
		t.Fatal("could not find reference to reassigned local x")
	}
	if _, ok := doc.evalNode(target, 0); ok {
		t.Fatal("reassigned local evaluated as a constant before eviction")
	}

	evictClosedDocumentCaches(h.server)
	if _, ok := doc.evalNode(target, 0); ok {
		t.Error("reassigned local evaluated as a constant after eviction")
	}
	if !reflect.DeepEqual(doc.ActualReads, reads) || !reflect.DeepEqual(doc.MutatedLocals, mutated) {
		t.Error("semantic facts changed during eviction")
	}
}

func TestClosedDocumentReindexUnchanged(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagUnusedLocal = true
	h.writeWorkspaceFile("reindex.lua", "local unused = 1\n")
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "reindex.lua"))
	closeParams, err := json.Marshal(DidCloseTextDocumentParams{TextDocument: TextDocumentIdentifier{URI: uri}})
	if err != nil {
		t.Fatal(err)
	}
	h.server.handleDidClose(Request{Params: closeParams})
	h.reindex()
}

func TestClosedDocumentWireOpenCloseCycle(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	path := filepath.Join(h.root, "wire.lua")
	h.writeWorkspaceFile("wire.lua", "local unused = 1\n")
	uri := h.server.pathToURI(path)
	input := framed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	input += framed(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	for version := 1; version <= 5; version++ {
		open, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{
			"textDocument": map[string]any{"uri": uri, "version": version, "languageId": "lua", "text": "local unused = 1\n"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		input += framed(string(open))
		close, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didClose", "params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
		}})
		if err != nil {
			t.Fatal(err)
		}
		input += framed(string(close))
	}
	input += framed(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`)
	input += framed(`{"jsonrpc":"2.0","method":"exit"}`)

	cmd := exec.Command(os.Args[0], "-test.run=^TestLSPProcessLifecycleHelper$")
	cmd.Env = append(os.Environ(), "LUGO_LIFECYCLE_HELPER=1")
	cmd.Stdin = strings.NewReader(input)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("wire lifecycle exit code %d; output: %s", exitErr.ExitCode(), stdout.String())
		}
		t.Fatalf("wire lifecycle: %v", err)
	}
}
