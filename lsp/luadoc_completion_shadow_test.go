package lsp

import (
	"testing"
)

func TestLuaDocClassCompletionChildFieldShadowsParentRegardlessOfNameOrder(t *testing.T) {
	tests := []struct {
		name   string
		parent string
		child  string
	}{
		{name: "parent sorts first", parent: "ABase", child: "ZChild"},
		{name: "child sorts first", parent: "ZBase", child: "AChild"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("types.lua", "---@class "+tc.parent+"\n---@deprecated use child value\n---@field value string parent description\nlocal "+tc.parent+" = {}\n\n---@class "+tc.child+": "+tc.parent+"\n---@field value number child description\nlocal "+tc.child+" = {}\n")
			h.writeWorkspaceFile("use.lua", "---@type "+tc.child+"\nlocal value = {}\nlocal result = value.--[[@completion]]\n")
			h.reindex()

			item := completionItemByLabel(h.completion("completion"), "value")
			if item == nil {
				t.Fatal("completion item value missing")
			}
			if item.Detail != "field" {
				t.Errorf("value detail = %q, want child field detail", item.Detail)
			}
			if len(item.Tags) != 0 {
				t.Errorf("value tags = %#v, want non-deprecated child field", item.Tags)
			}
		})
	}
}
