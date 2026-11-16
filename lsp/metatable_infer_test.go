package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/semantic"
)

func TestInferMetatableIndexTables(t *testing.T) {
	src := []byte(`local base = { inherited = 42 }
local middle = setmetatable({ middle = "m" }, { __index = base })
local obj = setmetatable({ own = true }, { __index = middle })
local inherited = obj.inherited
local own = obj.own
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}

	for name, want := range map[string]BasicType{"inherited": TypeNumber, "own": TypeBoolean} {
		got := inferNamedMember(t, doc, name)
		if got.Basics&want == 0 {
			t.Errorf("%s type = %#v, want bit %#v", name, got, want)
		}
	}
}

func TestInferMetatableIndexFunction(t *testing.T) {
	src := []byte(`local obj = setmetatable({}, { __index = function(_, key) return "fallback" end })
local value = obj.answer
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	got := inferNamedMember(t, doc, "answer")
	if got.Basics&TypeString == 0 {
		t.Fatalf("function __index result = %#v, want string", got)
	}
}

func TestInferMetatableIndexPreservesDeclarationURI(t *testing.T) {
	src := []byte(`local base = { inherited = {} }
local meta = { __index = base }
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///library.lua"}
	var metaID ast.NodeID
	for id := 1; id < len(tree.Nodes); id++ {
		if tree.Nodes[id].Kind == ast.KindTableExpr && doc.findFieldInTable(ast.NodeID(id), "__index") != ast.InvalidNode {
			metaID = ast.NodeID(id)
			break
		}
	}
	if metaID == ast.InvalidNode {
		t.Fatal("metatable literal not found")
	}
	got, ok := doc.inferMetatableField(metaID, []byte("inherited"), make(map[metatableVisit]struct{}), 0)
	if !ok || got.Basics&TypeTable == 0 || got.DeclURI != doc.URI {
		t.Fatalf("inherited type = %#v, found=%t, want table declared in %q", got, ok, doc.URI)
	}
}

func TestInferMetatableCycleTerminates(t *testing.T) {
	src := []byte(`local firstMeta = {}
local secondMeta = {}
firstMeta.__index = secondMeta
secondMeta.__index = firstMeta
local a = setmetatable({}, firstMeta)
local value = a.missing
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	_ = inferNamedMember(t, doc, "missing")
}

func inferNamedMember(t *testing.T, doc *Document, name string) TypeSet {
	t.Helper()
	for id := 1; id < len(doc.Tree.Nodes); id++ {
		node := doc.Tree.Nodes[id]
		if node.Kind != ast.KindMemberExpr || node.Right == ast.InvalidNode {
			continue
		}
		if string(doc.nodeSource(node.Right)) == name {
			return doc.InferType(ast.NodeID(id))
		}
	}
	t.Fatalf("member %q not found", name)
	return TypeSet{}
}

func TestInferMetatableDoesNotFollowOrdinaryIndexField(t *testing.T) {
	src := []byte(`local unrelated = { forbidden = 42 }
local base = { __index = unrelated }
local obj = setmetatable({}, { __index = base })
local result = obj.forbidden
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	if got := inferNamedMember(t, doc, "forbidden"); got.Basics&TypeNumber != 0 {
		t.Fatalf("ordinary __index field was treated as metatable: %#v", got)
	}
}

func TestInferMetatableOwnFieldShadowsInherited(t *testing.T) {
	src := []byte(`local base = { value = "inherited" }
local obj = setmetatable({ value = 42 }, { __index = base })
local result = obj.value
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	if got := inferNamedMember(t, doc, "value"); got.Basics != TypeNumber {
		t.Fatalf("own field type = %#v, want number only", got)
	}
}

func TestInferShadowedSetmetatableIsNotBuiltin(t *testing.T) {
	src := []byte(`local setmetatable = function(value, meta) return "custom" end
local result = setmetatable({}, { __index = { value = true } })
local use = result
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	id := findIdentByOccurrence(t, tree, "result", 1)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("shadowed setmetatable result = %#v, want string", got)
	}
}

func TestInferGetmetatableReturnsAttachedTable(t *testing.T) {
	src := []byte(`local mt = { tag = "prototype" }
local obj = setmetatable({}, mt)
local recovered = getmetatable(obj)
local result = recovered.tag
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua", Server: NewServer("test")}
	if got := inferNamedMember(t, doc, "tag"); got.Basics != TypeString {
		t.Fatalf("getmetatable field = %#v, want string", got)
	}
}

func TestInferGetmetatableHonorsProtection(t *testing.T) {
	src := []byte(`local obj = setmetatable({}, { __metatable = "locked" })
local recovered = getmetatable(obj)
local result = recovered
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua", Server: NewServer("test")}
	id := findIdentByOccurrence(t, tree, "recovered", 1)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("protected metatable = %#v, want string", got)
	}
}

func TestInferGetmetatableAfterNilRemoval(t *testing.T) {
	src := []byte(`local obj = setmetatable({}, nil)
local recovered = getmetatable(obj)
local result = recovered
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua", Server: NewServer("test")}
	id := findIdentByOccurrence(t, tree, "recovered", 1)
	if got := doc.InferType(id); got.Basics != TypeNil {
		t.Fatalf("removed metatable = %#v, want nil", got)
	}
}

func TestInferMetatableNilOwnFieldFallsBack(t *testing.T) {
	src := []byte(`local base = { value = "inherited" }
local obj = setmetatable({ value = nil }, { __index = base })
local result = obj.value
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	if got := inferNamedMember(t, doc, "value"); got.Basics != TypeString {
		t.Fatalf("nil own field inference = %#v, want inherited string", got)
	}
}

func TestInferGetmetatableHonorsAssignedProtection(t *testing.T) {
	src := []byte(`local mt = {}
mt.__metatable = "locked"
local obj = setmetatable({}, mt)
local recovered = getmetatable(obj)
local result = recovered
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua", Server: NewServer("test")}
	id := findIdentByOccurrence(t, tree, "recovered", 1)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("protected metatable = %#v, want string", got)
	}
}

func TestInferMetatableStringIndexedMember(t *testing.T) {
	src := []byte(`local base = { value = "inherited" }
local obj = setmetatable({}, { __index = base })
local result = obj["value"]
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	for id, node := range tree.Nodes {
		if node.Kind == ast.KindIndexExpr {
			if got := doc.InferType(ast.NodeID(id)); got.Basics != TypeString {
				t.Fatalf("indexed member = %#v, want string", got)
			}
			return
		}
	}
	t.Fatal("index expression not found")
}

func TestInferMetatableStringKeyDefinition(t *testing.T) {
	src := []byte(`local base = { ["value"] = "inherited" }
local obj = setmetatable({}, { ["__index"] = base })
local result = obj.value
`)
	tree := parseResolverLua(t, src)
	resolver := semantic.New(tree)
	resolver.Resolve(tree.Root)
	doc := &Document{Tree: tree, Resolver: resolver, URI: "file:///test.lua"}
	if got := inferNamedMember(t, doc, "value"); got.Basics != TypeString {
		t.Fatalf("string key definition = %#v, want string", got)
	}
}

func TestInferMetatableIndexChainSkipsNilIntermediateField(t *testing.T) {
	doc := newIndexTestDocument(t, `local base = { value = "inherited" }
local middle = setmetatable({ value = nil }, { __index = base })
local object = setmetatable({}, { __index = middle })
local result = object.value
`)
	if got := inferLastNamedMember(t, doc, "value"); got.Basics&TypeString == 0 {
		t.Fatalf("nil intermediate field = %#v, want inherited string", got)
	}
}

func TestInferMetatableIndexChainWithSeparateAttachment(t *testing.T) {
	doc := newIndexTestDocument(t, `local base = { value = "inherited" }
local middle = {}
setmetatable(middle, { __index = base })
local object = setmetatable({}, { __index = middle })
local result = object.value
`)
	if got := inferLastNamedMember(t, doc, "value"); got.Basics&TypeString == 0 {
		t.Fatalf("separate intermediate attachment = %#v, want inherited string", got)
	}
}

func TestInferMetatableIndexReplacementFollowsAccessOrder(t *testing.T) {
	doc := newIndexTestDocument(t, `local first = { value = "before" }
local second = { value = 42 }
local mt = { __index = first }
local object = setmetatable({}, mt)
local a = object.value
mt.__index = second
local b = object.value
mt.__index = nil
local c = object.value
`)
	var got []TypeSet
	for id, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindMemberExpr && string(doc.nodeSource(node.Left)) == "object" && string(doc.nodeSource(node.Right)) == "value" {
			got = append(got, doc.InferType(ast.NodeID(id)))
		}
	}
	if len(got) != 3 || got[0].Basics != TypeString || got[1].Basics != TypeNumber || got[2].Basics == TypeString || got[2].Basics == TypeNumber {
		t.Fatalf("types across __index replacement = %#v, want string, number, unknown", got)
	}
}

func TestInferMetatableIndexAttachmentAfterObjectCreation(t *testing.T) {
	doc := newIndexTestDocument(t, `local root = { value = "inherited" }
local middle = {}
local object = setmetatable({}, { __index = middle })
local before = object.value
setmetatable(middle, { __index = root })
local after = object.value
`)
	var got []TypeSet
	for id, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindMemberExpr && string(doc.nodeSource(node.Left)) == "object" && string(doc.nodeSource(node.Right)) == "value" {
			got = append(got, doc.InferType(ast.NodeID(id)))
		}
	}
	if len(got) != 2 || got[0].Basics&TypeString != 0 || got[1].Basics&TypeString == 0 {
		t.Fatalf("types before and after attaching intermediate metatable = %#v", got)
	}
}

func TestInferMetatableIndexIgnoresUnreachableReplacement(t *testing.T) {
	doc := newIndexTestDocument(t, `local first = { value = "before" }
local second = { value = 42 }
local mt = { __index = first }
local object = setmetatable({}, mt)
if false then mt.__index = second end
local result = object.value
`)
	got := inferLastNamedMember(t, doc, "value")
	if got.Basics != TypeString {
		t.Fatalf("unreachable replacement type = %#v, want string", got)
	}
}

func TestInferMetatableIndexIgnoresUncalledFunctionReplacement(t *testing.T) {
	doc := newIndexTestDocument(t, `local first = { value = "before" }
local second = { value = 42 }
local mt = { __index = first }
local object = setmetatable({}, mt)
local function change() mt.__index = second end
local result = object.value
`)
	got := inferLastNamedMember(t, doc, "value")
	if got.Basics != TypeString {
		t.Fatalf("uncalled replacement type = %#v, want string", got)
	}
}

func TestInferMetatableIndexTableFieldReplacement(t *testing.T) {
	doc := newIndexTestDocument(t, `local base = { value = 42 }
local object = setmetatable({}, { __index = base })
local before = object.value
base.value = "updated"
local after = object.value
base.value = nil
local cleared = object.value
`)
	var got []TypeSet
	for id, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindMemberExpr && string(doc.nodeSource(node.Left)) == "object" && string(doc.nodeSource(node.Right)) == "value" {
			got = append(got, doc.InferType(ast.NodeID(id)))
		}
	}
	if len(got) != 3 || got[0].Basics != TypeNumber || got[1].Basics != TypeString || got[2].Basics&(TypeNumber|TypeString) != 0 {
		t.Fatalf("types across index table field changes = %#v", got)
	}
}

func TestInferGetMetatableProtectionReplacementFollowsOrder(t *testing.T) {
	doc := newIndexTestDocument(t, `local mt = { __metatable = "locked" }
local object = setmetatable({}, mt)
local before = getmetatable(object)
mt.__metatable = 42
local after = getmetatable(object)
mt.__metatable = nil
local cleared = getmetatable(object)
`)
	for _, tc := range []struct {
		name string
		want BasicType
	}{{"before", TypeString}, {"after", TypeNumber}, {"cleared", TypeTable}} {
		id := findIdentByOccurrence(t, doc.Tree, tc.name, 0)
		if got := doc.InferType(id); got.Basics != tc.want {
			t.Errorf("%s = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}
