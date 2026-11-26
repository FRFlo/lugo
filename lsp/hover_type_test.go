package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompositeTypeHover(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Item
---@field title string
---@field nested Nested
local Item = {}
---@class Nested
---@field enabled boolean
local Nested = {}
---@alias Items Item[]
local Items
---@class Recursive<T>
---@field next Recursive<Recursive<T>>
local Recursive = {}
`)
	h.writeWorkspaceFile("use.lua", `---@param input Items
---@return table<string, Item>
local function process(input) return {} end
local callback = pro--[[@function]]cess
---@type fun(value: Item[]): Item | nil
local f
local other = --[[@callable]]f
---@type { item: Item, values: Item[] }
local config
local c = con--[[@inline]]fig
local object = { build = function() return { inner = { ok = true } } end }
local b = obj--[[@inferred]]ect
---@type Recursive<string>
local recursive
local r = rec--[[@recursive]]ursive
local function make() return { inner = { ok = true } } end
local m = ma--[[@inferred_function]]ke
---@type (Item | nil)[]
local wrapped
local w = wr--[[@wrapped]]apped
---@return Item
---@return string
local function multi() return {}, "" end
local k = mul--[[@multi]]ti
local function loop() return loop end
local l = lo--[[@recursive_function]]op
`)
	h.reindex()
	request := func(marker string, depth int) Hover {
		t.Helper()
		result := h.positionResponse(marker, func(req Request) {
			var params map[string]any
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatal(err)
			}
			params["lugoTableDepth"] = depth
			req.Params, _ = json.Marshal(params)
			h.server.handleHover(req)
		})
		var hover Hover
		if err := json.Unmarshal(result, &hover); err != nil {
			t.Fatal(err)
		}
		return hover
	}
	for _, marker := range []string{"function", "callable", "inline", "wrapped"} {
		t.Run(marker, func(t *testing.T) {
			summary := request(marker, 0)
			if summary.LugoTable == nil || !summary.LugoTable.More {
				t.Fatalf("summary = %#v", summary)
			}
			full := request(marker, -1)
			for _, want := range []string{"title: string", "enabled: boolean"} {
				if !strings.Contains(full.Contents.Value, want) {
					t.Errorf("expanded %s missing %q: %s", marker, want, full.Contents.Value)
				}
			}
			if full.LugoTable == nil || full.LugoTable.More {
				t.Errorf("full type should not have more levels: %#v", full.LugoTable)
			}
		})
	}
	full := request("inferred", -1)
	if !strings.Contains(full.Contents.Value, "ok: boolean") {
		t.Fatalf("inferred function return = %s", full.Contents.Value)
	}
	full = request("inferred_function", -1)
	if !strings.Contains(full.Contents.Value, "ok: boolean") {
		t.Fatalf("inferred function hover = %s", full.Contents.Value)
	}
	full = request("function", -1)
	if !strings.Contains(full.Contents.Value, "Items =") {
		t.Fatalf("function parameter alias was not expanded: %s", full.Contents.Value)
	}
	full = request("multi", -1)
	if !strings.Contains(full.Contents.Value, "title: string") || !strings.Contains(full.Contents.Value, "}, string") {
		t.Fatalf("multiple function returns were not expanded: %s", full.Contents.Value)
	}
	full = request("recursive", -1)
	if full.LugoTable == nil || full.LugoTable.More || !strings.Contains(full.Contents.Value, "next:") {
		t.Fatalf("recursive generic should terminate: %#v", full)
	}
	full = request("recursive_function", -1)
	if full.LugoTable == nil || full.LugoTable.More {
		t.Fatalf("recursive function should terminate: %#v", full)
	}
	finished := false
	for depth := 1; depth < 12; depth++ {
		hover := request("function", depth)
		if hover.LugoTable != nil && !hover.LugoTable.More {
			if !strings.Contains(hover.Contents.Value, "enabled: boolean") {
				t.Fatalf("depth %d stopped too early: %s", depth, hover.Contents.Value)
			}
			finished = true
			break
		}
	}
	if !finished {
		t.Fatal("progressive expansion did not reach the deepest type")
	}
	doc := h.docForMarker("function")
	union := TypeSet{Alternatives: []TypeSet{{CustomName: "Item"}, {Basics: TypeNil}}}
	expanded, more := h.server.formatHoverType(doc, union, "", -1, 0, &hoverFormatState{active: make(map[hoverTableKey]bool)})
	if more || !strings.Contains(expanded, "title: string") || !strings.Contains(expanded, "nil") {
		t.Fatalf("inferred union = %q, more=%v", expanded, more)
	}
}
