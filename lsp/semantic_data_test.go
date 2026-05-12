package lsp

import (
	"reflect"
	"testing"

	"github.com/coalaura/lugo/ast"
)

func TestSemanticData(t *testing.T) {
	t.Run("Get_nonexistent", func(t *testing.T) {
		table := NewSemanticDataTable()

		allocs := testing.AllocsPerRun(1000, func() {
			if got := table.Get(NodeID(42)); got != nil {
				t.Fatalf("Get() = %#v, want nil", got)
			}
		})
		if allocs != 0 {
			t.Fatalf("Get() allocated %.2f times, want 0", allocs)
		}
	})

	t.Run("SetGet_extensions", func(t *testing.T) {
		table := NewSemanticDataTable()
		nodeID := NodeID(7)
		scope := &Scope{Symbols: map[string]NodeID{"player": nodeID}}

		table.Set(nodeID, &SemanticData{
			Type:     Type{Primitive: TypeTable},
			Scope:    scope,
			Bindings: []Binding{{Name: "player", NodeID: nodeID, Type: Type{Primitive: TypeString}}},
			LuaDoc:   &LuaDocData{},
			FiveM:    &FiveMData{},
			Export:   &ExportData{},
		})

		got := table.Get(nodeID)
		if got == nil {
			t.Fatal("Get() = nil, want semantic data")
		}
		if got.Type.Primitive != TypeTable {
			t.Fatalf("Type.Primitive = %v, want %v", got.Type.Primitive, TypeTable)
		}
		if got.Scope != scope {
			t.Fatalf("Scope = %p, want %p", got.Scope, scope)
		}
		if len(got.Bindings) != 1 || got.Bindings[0].Name != "player" || got.Bindings[0].NodeID != nodeID || got.Bindings[0].Type.Primitive != TypeString {
			t.Fatalf("Bindings = %#v, want player binding", got.Bindings)
		}
		if got.LuaDoc == nil || got.FiveM == nil || got.Export == nil {
			t.Fatalf("extensions = LuaDoc:%v FiveM:%v Export:%v, want all present", got.LuaDoc, got.FiveM, got.Export)
		}
	})

	t.Run("Clear_reuse", func(t *testing.T) {
		table := NewSemanticDataTable()
		table.Set(NodeID(1), &SemanticData{Type: Type{Primitive: TypeString}})

		mapPtr := reflect.ValueOf(table.data).Pointer()
		table.Clear()

		if got := table.Get(NodeID(1)); got != nil {
			t.Fatalf("Get() after Clear = %#v, want nil", got)
		}
		if table.arenaPtr != 0 {
			t.Fatalf("arenaPtr after Clear = %d, want 0", table.arenaPtr)
		}
		if reflect.ValueOf(table.data).Pointer() != mapPtr {
			t.Fatal("Clear() replaced map, want map reuse")
		}

		table.Set(NodeID(2), &SemanticData{Type: Type{Primitive: TypeTable}})
		if got := table.Get(NodeID(2)); got == nil || got.Type.Primitive != TypeTable {
			t.Fatalf("Get() after reuse = %#v, want TypeTable", got)
		}
	})

	t.Run("Arena_rollover", func(t *testing.T) {
		table := NewSemanticDataTable()

		for i := 0; i < semanticDataArenaSize+1; i++ {
			table.Set(NodeID(i+1), &SemanticData{Type: Type{Primitive: TypeString}})
		}

		if len(table.arenaChunks) != 2 {
			t.Fatalf("arena chunk count = %d, want 2", len(table.arenaChunks))
		}
		if got := table.Get(NodeID(1)); got == nil || got.Type.Primitive != TypeString {
			t.Fatalf("first arena value = %#v, want TypeString", got)
		}
		if got := table.Get(NodeID(semanticDataArenaSize + 1)); got == nil || got.Type.Primitive != TypeString {
			t.Fatalf("rollover arena value = %#v, want TypeString", got)
		}
	})

	t.Run("NodeID_aliases_ast_NodeID", func(t *testing.T) {
		var id NodeID = NodeID(ast.NodeID(3))
		if ast.NodeID(id) != ast.NodeID(3) {
			t.Fatalf("NodeID alias = %d, want 3", id)
		}
	})
}

func BenchmarkSemanticDataTable(b *testing.B) {
	b.Run("Get", func(b *testing.B) {
		table := NewSemanticDataTable()
		table.Set(NodeID(1), &SemanticData{Type: Type{Primitive: TypeString}})

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			if table.Get(NodeID(1)) == nil {
				b.Fatal("missing semantic data")
			}
		}
	})
}
