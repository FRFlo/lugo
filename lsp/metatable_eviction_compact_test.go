package lsp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestEvictedMetatableNodeTextIsCompactAndRebuildable(t *testing.T) {
	const writes = 5000
	var source strings.Builder
	source.Grow(writes * 32)
	source.WriteString("local sink = {}\nlocal object = setmetatable({}, { __newindex = sink })\n")
	for i := 0; i < writes; i++ {
		fmt.Fprintf(&source, "object.field%d = %d\n", i, i)
	}
	source.WriteString("object[\"bracket-key\"] = 1\nlocal result = sink.field1\n")
	doc := benchmarkMetatableDocument(t, []byte(source.String()))
	doc.URI = "file:///compact-eviction.lua"
	s := NewServer("compact-eviction")
	doc.Server = s
	s.Documents[doc.URI] = doc

	evictClosedDocumentCaches(s)
	if doc.Source() != nil {
		t.Fatal("source must be released after eviction")
	}
	if got := len(doc.evictedNodeText); got < writes || got > writes+10 {
		t.Fatalf("cached node text entries = %d, want about %d", got, writes)
	}
	if got := len(doc.evictedNodeTextData); got > 48<<10 {
		t.Fatalf("compact text payload = %d bytes, want at most %d", got, 48<<10)
	}
	if got := len(doc.evictedNodeTextData) + len(doc.evictedNodeText)*12; got > 112<<10 {
		t.Fatalf("compact cache payload and index = %d bytes, want at most %d", got, 112<<10)
	}

	for i := 1; i < len(doc.evictedNodeText); i++ {
		if doc.evictedNodeText[i-1].node >= doc.evictedNodeText[i].node {
			t.Fatal("evicted node text entries must be strictly sorted by NodeID")
		}
	}
	for _, want := range []string{"field0", "field4999", `"bracket-key"`, "setmetatable"} {
		found := false
		for _, entry := range doc.evictedNodeText {
			if string(doc.nodeSource(entry.node)) == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("cached node text does not contain %q", want)
		}
	}

	s.invalidateMetatableIndexes()
	candidates := s.newIndexWriteCandidates("field1")
	if len(candidates) == 0 || string(candidates[0].doc.nodeSource(candidates[0].node)) != "field1" {
		t.Fatal("metatable write index did not rebuild after source eviction")
	}
	if candidates[0].node == ast.InvalidNode {
		t.Fatal("rebuilt candidate has invalid node id")
	}
	stringCandidates := s.newIndexWriteCandidates("bracket-key")
	if len(stringCandidates) != 1 {
		t.Fatalf("string-key candidate index did not rebuild after source eviction: %#v", stringCandidates)
	}
}
