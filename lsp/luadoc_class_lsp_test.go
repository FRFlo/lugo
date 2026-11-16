package lsp

import (
	"fmt"

	"slices"
	"strings"
	"testing"
)

func TestLuaDocClassLSPCrossDocumentFields(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Parent
---@field inherited string inherited field
local Parent = {}

---@class Child: Parent
---@field own number own field
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local value = {}
local a = value.inh--[[@hover]]erited
local b = value.inh--[[@definition]]erited
local c = value.--[[@completion]]
`)
	h.reindex()

	hover := h.hover("hover")
	if hover == nil || !containsText(hover.Contents, "inherited") || !containsText(hover.Contents, "string") {
		t.Errorf("inherited field hover = %#v, context=%+v, want indexed parent field and type", hover, h.resolve("hover"))
	}
	defs := h.definition("definition")
	if len(defs) == 0 || defs[0].URI != h.server.pathToURI(h.root+"/types.lua") {
		t.Errorf("inherited field definitions = %#v, want types.lua", defs)
	}
	labels := labelsFromItems(h.completion("completion").Items)
	for _, label := range []string{"inherited", "own"} {
		if !slices.Contains(labels, label) {
			t.Errorf("completion labels = %#v, missing %q", labels, label)
		}
	}
}

func TestLuaDocClassLSPMultipleParents(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class A
---@field fromA string
local A = {}
---@class B
---@field fromB number
local B = {}
---@class Child: A, B
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local value = {}
local first = value.fromA
local second = value.from--[[@multi_parent_definition]]B
local complete = value.--[[@multi_parent_completion]]
`)
	h.reindex()

	if got := inferNamedMember(t, h.server.Documents[h.server.pathToURI(h.root+"/use.lua")], "fromB"); got.Basics != TypeNumber {
		t.Errorf("second-parent field inference = %#v, want number", got)
	}
	definitions := h.definition("multi_parent_definition")
	if len(definitions) == 0 || definitions[0].URI != h.server.pathToURI(h.root+"/types.lua") {
		t.Fatalf("second-parent definition = %#v, want types.lua", definitions)
	}
	labels := labelsFromItems(h.completion("multi_parent_completion").Items)
	for _, label := range []string{"fromA", "fromB"} {
		if !slices.Contains(labels, label) {
			t.Errorf("multi-parent completion labels = %#v, missing %q", labels, label)
		}
	}
}

