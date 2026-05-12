package lsp

import (
	"testing"

	"github.com/coalaura/lugo/ast"
)

func TestInferColonMethodBindsSelf(t *testing.T) {
	tree := parseResolverLua(t, []byte("obj:method()\n"))
	doc := &Document{Tree: tree}
	call := findMethodCallByName(t, tree, "method")

	receiver := &StructuralType{Fields: map[string]Type{}}
	methodFn := &FunctionType{Returns: []Type{{Primitive: TypeString}}}
	receiver.Fields["method"] = Type{Primitive: TypeFunction, Structural: &StructuralType{Function: methodFn}}

	got := InferColonMethod(doc, call, Type{Primitive: TypeTable, Structural: receiver})
	if got.Primitive != TypeString {
		t.Fatalf("InferColonMethod() = %#v, want string", got)
	}
	if methodFn.SelfType != nil {
		t.Fatalf("method self = %#v, want nil on cached function type", methodFn.SelfType)
	}
}

func TestInferMethodChain(t *testing.T) {
	src := []byte(`local obj = {
  first = function()
    return { second = function()
      return { third = function() return 1 end }
    end }
  end,
}
local x = obj:first():second():third()
`)
	tree := parseResolverLua(t, src)
	doc := &Document{Tree: tree}
	chain := []ast.NodeID{
		findMethodCallByName(t, tree, "first"),
		findMethodCallByName(t, tree, "second"),
		findMethodCallByName(t, tree, "third"),
	}

	got := InferMethodChain(doc, chain)
	if got.Primitive != TypeNumber {
		t.Fatalf("InferMethodChain() = %#v, want number", got)
	}
}

func TestInferDotCallDoesNotBindSelf(t *testing.T) {
	src := []byte(`local obj = { method = function() return 1 end }
local x = obj.method()
`)
	tree := parseResolverLua(t, src)
	doc := &Document{Tree: tree}
	assign := findLocalAssignByName(t, tree, "x")

	got := InferAssignment(doc, assign)
	if got.Primitive != TypeNumber {
		t.Fatalf("InferAssignment(dot call) = %#v, want number", got)
	}

	tableNode := findFirstNodeByKind(t, tree, ast.KindTableExpr)
	objType := InferTableLiteral(doc, tableNode)
	method := objType.Structural.Fields["method"]
	if method.Structural == nil || method.Structural.Function == nil {
		t.Fatalf("method type = %#v, want function", method)
	}
	if method.Structural.Function.SelfType != nil {
		t.Fatalf("dot call self = %#v, want nil", method.Structural.Function.SelfType)
	}
}

func TestInferAssignment(t *testing.T) {
	tree := parseResolverLua(t, []byte("local x = 42\n"))
	doc := &Document{Tree: tree}
	assign := findLocalAssignByName(t, tree, "x")

	got := InferAssignment(doc, assign)
	if got.Primitive != TypeNumber {
		t.Fatalf("InferAssignment() = %#v, want number", got)
	}
}

func TestInferTableLiteral(t *testing.T) {
	tree := parseResolverLua(t, []byte("local obj = { name = 'lugo', count = 1, ['tag'] = 'fast' }\n"))
	doc := &Document{Tree: tree}
	tableNode := findFirstNodeByKind(t, tree, ast.KindTableExpr)

	got := InferTableLiteral(doc, tableNode)
	if got.Primitive != TypeTable || got.Structural == nil {
		t.Fatalf("InferTableLiteral() = %#v, want structural table", got)
	}
	if got.Structural.Fields["name"].Primitive != TypeString {
		t.Fatalf("name field = %#v, want string", got.Structural.Fields["name"])
	}
	if got.Structural.Fields["count"].Primitive != TypeNumber {
		t.Fatalf("count field = %#v, want number", got.Structural.Fields["count"])
	}
	if got.Structural.Fields["tag"].Primitive != TypeString {
		t.Fatalf("tag field = %#v, want string", got.Structural.Fields["tag"])
	}
}

func findMethodCallByName(t *testing.T, tree *ast.Tree, name string) ast.NodeID {
	t.Helper()
	for id := 1; id < len(tree.Nodes); id++ {
		node := tree.Nodes[id]
		if node.Kind != ast.KindMethodCall || node.Right == ast.InvalidNode {
			continue
		}
		right := tree.Nodes[node.Right]
		if right.Start <= right.End && right.End <= uint32(len(tree.Source)) && string(tree.Source[right.Start:right.End]) == name {
			return ast.NodeID(id)
		}
	}
	t.Fatalf("method call %q not found", name)
	return ast.InvalidNode
}

func findLocalAssignByName(t *testing.T, tree *ast.Tree, name string) ast.NodeID {
	t.Helper()
	for id := 1; id < len(tree.Nodes); id++ {
		node := tree.Nodes[id]
		if node.Kind != ast.KindLocalAssign || node.Left == ast.InvalidNode {
			continue
		}
		left := tree.Nodes[node.Left]
		if left.Kind != ast.KindNameList {
			continue
		}
		for i := uint16(0); i < left.Count; i++ {
			identID := tree.ExtraList[left.Extra+uint32(i)]
			ident := tree.Nodes[identID]
			if ident.Start <= ident.End && ident.End <= uint32(len(tree.Source)) && string(tree.Source[ident.Start:ident.End]) == name {
				return ast.NodeID(id)
			}
		}
	}
	t.Fatalf("local assignment %q not found", name)
	return ast.InvalidNode
}

func findFirstNodeByKind(t *testing.T, tree *ast.Tree, kind ast.NodeKind) ast.NodeID {
	t.Helper()
	for id := 1; id < len(tree.Nodes); id++ {
		if tree.Nodes[id].Kind == kind {
			return ast.NodeID(id)
		}
	}
	t.Fatalf("node kind %v not found", kind)
	return ast.InvalidNode
}
