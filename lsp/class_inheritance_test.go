package lsp

import "testing"

func TestLuaDocClassInheritanceAnnotations(t *testing.T) {
	doc := parseLuaDoc([]byte(`
@class Child : Parent, Other Child class
@field own string Own field
@field shared number Override
`), false)

	if doc.Class == nil || doc.Class.Name != "Child" || doc.Class.Parent != "Parent" {
		t.Fatalf("class annotation = %+v, want Child extending Parent", doc.Class)
	}
	if got := doc.Class.Parents; len(got) != 2 || got[0] != "Parent" || got[1] != "Other" {
		t.Fatalf("class parents = %#v, want Parent then Other", got)
	}
	if len(doc.Fields) != 2 || doc.Fields[0].Name != "own" || doc.Fields[0].Type != "string" || doc.Fields[1].Name != "shared" || doc.Fields[1].Type != "number" {
		t.Fatalf("class fields = %+v, want own:string and shared:number", doc.Fields)
	}
}

func TestGlobalIndexLuaDocClassFieldMultipleParents(t *testing.T) {
	idx := NewGlobalIndex()
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "A", &SymbolEntry{})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "A.first", &SymbolEntry{Type: Type{Primitive: TypeString}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "B", &SymbolEntry{})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "B.second", &SymbolEntry{Type: Type{Primitive: TypeNumber}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "Child", &SymbolEntry{Parents: []string{"A", "B"}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "Child.shared", &SymbolEntry{Type: Type{Primitive: TypeBoolean}})

	for _, tc := range []struct {
		field string
		want  BasicType
	}{{"first", TypeString}, {"second", TypeNumber}, {"shared", TypeBoolean}} {
		got := idx.LookupLuaDocClassField("file:///app", GlobalIndexScopeShared, "Child", tc.field)
		if got == nil || got.Type.Primitive != tc.want {
			t.Errorf("field %q = %+v, want primitive %v", tc.field, got, tc.want)
		}
	}
}

func TestGlobalIndexLuaDocClassFieldMultipleParentsDeterministicAndCycleSafe(t *testing.T) {
	idx := NewGlobalIndex()
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "A", &SymbolEntry{Parents: []string{"Child"}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "A.value", &SymbolEntry{Type: Type{Primitive: TypeString}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "B", &SymbolEntry{})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "B.value", &SymbolEntry{Type: Type{Primitive: TypeNumber}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "Child", &SymbolEntry{Parents: []string{"A", "B"}})

	for range 2 {
		got := idx.LookupLuaDocClassField("file:///app", GlobalIndexScopeShared, "Child", "value")
		if got == nil || got.Type.Primitive != TypeString {
			t.Fatalf("field value = %+v, want first parent A's string", got)
		}
	}
}

func TestGlobalIndexLuaDocClassFieldInheritance(t *testing.T) {
	idx := NewGlobalIndex()
	idx.AddSymbol("file:///lib", GlobalIndexScopeShared, "Root", &SymbolEntry{})
	idx.AddSymbol("file:///lib", GlobalIndexScopeShared, "Root.id", &SymbolEntry{Type: Type{Primitive: TypeNumber}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "Middle", &SymbolEntry{Parent: "Root"})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "Middle.label", &SymbolEntry{Type: Type{Primitive: TypeString}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "Leaf", &SymbolEntry{Parent: "Middle"})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "Leaf.id", &SymbolEntry{Type: Type{Primitive: TypeBoolean}})

	for _, dependency := range []ResourceURI{"file:///lib"} {
		idx.Resources["file:///app"].Dependencies = append(idx.Resources["file:///app"].Dependencies, dependency)
	}

	cases := []struct {
		name      string
		className string
		fieldName string
		want      Type
	}{
		{name: "child declaration shadows parent", className: "Leaf", fieldName: "id", want: Type{Primitive: TypeBoolean}},
		{name: "parent declaration", className: "Leaf", fieldName: "label", want: Type{Primitive: TypeString}},
		{name: "grandparent declaration", className: "Middle", fieldName: "id", want: Type{Primitive: TypeNumber}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idx.LookupLuaDocClassField("file:///app", GlobalIndexScopeShared, tc.className, tc.fieldName)
			if got == nil || !got.Type.Equal(tc.want) {
				t.Fatalf("LookupLuaDocClassField(%q, %q) = %+v, want type %v", tc.className, tc.fieldName, got, tc.want)
			}
		})
	}
}

func TestGlobalIndexLuaDocClassFieldInheritanceCycleSafe(t *testing.T) {
	idx := NewGlobalIndex()
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "A", &SymbolEntry{Parent: "B"})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "B", &SymbolEntry{Parent: "A"})

	if got := idx.LookupLuaDocClassField("file:///app", GlobalIndexScopeShared, "A", "missing"); got != nil {
		t.Fatalf("cycle lookup = %+v, want nil", got)
	}
}

func BenchmarkGlobalIndexLuaDocClassFieldMultipleParents(b *testing.B) {
	idx := NewGlobalIndex()
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "A", &SymbolEntry{})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "A.fromA", &SymbolEntry{Type: Type{Primitive: TypeString}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "B", &SymbolEntry{})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "B.fromB", &SymbolEntry{Type: Type{Primitive: TypeNumber}})
	idx.AddSymbol("file:///app", GlobalIndexScopeShared, "Child", &SymbolEntry{Parents: []string{"A", "B"}})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = idx.LookupLuaDocClassField("file:///app", GlobalIndexScopeShared, "Child", "fromB")
	}
}
