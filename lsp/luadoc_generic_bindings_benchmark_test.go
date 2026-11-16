package lsp

import "testing"

var genericBindingsBenchmarkSink []map[string]string

func BenchmarkLuaDocGenericBindingsForHierarchy(b *testing.B) {
	for _, tc := range []struct {
		name  string
		entry *SymbolEntry
		args  []string
	}{
		{name: "ordinary", entry: &SymbolEntry{LuaDoc: &LuaDocData{}}},
		{name: "generic", entry: &SymbolEntry{LuaDoc: &LuaDocData{GenericClass: &LuaDocGenericClassData{Params: []LuaDocGeneric{{Name: "T"}}}}}, args: []string{"string"}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			hierarchy := []LuaDocClassHierarchyEntry{{Name: "Box", Entry: tc.entry, FirstParent: -1}}
			b.ReportAllocs()
			for b.Loop() {
				genericBindingsBenchmarkSink = luaDocGenericBindingsForHierarchy(hierarchy, tc.args)
			}
		})
	}
}
