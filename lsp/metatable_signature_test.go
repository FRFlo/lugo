package lsp

import (
	"strings"
	"testing"
)

func TestMetatableCallSignatureHelp(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local callable = setmetatable({}, { __call = function(self, name, count) return true end })
local result = callable(--[[@call_signature]])
`)
	h.reindex()
	help := h.signatureHelp("call_signature")
	if help == nil || len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "name") || !strings.Contains(help.Signatures[0].Label, "count") {
		t.Fatalf("call signature = %#v, want callable parameters", help)
	}
}

func TestMetatableNamedCallSignatureHelp(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local function invoke(receiver, name, count) return true end
local callable = setmetatable({}, { __call = invoke })
local result = callable(--[[@named_call_signature]])
`)
	h.reindex()
	help := h.signatureHelp("named_call_signature")
	if help == nil || len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "name") || !strings.Contains(help.Signatures[0].Label, "count") || strings.Contains(help.Signatures[0].Label, "receiver") {
		t.Fatalf("call signature = %#v, want visible callable parameters", help)
	}
}

func TestMetatableAssignedCallSignatureHelp(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local meta = {}
meta.__call = function(receiver, name) return true end
local callable = setmetatable({}, meta)
local result = callable(--[[@assigned_call_signature]])
`)
	h.reindex()
	help := h.signatureHelp("assigned_call_signature")
	if help == nil || len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "name") || strings.Contains(help.Signatures[0].Label, "receiver") {
		t.Fatalf("assigned call signature = %#v, want visible argument", help)
	}
}

func TestMetatableCallSignatureTracksReplacement(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local meta = { __call = function(self, before) end }
local callable = setmetatable({}, meta)
local one = callable(--[[@before_call_replacement]])
meta.__call = function(self, after) end
local two = callable(--[[@after_call_replacement]])
meta.__call = function(self, final) end
local three = callable(--[[@final_call_replacement]])
`)
	h.reindex()
	before := h.signatureHelp("before_call_replacement")
	after := h.signatureHelp("after_call_replacement")
	if before == nil || len(before.Signatures) == 0 || !strings.Contains(before.Signatures[0].Label, "before") {
		t.Fatalf("before signature = %#v", before)
	}
	if after == nil || len(after.Signatures) == 0 || !strings.Contains(after.Signatures[0].Label, "after") || strings.Contains(after.Signatures[0].Label, "before") {
		t.Fatalf("after signature = %#v", after)
	}
	final := h.signatureHelp("final_call_replacement")
	if final == nil || len(final.Signatures) == 0 || !strings.Contains(final.Signatures[0].Label, "final") {
		t.Fatalf("final signature = %#v", final)
	}
}