func TestLuaDocClassLSPInheritedFieldAcrossVisibleResourceChain(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("a/types.lua", `---@class Root
---@field inherited string inherited from A
local Root = {}

---@class Other
---@field unrelated number unrelated A class symbol
local Other = {}
`)
	h.writeWorkspaceFile("b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'a'\nshared_script '@a/types.lua'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("b/types.lua", `---@class Middle: Root
local Middle = {}
`)
	h.writeWorkspaceFile("c/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'b'\nshared_script '@b/types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("c/use.lua", `---@type Middle
local value = {}
local result = value.inh--[[@chain_definition]]erited
local bracket = value["inherited--[[@chain_bracket_reference]]"]
local complete = value.--[[@chain_completion]]
`)
	h.reindex()

	aDoc := h.server.Documents[h.server.pathToURI(h.root+"/a/types.lua")]
	bDoc := h.server.Documents[h.server.pathToURI(h.root+"/b/types.lua")]
	cDoc := h.server.Documents[h.server.pathToURI(h.root+"/c/use.lua")]
	if !h.server.canSeeSymbol(bDoc, aDoc) || !h.server.canSeeSymbol(cDoc, bDoc) {
		t.Fatal("fixture visibility must allow B→A and C→B")
	}
	if h.server.canSeeSymbol(cDoc, aDoc) {
		t.Fatal("C must not gain direct visibility of A")
	}

	labels := labelsFromItems(h.completion("chain_completion").Items)
	if !slices.Contains(labels, "inherited") {
		t.Errorf("C completion labels = %#v, want inherited Root field through Middle", labels)
	}
	if slices.Contains(labels, "unrelated") {
		t.Fatalf("C completion exposed unrelated A field: %#v", labels)
	}
	definitions := h.definition("chain_definition")
	if len(definitions) == 0 || definitions[0].URI != aDoc.URI {
		t.Fatalf("inherited field definitions = %#v, want Root field in A", definitions)
	}
	if got := inferNamedMember(t, cDoc, "inherited"); got.Basics != TypeString {
		t.Fatalf("inherited field type = %#v, want string", got)
	}
	if references := h.references("chain_definition", false); len(references) != 2 {
		t.Fatalf("inherited field references = %#v, want dot and bracket uses", references)
	}
}

func TestLuaDocClassLSPMultiParentVisibilityPaths(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("a/types.lua", `---@class ParentA
---@field fromA string
local ParentA = {}
`)
	h.writeWorkspaceFile("b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("b/types.lua", `---@class ParentB
---@field fromB number
local ParentB = {}
`)
	h.writeWorkspaceFile("c/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'a'\ndependency 'b'\nshared_script '@a/types.lua'\nshared_script '@b/types.lua'\nshared_script 'types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("c/types.lua", `---@class Child: ParentA, ParentB
local Child = {}
`)
	h.writeWorkspaceFile("c/use.lua", `---@type Child
local value = {}
local a = value.fromA--[[@multi_a_definition]]
local b = value.fromB--[[@multi_b_definition]]
local complete = value.--[[@multi_parent_resource_completion]]
`)
	h.reindex()

	aDoc := h.server.Documents[h.server.pathToURI(h.root+"/a/types.lua")]
	bDoc := h.server.Documents[h.server.pathToURI(h.root+"/b/types.lua")]
	childDoc := h.server.Documents[h.server.pathToURI(h.root+"/c/types.lua")]
	useDoc := h.server.Documents[h.server.pathToURI(h.root+"/c/use.lua")]
	if !h.server.canSeeSymbol(childDoc, aDoc) || !h.server.canSeeSymbol(childDoc, bDoc) {
		t.Fatal("child resource should directly see both parent resources")
	}
	if h.server.canSeeSymbol(aDoc, bDoc) || h.server.canSeeSymbol(bDoc, aDoc) {
		t.Fatal("parent resources should not see one another")
	}
	resource, scope := h.server.globalIndexContext(childDoc)
	field, path := h.server.GlobalIndex.LookupLuaDocClassFieldWithPath(resource, scope, "Child", "fromB")
	if field == nil || field.URI != bDoc.URI {
		t.Fatalf("scoped second-parent field = %#v, want declaration in B", field)
	}
	if !h.server.canSeeLuaDocClassField(childDoc, bDoc, path) {
		t.Fatalf("second-parent inheritance path = %#v, should validate C→B without traversing sibling A", path)
	}

	for _, tc := range []struct {
		marker string
		name   string
		want   BasicType
		uri    string
	}{
		{"multi_a_definition", "fromA", TypeString, aDoc.URI},
		{"multi_b_definition", "fromB", TypeNumber, bDoc.URI},
	} {
		if got := inferNamedMember(t, useDoc, tc.name); got.Basics != tc.want {
			t.Errorf("%s inferred type = %#v, want %v", tc.name, got, tc.want)
		}
		definitions := h.definition(tc.marker)
		if len(definitions) == 0 || definitions[0].URI != tc.uri {
			t.Errorf("%s definition = %#v, want %s", tc.name, definitions, tc.uri)
		}
	}
	labels := labelsFromItems(h.completion("multi_parent_resource_completion").Items)
	for _, name := range []string{"fromA", "fromB"} {
		if !slices.Contains(labels, name) {
			t.Errorf("multi-parent resource completion = %#v, missing %s", labels, name)
		}
	}
}

func TestLuaDocClassLSPSharedAncestorKeepsAlternateVisiblePath(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("d/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("d/types.lua", `---@class Grand
---@field fromGrand string
local Grand = {}
`)
	h.writeWorkspaceFile("a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("a/types.lua", `---@class ParentA: Grand
local ParentA = {}
`)
	h.writeWorkspaceFile("b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'd'\nshared_script '@d/types.lua'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("b/types.lua", `---@class ParentB: Grand
local ParentB = {}
`)
	h.writeWorkspaceFile("c/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'a'\ndependency 'b'\nshared_script '@a/types.lua'\nshared_script '@b/types.lua'\nshared_script 'types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("c/types.lua", `---@class Child: ParentA, ParentB
local Child = {}
`)
	h.writeWorkspaceFile("c/use.lua", `---@type Child
local value = {}
local result = value.fro--[[@shared_ancestor_definition]]mGrand
local complete = value.--[[@shared_ancestor_completion]]
`)
	h.reindex()

	aDoc := h.server.Documents[h.server.pathToURI(h.root+"/a/types.lua")]
	bDoc := h.server.Documents[h.server.pathToURI(h.root+"/b/types.lua")]
	dDoc := h.server.Documents[h.server.pathToURI(h.root+"/d/types.lua")]
	cDoc := h.server.Documents[h.server.pathToURI(h.root+"/c/use.lua")]
	if h.server.canSeeSymbol(aDoc, dDoc) || !h.server.canSeeSymbol(bDoc, dDoc) || h.server.canSeeSymbol(cDoc, dDoc) {
		t.Fatal("fixture should expose D through B only, not through A or directly from C")
	}
	if got := inferNamedMember(t, cDoc, "fromGrand"); got.Basics != TypeString {
		t.Fatalf("shared ancestor field type = %#v, want string through B", got)
	}
	definitions := h.definition("shared_ancestor_definition")
	if len(definitions) == 0 || definitions[0].URI != dDoc.URI {
		t.Fatalf("shared ancestor definition = %#v, want D", definitions)
	}
	labels := labelsFromItems(h.completion("shared_ancestor_completion").Items)
	if !slices.Contains(labels, "fromGrand") {
		t.Fatalf("shared ancestor completion = %#v, want fromGrand through valid B path", labels)
	}
}

func TestLuaDocClassLSPParentNameCollisionStaysScoped(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("a/types.lua", `---@class Base
---@field secret string unrelated Base field from A
local Base = {}

---@type Base
Core.Base = {}
function Core.Base:secretFromAlias() end
`)
	h.writeWorkspaceFile("b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("b/types.lua", `---@class Base
local Base = {}

---@class Child: Base
local Child = {}
`)
	h.writeWorkspaceFile("c/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'a'\ndependency 'b'\nshared_script '@a/types.lua'\nshared_script '@b/types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("c/use.lua", `---@type Child
local value = {}
local result = value.sec--[[@collision_definition]]ret
local complete = value:--[[@collision_completion]]
`)
	h.reindex()

	aDoc := h.server.Documents[h.server.pathToURI(h.root+"/a/types.lua")]
	bDoc := h.server.Documents[h.server.pathToURI(h.root+"/b/types.lua")]
	cDoc := h.server.Documents[h.server.pathToURI(h.root+"/c/use.lua")]
	if !h.server.canSeeSymbol(cDoc, aDoc) || !h.server.canSeeSymbol(cDoc, bDoc) {
		t.Fatal("fixture should make both resources directly visible to C")
	}
	if h.server.canSeeSymbol(bDoc, aDoc) {
		t.Fatal("B should not directly see A")
	}
	if len(h.server.TableAliases) == 0 {
		t.Fatal("fixture should register the A Base→Core.Base table alias")
	}
	labels := labelsFromItems(h.completion("collision_completion").Items)
	if slices.Contains(labels, "secret") || slices.Contains(labels, "secretFromAlias") {
		t.Fatalf("Child completion leaked unrelated A.Base.secret: %#v", labels)
	}
	if definitions := h.definition("collision_definition"); len(definitions) != 0 {
		t.Fatalf("Child.secret resolved to unrelated A.Base.secret: %#v", definitions)
	}
	if got := inferNamedMember(t, cDoc, "secret"); got.Basics == TypeString {
		t.Fatalf("Child.secret inferred unrelated A.Base.secret: %#v", got)
	}
}

func TestLuaDocClassAliasSamePathStaysScopedByResource(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("b/types.lua", `---@class AccountB
local AccountB = {}
---@type AccountB
Core.Account = {}
function Core.Account:fromB--[[@alias_fromB_definition]]() end
`)
	h.writeWorkspaceFile("a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'b'\nshared_script '@b/types.lua'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("a/types.lua", `---@class AccountA
local AccountA = {}
---@type AccountA
Core.Account = {}
function Core.Account:fromA--[[@alias_fromA_definition]]() end
`)
	h.writeWorkspaceFile("c/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'a'\ndependency 'b'\nshared_script '@a/types.lua'\nshared_script '@b/types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("c/use.lua", `---@type AccountA
local value = {}
local leaked = value.fromB--[[@alias_collision_definition]]()
local valid = value.fromA--[[@alias_fromA_use]]()
local complete = value:--[[@alias_collision_completion]]
`)
	h.reindex()

	aDoc := h.server.Documents[h.server.pathToURI(h.root+"/a/types.lua")]
	bDoc := h.server.Documents[h.server.pathToURI(h.root+"/b/types.lua")]
	cDoc := h.server.Documents[h.server.pathToURI(h.root+"/c/use.lua")]
	if !h.server.canSeeSymbol(aDoc, bDoc) || !h.server.canSeeSymbol(cDoc, aDoc) || !h.server.canSeeSymbol(cDoc, bDoc) {
		t.Fatal("fixture should make B visible to A and both A and B visible to C")
	}
	labels := labelsFromItems(h.completion("alias_collision_completion").Items)
	if !slices.Contains(labels, "fromA") {
		t.Fatalf("AccountA completion lost its own Core.Account method: %#v", labels)
	}
	if slices.Contains(labels, "fromB") {
		t.Fatalf("AccountA completion leaked method from AccountB's Core.Account alias: %#v", labels)
	}
	if definitions := h.definition("alias_collision_definition"); len(definitions) != 0 {
		t.Fatalf("AccountA.fromB resolved to AccountB's Core.Account method: %#v", definitions)
	}
	if definitions := h.definition("alias_fromA_use"); len(definitions) != 1 || definitions[0].URI != aDoc.URI {
		t.Fatalf("AccountA.fromA navigation = %#v, want its definition in resource A", definitions)
	}
	if references := h.references("alias_fromA_definition", true); len(references) != 2 {
		t.Fatalf("AccountA.fromA references = %#v, want its declaration and C use", references)
	}
	if references := h.references("alias_fromB_definition", true); len(references) != 1 {
		t.Fatalf("AccountB.fromB references leaked into AccountA: %#v", references)
	}
	if got := inferNamedMember(t, cDoc, "fromB"); got.Basics == TypeFunction {
		t.Fatalf("AccountA.fromB inferred from AccountB's Core.Account alias: %#v", got)
	}
	if got := inferNamedMember(t, cDoc, "fromA"); got.Basics != TypeFunction {
		t.Fatalf("AccountA.fromA did not infer from its own Core.Account alias: %#v", got)
	}
}

func TestLuaDocClassResourceDoesNotMergeSameNameFieldsFromDependency(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("a/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("a/types.lua", `---@class Base
---@field secret string from A.Base
local Base = {}
`)
	h.writeWorkspaceFile("b/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'a'\nshared_script '@a/types.lua'\nshared_script 'types.lua'\n")
	h.writeWorkspaceFile("b/types.lua", `---@class Base
local Base = {}

---@class Child: Base
local Child = {}
`)
	h.writeWorkspaceFile("c/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'b'\nshared_script '@b/types.lua'\nclient_script 'use.lua'\n")
	h.writeWorkspaceFile("c/use.lua", `---@type Child
local value = {}
local result = value.sec--[[@same_name_dependency_definition]]ret
local complete = value.--[[@same_name_dependency_completion]]
`)
	h.reindex()

	aDoc := h.server.Documents[h.server.pathToURI(h.root+"/a/types.lua")]
	bDoc := h.server.Documents[h.server.pathToURI(h.root+"/b/types.lua")]
	cDoc := h.server.Documents[h.server.pathToURI(h.root+"/c/use.lua")]
	if !h.server.canSeeSymbol(bDoc, aDoc) || h.server.canSeeSymbol(cDoc, aDoc) {
		t.Fatal("B should see A directly, while C sees only B")
	}
	if labels := labelsFromItems(h.completion("same_name_dependency_completion").Items); slices.Contains(labels, "secret") {
		t.Fatalf("Child completion leaked A.Base.secret through B.Base shadow: %#v", labels)
	}
	if definitions := h.definition("same_name_dependency_definition"); len(definitions) != 0 {
		t.Fatalf("Child.secret resolved to shadowed A.Base field: %#v", definitions)
	}
	if got := inferNamedMember(t, cDoc, "secret"); got.Basics == TypeString {
		t.Fatalf("Child.secret inferred shadowed A.Base field: %#v", got)
	}
}

func TestLuaDocClassLSPChildFieldShadowsParent(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Parent
---@field value string parent description
local Parent = {}
---@class Child: Parent
---@field value number child description
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local value = {}
local a = value.val--[[@hover]]ue
`)
	h.reindex()
	hover := h.hover("hover")
	if hover == nil || !containsText(hover.Contents, "number") || containsText(hover.Contents, "string") {
		t.Fatalf("shadowed field hover = %#v, want child's number field only", hover)
	}
}

