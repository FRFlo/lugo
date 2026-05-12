package lsp

import (
	"testing"

	"github.com/coalaura/lugo/ast"
	"github.com/coalaura/lugo/parser"
	"github.com/coalaura/lugo/semantic"
)

func TestResolver(t *testing.T) {
	t.Run("ForwardRef", func(t *testing.T) {
		tree := parseResolverLua(t, []byte("local x = foo()\nfunction foo() return 1 end\n"))
		resolver := NewResolver(tree, ResolverOptions{SemanticData: NewSemanticDataTable()})
		if err := resolver.Resolve(tree.Root); err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}

		fooCall := findIdentByOccurrence(t, tree, "foo", 0)
		fooDef := findIdentByOccurrence(t, tree, "foo", 1)
		if resolver.References[fooCall] != fooDef {
			t.Fatalf("forward reference foo binding = %d, want %d", resolver.References[fooCall], fooDef)
		}

		xDef := findIdentByOccurrence(t, tree, "x", 0)
		data := resolver.Data.Get(NodeID(xDef))
		if data == nil || data.Type.Primitive != TypeNumber {
			t.Fatalf("x type = %#v, want number from foo return", data)
		}
	})

	t.Run("Phase1NoInfer", func(t *testing.T) {
		tree := parseResolverLua(t, []byte("local x = 1\nfunction foo() return x end\n"))
		resolver := NewResolver(tree, ResolverOptions{SemanticData: NewSemanticDataTable()})
		if err := resolver.Phase1(tree.Root); err != nil {
			t.Fatalf("Phase1() error = %v", err)
		}
		if resolver.Phase != ResolverPhaseDeclarations {
			t.Fatalf("phase = %v, want declarations", resolver.Phase)
		}
		if len(resolver.LocalDefs) == 0 || len(resolver.GlobalDefs) == 0 {
			t.Fatalf("Phase1 declarations local=%d global=%d, want both", len(resolver.LocalDefs), len(resolver.GlobalDefs))
		}
		for nodeID, data := range resolver.Data.data {
			if data.Type.Primitive != TypeUnknown || data.Type.Structural != nil {
				t.Fatalf("Phase1 node %d has type %#v, want zero type", nodeID, data.Type)
			}
		}
	})

	t.Run("FiveMCascade", func(t *testing.T) {
		idx := NewGlobalIndex()
		idx.RegisterFiveMResource(&FiveMResource{Name: "dep", RootURI: "dep"}, true)
		idx.RegisterFiveMResource(&FiveMResource{Name: "main", RootURI: "main", Dependencies: []string{"dep"}}, true)
		tree := parseResolverLua(t, []byte("local x = NativeThing\n"))
		state := NewResolverPhaseState()
		state.MarkPhase1Complete("main")
		resolver := NewResolver(tree, ResolverOptions{FeatureFiveM: true, ResourceURI: "main", Index: idx, PhaseState: state})
		if err := resolver.Phase1(tree.Root); err != nil {
			t.Fatalf("Phase1() error = %v", err)
		}
		if err := resolver.Phase2(tree.Root); err == nil {
			t.Fatal("Phase2() error = nil, want dependency wait")
		}
		state.MarkPhase1Complete("dep")
		if err := resolver.Phase2(tree.Root); err != nil {
			t.Fatalf("Phase2() after dependency phase1 error = %v", err)
		}
	})

	t.Run("FiveMIndexType", func(t *testing.T) {
		idx := NewGlobalIndex()
		idx.AddSymbol("res", GlobalIndexScopeShared, "GetPlayerName", &SymbolEntry{Key: GlobalKey{PropHash: ast.HashBytes([]byte("GetPlayerName"))}, Type: Type{Primitive: TypeFunction, Structural: &StructuralType{Function: &FunctionType{Returns: []Type{{Primitive: TypeString}}}}}})
		tree := parseResolverLua(t, []byte("local name = GetPlayerName()\n"))
		resolver := NewResolver(tree, ResolverOptions{FeatureFiveM: true, ResourceURI: "res", Index: idx})
		if err := resolver.Resolve(tree.Root); err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		nameDef := findIdentByOccurrence(t, tree, "name", 0)
		data := resolver.Data.Get(NodeID(nameDef))
		if data == nil || data.Type.Primitive != TypeString {
			t.Fatalf("FiveM native result type = %#v, want string", data)
		}
	})

	t.Run("LocalInitializerUsesOuterScope", func(t *testing.T) {
		tree := parseResolverLua(t, []byte("local x = 1\ndo\n  local x = x\nend\n"))
		resolver := NewResolver(tree, ResolverOptions{})
		if err := resolver.Phase1(tree.Root); err != nil {
			t.Fatalf("Phase1() error = %v", err)
		}

		outer := findIdentByOccurrence(t, tree, "x", 0)
		rhs := findIdentByOccurrence(t, tree, "x", 2)
		if resolver.References[rhs] != outer {
			t.Fatalf("local initializer rhs binding = %d, want outer %d", resolver.References[rhs], outer)
		}
	})

	t.Run("GlobalAssignmentType", func(t *testing.T) {
		tree := parseResolverLua(t, []byte("Foo = function() return 1 end\nlocal x = Foo()\n"))
		resolver := NewResolver(tree, ResolverOptions{})
		if err := resolver.Resolve(tree.Root); err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}

		xDef := findIdentByOccurrence(t, tree, "x", 0)
		data := resolver.Data.Get(NodeID(xDef))
		if data == nil || data.Type.Primitive != TypeNumber {
			t.Fatalf("global assignment call result type = %#v, want number", data)
		}
	})

	t.Run("TableFieldBindingAndPublish", func(t *testing.T) {
		idx := NewGlobalIndex()
		tree := parseResolverLua(t, []byte("local T = { a = 1 }\nlocal x = T.a\n"))
		resolver := NewResolver(tree, ResolverOptions{ResourceURI: "res", Index: idx})
		if err := resolver.Resolve(tree.Root); err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}

		fieldDef := findIdentByOccurrence(t, tree, "a", 0)
		fieldRef := findIdentByOccurrence(t, tree, "a", 1)
		if resolver.References[fieldRef] != fieldDef {
			t.Fatalf("table field reference = %d, want %d", resolver.References[fieldRef], fieldDef)
		}
		xDef := findIdentByOccurrence(t, tree, "x", 0)
		data := resolver.Data.Get(NodeID(xDef))
		if data == nil || data.Type.Primitive != TypeNumber {
			t.Fatalf("table field assigned type = %#v, want number", data)
		}
		entries := idx.LookupByHash(GlobalKey{ReceiverHash: ast.HashBytes([]byte("T")), PropHash: ast.HashBytes([]byte("a"))})
		if len(entries) == 0 || entries[0].Type.Primitive != TypeNumber {
			t.Fatalf("published field entries = %#v, want number field", entries)
		}
	})
}

