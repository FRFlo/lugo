package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestMetatableNavigationToIndexMember(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local methods = { run = function(self) return true end }
local proto = { __index = methods }
local object = setmetatable({}, proto)
object:--[[@meta_run]]run()
`)
	h.reindex()

	ctx := h.resolve("meta_run")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("metatable member did not resolve: %#v", ctx)
	}
	if got, want := string(h.docForMarker("meta_run").nodeSource(ctx.TargetDefID)), "run"; got != want {
		t.Fatalf("definition text = %q, want %q", got, want)
	}
	defs := h.definition("meta_run")
	if len(defs) != 1 || defs[0].URI != ctx.TargetURI {
		t.Fatalf("definition locations = %#v, want one location in %s", defs, ctx.TargetURI)
	}
	if h.hover("meta_run") == nil {
		t.Fatal("hover on metatable member returned nil")
	}
	refs := h.references("meta_run", true)
	if len(refs) != 2 {
		t.Fatalf("references = %#v, want declaration and metatable access", refs)
	}
}

func TestMetatableNavigationThroughIndexVariableAndCycle(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local methods = { run = function(self) return true end }
local first = {}
local second = {}
setmetatable(first, { __index = second })
setmetatable(second, { __index = methods })
setmetatable(methods, { __index = first })
first:--[[@meta_cycle]]run()
`)
	h.reindex()

	ctx := h.resolve("meta_cycle")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("cyclic metatable chain did not resolve: %#v", ctx)
	}
}

func TestMetatableNavigationPrefersOwnMember(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { run = function() return "base" end }
local object = setmetatable({ run = function() return "own" end }, { __index = base })
object:--[[@own_run]]run()
`)
	h.reindex()
	ctx := h.resolve("own_run")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("own member did not resolve: %#v", ctx)
	}
	doc := h.docForMarker("own_run")
	if ctx.TargetDoc != doc || ctx.TargetDefID != findIdentByOccurrence(t, doc.Tree, "run", 1) {
		t.Fatalf("own member resolved to %#v, want own declaration", ctx)
	}
}

func TestMetatableNavigationThroughInheritedTable(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local root = { run = function() return true end }
local child = setmetatable({}, { __index = root })
local object = setmetatable({}, { __index = child })
object:--[[@inherited_run]]run()
`)
	h.reindex()
	ctx := h.resolve("inherited_run")
	doc := h.docForMarker("inherited_run")
	if ctx == nil || ctx.TargetDoc != doc || ctx.TargetDefID != findIdentByOccurrence(t, doc.Tree, "run", 0) {
		t.Fatalf("inherited member resolved to %#v, want root declaration", ctx)
	}
}

func TestMetatableNavigationTracksAttachmentReplacementAndRemoval(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local first = { run = function() return "first" end }
local second = { run = function() return "second" end }
local object = {}
setmetatable(object, { __index = first })
object:--[[@first_meta_run]]run()
setmetatable(object, { __index = second })
object:--[[@second_meta_run]]run()
setmetatable(object, nil)
object:--[[@removed_meta_run]]run()
`)
	h.reindex()
	for _, marker := range []string{"first_meta_run", "second_meta_run"} {
		ctx := h.resolve(marker)
		if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
			t.Errorf("%s did not resolve through the active metatable: %#v", marker, ctx)
		}
	}
	if ctx := h.resolve("removed_meta_run"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("member resolved through a cleared metatable: %#v", ctx)
	}
}

func TestMetatableNavigationRespectsOrderAndShadowing(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() return true end }
local object = {}
object:--[[@before_attach]]run()
setmetatable(object, { __index = proto })
object:--[[@after_attach]]run()
do
  local object = {}
  object:--[[@shadow_before_attach]]run()
  setmetatable(object, { __index = proto })
  object:--[[@shadow_after_attach]]run()
end
object:--[[@outer_after_shadow]]run()
`)
	h.reindex()
	for _, marker := range []string{"before_attach", "shadow_before_attach"} {
		if ctx := h.resolve(marker); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
			t.Errorf("%s resolved through an attachment outside its active binding: %#v", marker, ctx)
		}
	}
	for _, marker := range []string{"after_attach", "shadow_after_attach", "outer_after_shadow"} {
		if ctx := h.resolve(marker); ctx == nil || ctx.TargetDefID == ast.InvalidNode {
			t.Errorf("%s did not resolve through its preceding attachment: %#v", marker, ctx)
		}
	}
}