func TestLuaDocClassLSPInheritanceCycleTerminates(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class CycleA: CycleB
---@field own string
local CycleA = {}
---@class CycleB: CycleA
---@field other number
local CycleB = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type CycleA
local value = {}
local c = value.--[[@completion]]
`)
	h.reindex()
	labels := labelsFromItems(h.completion("completion").Items)
	for _, label := range []string{"own", "other"} {
		if !slices.Contains(labels, label) {
			t.Errorf("cycle completion labels = %#v, missing %q", labels, label)
		}
	}
}

func containsText(contents MarkupContent, want string) bool {
	return strings.Contains(contents.Value, want)
}

func TestLuaDocClassInheritedFieldTypeInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
---@field name string
local Base = {}
---@class Derived: Base
local Derived = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Derived
local item = {}
local result = item.name
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "name"); got.Basics != TypeString {
		t.Fatalf("inherited field inference = %#v, want string", got)
	}
}

func TestLuaDocClassLSPRespectsFiveMExecutionProfile(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
client_scripts { 'client_types.lua', 'client.lua' }
server_scripts { 'server_types.lua', 'server.lua' }
`)
	h.writeWorkspaceFile("resource/client_types.lua", `---@class SideClass
---@field clientOnly string
local SideClass = {}
`)
	h.writeWorkspaceFile("resource/server_types.lua", `---@class SideClass
---@field serverOnly number
local SideClass = {}
`)
	h.writeWorkspaceFile("resource/client.lua", `---@type SideClass
local value = {}
local x = value.--[[@client_complete]]
`)
	h.writeWorkspaceFile("resource/server.lua", `---@type SideClass
local value = {}
local x = value.--[[@server_complete]]
`)
	h.reindex()
	for _, tc := range []struct{ marker, visible, hidden string }{
		{"client_complete", "clientOnly", "serverOnly"},
		{"server_complete", "serverOnly", "clientOnly"},
	} {
		t.Run(tc.marker, func(t *testing.T) {
			labels := labelsFromItems(h.completion(tc.marker).Items)
			if !slices.Contains(labels, tc.visible) || slices.Contains(labels, tc.hidden) {
				t.Fatalf("completion labels = %#v, want %s without %s", labels, tc.visible, tc.hidden)
			}
		})
	}
}

func TestLuaDocClassNamedFieldChainInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Address
---@field city string
local Address = {}
---@class Person
---@field address Address
local Person = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Person
local person = {}
local result = person.address.city
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "address"); got.CustomName != "Address" {
		t.Errorf("address type = %#v, want Address", got)
	}
	if got := inferNamedMember(t, doc, "city"); got.Basics != TypeString {
		t.Errorf("city type = %#v, want string", got)
	}
}

func TestLuaDocClassFieldReferencesIncludeUsages(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Foo
---@field bar number
local Foo = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Foo
local item = {}
local first = item.b--[[@field_refs]]ar
local second = item.bar
`)
	h.reindex()
	refs := h.references("field_refs", false)
	if len(refs) != 2 {
		t.Fatalf("field references = %#v, want both uses", refs)
	}
}

func TestLuaDocDeepInheritanceFieldInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	var source strings.Builder
	source.WriteString("---@class Base\n---@field value string\nlocal Base = {}\n")
	previous := "Base"
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("Class%d", i)
		fmt.Fprintf(&source, "---@class %s: %s\nlocal %s = {}\n", name, previous, name)
		previous = name
	}
	h.writeWorkspaceFile("types.lua", source.String())
	h.writeWorkspaceFile("use.lua", "---@type "+previous+"\nlocal object = {}\nlocal result = object.val--[[@deep_inherit_definition]]ue\nlocal complete = object.--[[@deep_inherit_completion]]\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	if got := inferNamedMember(t, doc, "value"); got.Basics != TypeString {
		t.Fatalf("deep inherited field = %#v, want string", got)
	}
	if definitions := h.definition("deep_inherit_definition"); len(definitions) == 0 || definitions[0].URI != h.server.pathToURI(h.root+"/types.lua") {
		t.Fatalf("deep inherited definition = %#v, want base declaration", definitions)
	}
	if labels := labelsFromItems(h.completion("deep_inherit_completion").Items); !slices.Contains(labels, "value") {
		t.Fatalf("deep inherited completion = %#v, want value", labels)
	}
}

func TestLuaDocInheritedCallableFieldSignatureHelp(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
---@field onChange fun(input: number): string
local Base = {}
---@class Child: Base
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local object = {}
local result = object.onChange(--[[@inherited_field_signature]])
`)
	h.reindex()

	help := h.signatureHelp("inherited_field_signature")
	if help == nil || len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "input") {
		t.Fatalf("signature help = %#v, want inherited callable field signature", help)
	}
}

