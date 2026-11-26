package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTableHoverDepth(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("tables.lua", `local config = {
  title = "hello",
  nes--[[@field_definition]]ted = { count = 3, deeper = { enabled = true } },
  ["with space"] = 42,
  [""] = true,
  ["end"] = false,
  list = { { value = 7 }, [4] = false },
}

config.extra = "added"
config.self = config
config.nested.later = 12
local a = con--[[@table]]fig
local b = config.nes--[[@field]]ted
`)
	h.reindex()
	request := func(marker string, depth int) *Hover {
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
		return &hover
	}
	for _, tc := range []struct {
		name     string
		depth    int
		contains []string
		absent   []string
		more     bool
	}{
		{"summary", 0, nil, []string{"Type details"}, true},
		{"first level", 1, []string{"title: string", "nested: table", `["with space"]: number`, `[""]: boolean`, `["end"]: boolean`, "extra: string", "self: table"}, []string{"count: number", "later: number"}, true},
		{"second level", 2, []string{"count: number", "deeper: table", "[1]: table", "[4]: boolean", "later: number"}, []string{"enabled: boolean", "value: number"}, true},
		{"all levels", 3, []string{"enabled: boolean", "value: number"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hover := request("table", tc.depth)
			if hover.LugoTable == nil || hover.LugoTable.Depth != tc.depth || hover.LugoTable.More != tc.more {
				t.Fatalf("table info = %#v; hover = %s", hover.LugoTable, hover.Contents.Value)
			}
			for _, want := range tc.contains {
				if !strings.Contains(hover.Contents.Value, want) {
					t.Errorf("hover missing %q: %s", want, hover.Contents.Value)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(hover.Contents.Value, unwanted) {
					t.Errorf("hover unexpectedly contains %q: %s", unwanted, hover.Contents.Value)
				}
			}
		})
	}
	field := request("field", 2)
	if !strings.Contains(field.Contents.Value, "enabled: boolean") {
		t.Fatalf("nested field hover = %s", field.Contents.Value)
	}
	field = request("field_definition", 2)
	if !strings.Contains(field.Contents.Value, "enabled: boolean") {
		t.Fatalf("field definition hover = %s", field.Contents.Value)
	}
}

func TestTableHoverLuaDocClass(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Parent
---@field inherited string
local Parent = {}
---@class Child: Parent
---@field nested Nested
---@field self Child
local Child = {}
---@class Nested
---@field enabled boolean
local Nested = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Child
local value = {}
local x = val--[[@class_hover]]ue
`)
	h.reindex()
	hover := h.hover("class_hover")
	if hover == nil || hover.LugoTable == nil || !hover.LugoTable.More {
		t.Fatalf("class hover = %#v", hover)
	}
	for _, want := range []string{"inherited: string", "nested: Nested", "self: Child"} {
		if !strings.Contains(hover.Contents.Value, want) {
			t.Errorf("class hover missing %q: %s", want, hover.Contents.Value)
		}
	}
	result := h.positionResponse("class_hover", func(req Request) {
		var params map[string]any
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Fatal(err)
		}
		params["lugoTableDepth"] = 2
		req.Params, _ = json.Marshal(params)
		h.server.handleHover(req)
	})
	if err := json.Unmarshal(result, &hover); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hover.Contents.Value, "enabled: boolean") || hover.LugoTable == nil || hover.LugoTable.More {
		t.Fatalf("expanded class hover = %#v", hover)
	}
}