func TestMetatableNavigationIgnoresShadowedSetmetatable(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local object = {}
local function setmetatable(value, meta) return value end
setmetatable(object, { __index = proto })
object:--[[@shadowed_builtin]]run()
`)
	h.reindex()
	if ctx := h.resolve("shadowed_builtin"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("local function was treated as builtin setmetatable: %#v", ctx)
	}
}

func TestMetatableNavigationDoesNotAssumeNestedFunctionExecuted(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local object = {}
local function initialize()
  setmetatable(object, { __index = proto })
end
object:--[[@not_initialized]]run()
`)
	h.reindex()
	if ctx := h.resolve("not_initialized"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("unexecuted function produced a metatable attachment: %#v", ctx)
	}
}

func TestMetatableNavigationNilFieldFallsBack(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { run = function() end }
local object = setmetatable({ run = nil }, { __index = base })
object:--[[@nil_member]]run()
`)
	h.reindex()
	ctx := h.resolve("nil_member")
	doc := h.docForMarker("nil_member")
	if ctx == nil || ctx.TargetDoc != doc || ctx.TargetDefID != findIdentByOccurrence(t, doc.Tree, "run", 0) {
		t.Fatalf("nil member resolved to %#v, want inherited definition", ctx)
	}
}

func TestMetatableNavigationIgnoresUnreachableAttachment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local object = {}
if false then
  setmetatable(object, { __index = { run = function() end } })
end
object:--[[@unreachable_attachment]]run()
`)
	h.reindex()
	if ctx := h.resolve("unreachable_attachment"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("unreachable attachment was applied: %#v", ctx)
	}
}

func TestMetatableNavigationDoesNotAssumeConditionalAttachment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local otherProto = { run = function() end }
local object = {}
setmetatable(object, { __index = proto })
if flag then
  setmetatable(object, { __index = otherProto })
elseif otherFlag then
  setmetatable(object, { __index = otherProto })
else
  setmetatable(object, { __index = otherProto })
end
object:--[[@conditional_attachment]]run()
`)
	h.reindex()
	if ctx := h.resolve("conditional_attachment"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("conditional attachment was presented as certain: %#v", ctx)
	}
}

func TestMetatableNavigationDoesNotAssumeLoopAttachment(t *testing.T) {
	tests := []struct {
		name string
		loop string
	}{
		{name: "while", loop: "while flag do"},
		{name: "numeric for", loop: "for i = 1, 3 do"},
		{name: "generic for", loop: "for _, value in pairs({}) do"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local object = {}
`+tt.loop+`
  setmetatable(object, { __index = proto })
end
object:--[[@loop_attachment]]run()
`)
			h.reindex()
			if ctx := h.resolve("loop_attachment"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
				t.Fatalf("possibly skipped loop attachment was presented as certain: %#v", ctx)
			}
		})
	}
}

func TestMetatableNavigationWithinConditionalAttachment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local object = {}
if flag then
  setmetatable(object, { __index = proto })
  object:--[[@inside_conditional_attachment]]run()
end
`)
	h.reindex()
	if ctx := h.resolve("inside_conditional_attachment"); ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("preceding attachment in the same branch was not resolved: %#v", ctx)
	}
}

func TestMetatableNavigationElseKeepsPriorAttachment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local other = { run = function() end }
local object = {}
setmetatable(object, { __index = proto })
if flag then
  setmetatable(object, { __index = other })
else
  object:--[[@else_prior_attachment]]run()
end
`)
	h.reindex()
	ctx := h.resolve("else_prior_attachment")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode || string(ctx.TargetDoc.nodeSource(ctx.TargetDefID)) != "run" {
		t.Fatalf("prior attachment was lost in alternate branch: %#v", ctx)
	}
}

func TestMetatableNavigationAfterTrueConditionalAttachment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local object = {}
if true then
  setmetatable(object, { __index = proto })
