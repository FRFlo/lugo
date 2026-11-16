package lsp

import (
	"fmt"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestMetatableFieldValueAtUsesLatestWriteBeforeOffsetAndBinding(t *testing.T) {
	doc := benchmarkMetatableDocument(t, []byte(`local mt = { __newindex = "initial" }
mt.__newindex = "first"
do
  local mt = { __newindex = "shadow" }
  mt.__newindex = "shadow write"
end
mt["__newindex"] = "last"
`))
	var offsets []uint32
	for i, b := range doc.Tree.Source {
		if b == '\n' {
			offsets = append(offsets, uint32(i+1))
		}
	}
	tableID := benchmarkFindKind(doc.Tree, ast.KindTableExpr, 0)
	if tableID == ast.InvalidNode {
		t.Fatal("metatable table not found")
	}
	value := doc.metatableFieldValueAt(tableID, "__newindex", offsets[0])
	if got := string(doc.nodeSource(value)); got != `"initial"` {
		t.Fatalf("value before first assignment = %q, want initial", got)
	}
	value = doc.metatableFieldValueAt(tableID, "__newindex", uint32(len(doc.Tree.Source)))
	if got := string(doc.nodeSource(value)); got != `"last"` {
		t.Fatalf("latest value = %q, want last", got)
	}
}

func BenchmarkMetatableFieldValueAtLarge(b *testing.B) {
	var source []byte
	for i := 0; i < 4000; i++ {
		source = append(source, []byte(fmt.Sprintf("local noise%d = { value = %d }\n", i, i))...)
	}
	source = append(source, []byte("local mt = { __newindex = {} }\nmt.__newindex = {}\n")...)
	doc := benchmarkMetatableDocument(b, source)
	tableID := benchmarkFindKind(doc.Tree, ast.KindTableExpr, 0)
	offset := uint32(len(source))
	_ = doc.metatableFieldValueAt(tableID, "__newindex", offset)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = doc.metatableFieldValueAt(tableID, "__newindex", offset)
	}
}

// BenchmarkMetatableFieldValueAtScanBaseline keeps the original whole-tree
// lookup in test code so indexed and scan costs can be compared on one run.
func BenchmarkMetatableFieldValueAtScanBaseline(b *testing.B) {
	var source []byte
	for i := 0; i < 4000; i++ {
		source = append(source, []byte(fmt.Sprintf("local noise%d = { value = %d }\n", i, i))...)
	}
	source = append(source, []byte("local mt = { __newindex = {} }\nmt.__newindex = {}\n")...)
	doc := benchmarkMetatableDocument(b, source)
	tableID := benchmarkFindIdent(doc.Tree, "mt", 0)
	offset := uint32(len(source))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = metatableFieldValueAtScanBaseline(doc, doc.getDefForValue(tableID), "__newindex", offset)
	}
}

func metatableFieldValueAtScanBaseline(doc *Document, definition ast.NodeID, name string, offset uint32) ast.NodeID {
	value, latestStart := ast.InvalidNode, uint32(0)
	for i, member := range doc.Tree.Nodes {
		if (member.Kind != ast.KindMemberExpr && member.Kind != ast.KindIndexExpr) || member.Left == ast.InvalidNode || member.Right == ast.InvalidNode || int(member.Left) >= len(doc.Tree.Nodes) || int(member.Right) >= len(doc.Tree.Nodes) || doc.referenceAt(member.Left) != definition {
			continue
		}
		key := doc.Tree.Nodes[member.Right]
		if member.Kind == ast.KindMemberExpr {
			if key.Kind != ast.KindIdent || string(doc.nodeSource(member.Right)) != name {
				continue
			}
		} else {
			if key.Kind != ast.KindString {
				continue
			}
			decoded, ok := doc.evalNode(member.Right, 0)
			if !ok || decoded.kind != ast.KindString || decoded.str != name {
				continue
			}
		}
		if member.Parent == ast.InvalidNode || int(member.Parent) >= len(doc.Tree.Nodes) {
			continue
		}
		list := doc.Tree.Nodes[member.Parent]
		if list.Kind != ast.KindExprList || list.Parent == ast.InvalidNode || int(list.Parent) >= len(doc.Tree.Nodes) {
			continue
		}
		assign := doc.Tree.Nodes[list.Parent]
		if assign.Kind != ast.KindAssign || assign.Left != member.Parent || assign.Start >= offset {
			continue
		}
		assigned := doc.getAssignedValue(ast.NodeID(i))
		if assigned != ast.InvalidNode && assign.Start >= latestStart {
			value, latestStart = assigned, assign.Start
		}
	}
	return value
}

func benchmarkFindKind(tree *ast.Tree, kind ast.NodeKind, occurrence int) ast.NodeID {
	for id, seen := 1, 0; id < len(tree.Nodes); id++ {
		if tree.Nodes[id].Kind == kind {
			if seen == occurrence {
				return ast.NodeID(id)
			}
			seen++
		}
	}
	return ast.InvalidNode
}

func BenchmarkBuildNewIndexWriteIndexLarge(b *testing.B) {
	var source []byte
	for i := 0; i < 4000; i++ {
		source = append(source, []byte(fmt.Sprintf("local noise%d = { value = %d }\n", i, i))...)
	}
	source = append(source, []byte("local sink = {}\nlocal object = setmetatable({}, { __newindex = sink })\nobject.answer = 42\n")...)
	doc := benchmarkMetatableDocument(b, source)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = buildNewIndexWriteIndexFromSlice([]*Document{doc})
	}
}
