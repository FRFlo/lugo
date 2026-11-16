package lsp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
)

func BenchmarkInferMetatableAttachmentAt(b *testing.B) {
	var src []byte
	for i := 0; i < 250; i++ {
		src = append(src, []byte(fmt.Sprintf("local noise%d = { value = %d }\n", i, i))...)
	}
	src = append(src, []byte("local object = {}\nsetmetatable(object, { __index = {} })\nlocal value = object\n")...)
	doc := benchmarkMetatableDocument(b, src)
	defID := benchmarkFindIdent(doc.Tree, "object", 0)
	if defID == ast.InvalidNode {
		b.Fatal("object definition not found")
	}
	offset := uint32(len(src))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = doc.inferMetatableAttachmentAt(defID, TypeSet{}, offset)
	}
}

func BenchmarkNewIndexWrite(b *testing.B) {
	const docs = 80
	server := NewServer("benchmark")
	var receiverDoc *Document
	for i := 0; i < docs; i++ {
		src := []byte(fmt.Sprintf("local sink%d = {}\nlocal object%d = setmetatable({}, { __newindex = sink%d })\nobject%d.answer = %d\n", i, i, i, i, i))
		doc := benchmarkMetatableDocument(b, src)
		doc.Server = server
		doc.URI = fmt.Sprintf("file:///bench/%d.lua", i)
		server.Documents[doc.URI] = doc
		if i == docs-1 {
			receiverDoc = doc
		}
	}
	receiverType := TypeSet{DeclNode: benchmarkFindIdent(receiverDoc.Tree, "sink79", 0), DeclURI: receiverDoc.URI}
	if receiverType.DeclNode == ast.InvalidNode {
		b.Fatal("receiver definition not found")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = receiverDoc.newIndexWrite(receiverType, "answer", ^uint32(0))
	}
}

func TestMetatableAttachmentIndexInvalidatesAfterReindex(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local first = { run = function() return "first" end }
local second = { run = function() return "second" end }
local object = {}
setmetatable(object, { __index = first })
object:--[[@attached]]run()
`)
	h.reindex()
	first := h.resolve("attached")
	if first == nil || first.TargetDefID == ast.InvalidNode || string(first.TargetDoc.nodeSource(first.TargetDefID)) != "run" {
		t.Fatalf("initial attachment did not resolve: %#v", first)
	}
	firstValue := first.TargetDoc.getAssignedValue(first.TargetDefID)
	firstSource := string(first.TargetDoc.nodeSource(firstValue))

	updatedSource := `local first = { run = function() return "first" end }
local second = { run = function() return "second" end }
local object = {}
setmetatable(object, { __index = second })
object:run()
`
	h.writeWorkspaceFile("main.lua", updatedSource[:len(updatedSource)-len("object:run()\n")]+"object:--[[@attached]]run()\n")
	h.server.updateDocument(first.TargetURI, []byte(updatedSource))
	h.reindex()
	second := h.resolve("attached")
	if second == nil || second.TargetDefID == ast.InvalidNode {
		t.Fatalf("reindexed attachment did not resolve: %#v", second)
	}
	secondSource := string(second.TargetDoc.nodeSource(second.TargetDoc.getAssignedValue(second.TargetDefID)))
	if !strings.Contains(firstSource, "first") || !strings.Contains(secondSource, "second") {
		t.Fatalf("reindexed attachment target value = %q, initial = %q; want second instead of first", secondSource, firstSource)
	}
}

func TestNewIndexWriteIndexInvalidatesAfterReindex(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local sink = {}
local object = setmetatable({}, { __newindex = sink })
object.answer = 42
local result = sink.--[[@answer]]answer
`)
	h.reindex()
	doc := h.docForMarker("answer")
	sink := findIdentByOccurrence(t, doc.Tree, "sink", 0)
	receiver := doc.InferType(sink)
	if writeDoc, _, valueID := doc.newIndexWrite(receiver, "answer", ^uint32(0)); writeDoc == nil || valueID == ast.InvalidNode {
		t.Fatalf("initial redirected write was missing: document=%p value=%d", writeDoc, valueID)
	}

	updatedSource := `local sink = {}
local object = setmetatable({}, { __newindex = sink })
local result = sink.answer
`
	h.writeWorkspaceFile("main.lua", strings.Replace(updatedSource, "sink.answer", "sink.--[[@answer]]answer", 1))
	h.server.updateDocument(doc.URI, []byte(updatedSource))
	h.reindex()
	doc = h.docForMarker("answer")
	sink = findIdentByOccurrence(t, doc.Tree, "sink", 0)
	receiver = doc.InferType(sink)
	if writeDoc, nodeID, valueID := doc.newIndexWrite(receiver, "answer", ^uint32(0)); writeDoc != nil || nodeID != ast.InvalidNode || valueID != ast.InvalidNode {
		t.Fatalf("removed redirected write remained indexed: document=%p node=%d value=%d", writeDoc, nodeID, valueID)
	}
}

func benchmarkMetatableDocument(tb testing.TB, source []byte) *Document {
	tb.Helper()
	tree := ast.NewTree(source)
	p := parser.New(source, tree, 0)
	p.Parse()
	if len(p.Errors) != 0 {
		tb.Fatalf("parse errors = %#v", p.Errors)
	}
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	return &Document{Tree: tree, Resolver: resolver}
}

func benchmarkFindIdent(tree *ast.Tree, name string, occurrence int) ast.NodeID {
	for id, seen := 1, 0; id < len(tree.Nodes); id++ {
		node := tree.Nodes[id]
		if node.Kind == ast.KindIdent && string(tree.Source[node.Start:node.End]) == name {
			if seen == occurrence {
				return ast.NodeID(id)
			}
			seen++
		}
	}
	return ast.InvalidNode
}