func TestForwardRef(t *testing.T) {
	tree := parseResolverLua(t, []byte("local x = foo()\nfunction foo() return 1 end\n"))
	resolver := NewResolver(tree, ResolverOptions{})
	if err := resolver.Resolve(tree.Root); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	fooCall := findIdentByOccurrence(t, tree, "foo", 0)
	fooDef := findIdentByOccurrence(t, tree, "foo", 1)
	if resolver.References[fooCall] != fooDef {
		t.Fatalf("forward reference = %d, want %d", resolver.References[fooCall], fooDef)
	}
}

func TestPhase1NoInfer(t *testing.T) {
	tree := parseResolverLua(t, []byte("local x = 1\n"))
	resolver := NewResolver(tree, ResolverOptions{})
	if err := resolver.Phase1(tree.Root); err != nil {
		t.Fatalf("Phase1() error = %v", err)
	}
	for _, data := range resolver.Data.data {
		if data.Type.Primitive != TypeUnknown || data.Type.Structural != nil {
			t.Fatalf("Phase1 type = %#v, want zero", data.Type)
		}
	}
}

func TestParity(t *testing.T) {
	t.Run("Resolver", func(t *testing.T) {
		src := []byte("local a = 1\nlocal function foo() return a end\nlocal b = foo()\n")
		treeOld := parseResolverLua(t, src)
		old := semantic.New(treeOld)
		old.Resolve(treeOld.Root)

		treeNew := parseResolverLua(t, src)
		resolver := NewResolver(treeNew, ResolverOptions{FeatureFiveM: false})
		if err := resolver.Phase1(treeNew.Root); err != nil {
			t.Fatalf("Phase1() error = %v", err)
		}

		if len(old.References) != len(resolver.References) {
			t.Fatalf("reference len = %d, want %d", len(resolver.References), len(old.References))
		}
		for id := range old.References {
			if old.References[id] != resolver.References[id] {
				t.Fatalf("reference[%d] = %d, want %d", id, resolver.References[id], old.References[id])
			}
		}
		if len(old.LocalDefs) != len(resolver.LocalDefs) {
			t.Fatalf("local defs = %d, want %d", len(resolver.LocalDefs), len(old.LocalDefs))
		}
		if len(old.GlobalRefs) != len(resolver.GlobalRefs) || len(old.GlobalDefs) != len(resolver.GlobalDefs) {
			t.Fatalf("globals refs/defs = %d/%d, want %d/%d", len(resolver.GlobalRefs), len(resolver.GlobalDefs), len(old.GlobalRefs), len(old.GlobalDefs))
		}
	})
}

func parseResolverLua(t *testing.T, src []byte) *ast.Tree {
	t.Helper()
	tree := ast.NewTree(src)
	p := parser.New(src, tree, 0)
	p.Parse()
	if len(p.Errors) != 0 {
		t.Fatalf("parse errors = %#v", p.Errors)
	}
	return tree
}

func findIdentByOccurrence(t *testing.T, tree *ast.Tree, name string, occurrence int) ast.NodeID {
	t.Helper()
	seen := 0
	for id := 1; id < len(tree.Nodes); id++ {
		node := tree.Nodes[id]
		if node.Kind != ast.KindIdent || node.Start > node.End || node.End > uint32(len(tree.Source)) {
			continue
		}
		if string(tree.Source[node.Start:node.End]) == name {
			if seen == occurrence {
				return ast.NodeID(id)
			}
			seen++
		}
	}
	t.Fatalf("identifier %q occurrence %d not found", name, occurrence)
	return ast.InvalidNode
}