end
object:--[[@true_attachment]]run()
`)
	h.reindex()
	ctx := h.resolve("true_attachment")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("attachment in an always-executed branch was lost: %#v", ctx)
	}
}

func TestMetatableNavigationIgnoresUnreachableElseAttachment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local other = { stop = function() end }
local object = {}
if true then
  setmetatable(object, { __index = proto })
else
  setmetatable(object, { __index = other })
end
object:--[[@true_else_attachment]]run()
`)
	h.reindex()
	ctx := h.resolve("true_else_attachment")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("unreachable else displaced true branch attachment: %#v", ctx)
	}
}

func TestMetatableNavigationAfterFalseConditionalElse(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local proto = { run = function() end }
local object = {}
if false then
  setmetatable(object, { __index = {} })
else
  setmetatable(object, { __index = proto })
end
object:--[[@false_else_attachment]]run()
`)
	h.reindex()
	ctx := h.resolve("false_else_attachment")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("attachment in the guaranteed else branch was lost: %#v", ctx)
	}
}

func TestMetatableNavigationIndexChainSkipsNilIntermediateField(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { value = "inherited" }
local middle = setmetatable({ value = nil }, { __index = base })
local object = setmetatable({}, { __index = middle })
local result = object.val--[[@nil_intermediate]]ue
`)
	h.reindex()
	ctx := h.resolve("nil_intermediate")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("nil intermediate field did not resolve: %#v", ctx)
	}
	if line, _ := ctx.TargetDoc.Tree.Position(ctx.TargetDoc.Tree.Nodes[ctx.TargetDefID].Start); line != 0 {
		t.Fatalf("nil intermediate field resolved to line %d, want base declaration", line)
	}
}

func TestMetatableNavigationIndexChainWithSeparateAttachment(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { value = "inherited" }
local middle = {}
setmetatable(middle, { __index = base })
local object = setmetatable({}, { __index = middle })
local result = object.val--[[@separate_intermediate]]ue
`)
	h.reindex()
	ctx := h.resolve("separate_intermediate")
	if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
		t.Fatalf("separately attached intermediate field did not resolve: %#v", ctx)
	}
}

func TestMetatableNavigationIndexReplacementFollowsAccessOrder(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local first = { value = "before" }
local second = { value = 42 }
local mt = { __index = first }
local object = setmetatable({}, mt)
local a = object.val--[[@index_before]]ue
mt.__index = second
local b = object.val--[[@index_after]]ue
mt.__index = nil
local c = object.val--[[@index_clear]]ue
`)
	h.reindex()
	for marker, wantLine := range map[string]uint32{"index_before": 0, "index_after": 1} {
		ctx := h.resolve(marker)
		if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
			t.Fatalf("%s missing definition: %#v", marker, ctx)
		}
		line, _ := ctx.TargetDoc.Tree.Position(ctx.TargetDoc.Tree.Nodes[ctx.TargetDefID].Start)
		if line != wantLine {
			t.Errorf("%s resolved line %d, want %d", marker, line, wantLine)
		}
	}
	if ctx := h.resolve("index_clear"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("cleared __index still resolves: %#v", ctx)
	}
}

func TestMetatableNavigationIndexTableFieldReplacement(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { value = 42 }
local object = setmetatable({}, { __index = base })
local before = object.val--[[@table_field_before]]ue
base.value = "updated"
local after = object.val--[[@table_field_after]]ue
base.value = nil
local cleared = object.val--[[@table_field_cleared]]ue
`)
	h.reindex()
	for marker, wantLine := range map[string]uint32{"table_field_before": 0, "table_field_after": 3} {
		ctx := h.resolve(marker)
		if ctx == nil || ctx.TargetDefID == ast.InvalidNode {
			t.Fatalf("%s missing definition: %#v", marker, ctx)
		}
		line, _ := ctx.TargetDoc.Tree.Position(ctx.TargetDoc.Tree.Nodes[ctx.TargetDefID].Start)
		if line != wantLine {
			t.Errorf("%s definition line = %d, want %d", marker, line, wantLine)
		}
	}
	if ctx := h.resolve("table_field_cleared"); ctx != nil && ctx.TargetDefID != ast.InvalidNode {
		t.Fatalf("cleared field still resolves: %#v", ctx)
	}
}
