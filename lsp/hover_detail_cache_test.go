package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHoverDetailUsesCurrentClassFields(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	writeTypes := func(fieldType string) {
		h.writeWorkspaceFile("types.lua", "---@class Leaf\n---@field value "+fieldType+"\nlocal Leaf = {}\n---@class Pair\n---@field first Leaf\n---@field second Leaf\nlocal Pair = {}\n")
	}
	writeTypes("number")
	h.writeWorkspaceFile("use.lua", "---@type Pair\nlocal pair = {}\nlocal x = pa--[[@hover]]ir\n")
	h.reindex()
	hover := func() Hover {
		t.Helper()
		result := h.positionResponse("hover", func(req Request) {
			var params map[string]any
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatal(err)
			}
			params["lugoTableDepth"] = -1
			req.Params, _ = json.Marshal(params)
			h.server.handleHover(req)
		})
		var got Hover
		if err := json.Unmarshal(result, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	before := hover().Contents.Value
	if strings.Count(before, "value: number") != 2 {
		t.Fatalf("initial hover = %s", before)
	}
	writeTypes("string")
	h.reindex()
	after := hover().Contents.Value
	if strings.Count(after, "value: string") != 2 || strings.Contains(after, "value: number") {
		t.Fatalf("updated hover = %s", after)
	}
}