func TestLuaDocInheritedCallableFieldReturnInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
---@field onChange fun(input: number): string
local Base = {}
---@class Child: Base
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local object = {}
local result = object.onChange(42)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics != TypeString {
		t.Fatalf("call return = %#v, want string", got)
	}
}

func BenchmarkLuaDocDeepInheritanceCompletionMemory(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	var source strings.Builder
	source.WriteString("---@class Base\n---@field value string\nlocal Base = {}\n")
	previous := "Base"
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("Class%d", i)
		fmt.Fprintf(&source, "---@class %s: %s\nlocal %s = {}\n", name, previous, name)
		previous = name
	}
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&source, "Global%d = {}\n", i)
	}
	h.writeWorkspaceFile("types.lua", source.String())
	h.writeWorkspaceFile("use.lua", "---@type "+previous+"\nlocal object = {}\nobject.--[[@deep_completion_bench]]\n")
	h.reindex()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = h.completion("deep_completion_bench")
	}
}

func TestLuaDocInheritedCallableFieldColonSignatureHelp(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
---@field onChange fun(self: Base, amount: number): string
local Base = {}
---@class Child: Base
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local object = {}
local result = object:onChange(--[[@inherited_colon_signature]])
`)
	h.reindex()
	help := h.signatureHelp("inherited_colon_signature")
	if help == nil || len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "amount") || strings.Contains(help.Signatures[0].Label, "self") {
		t.Fatalf("colon signature = %#v, want amount without implicit self", help)
	}
}

func TestLuaDocInheritedCallableFieldColonReturnInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
---@field onChange fun(self: Base, amount: number): string
local Base = {}
---@class Child: Base
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local object = {}
local result = object:onChange(42)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/use.lua")]
	got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
	if got.Basics != TypeString {
		t.Fatalf("colon call return = %#v, want string", got)
	}
}

func TestLuaDocInheritedCallableFieldIndexedSignatureHelp(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Base
---@field onChange fun(input: number): string
local Base = {}
---@class Child: Base
local Child = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local object = {}
local result = object["onChange"](--[[@inherited_indexed_signature]])
`)
	h.reindex()
	help := h.signatureHelp("inherited_indexed_signature")
	if help == nil || len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "input") {
		t.Fatalf("indexed field signature = %#v, want input parameter", help)
	}
}
