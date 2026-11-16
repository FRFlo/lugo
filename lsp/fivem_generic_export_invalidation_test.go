package lsp

import (
	"testing"
)

func TestFiveMGenericExportInferenceAndProviderReindex(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script { 'provider.lua', 'exports.lua' }\n")
	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return T
function identity(value) return value end
`)
	h.writeWorkspaceFile("provider/exports.lua", `exports('identity', identity)
`)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nclient_script 'consumer.lua'\n")
	h.writeWorkspaceFile("consumer/consumer.lua", `local result = exports.provider:identity(42)
`)
	h.reindex()

	consumerURI := h.server.pathToURI(h.root + "/consumer/consumer.lua")
	consumer := h.server.Documents[consumerURI]
	resultID := findIdentByOccurrence(t, consumer.Tree, "result", 0)
	if got := consumer.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("initial generic export result = %#v, want number", got)
	}

	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return string
function identity(value) return tostring(value) end
`)
	h.reindex()
	consumer = h.server.Documents[consumerURI]
	if got := consumer.InferType(resultID); got.Basics != TypeString {
		t.Fatalf("generic export result after provider reindex = %#v, want string", got)
	}
}

func TestFiveMGenericExportInferenceRefreshesAfterProviderReindex(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'provider.lua'\n")
	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return T
exports('identity', function(value) return value end)
`)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nclient_script 'consumer.lua'\n")
	h.writeWorkspaceFile("consumer/consumer.lua", `local result = exports.provider:identity(42)
`)
	h.reindex()

	consumerURI := h.server.pathToURI(h.root + "/consumer/consumer.lua")
	consumer := h.server.Documents[consumerURI]
	resultID := findIdentByOccurrence(t, consumer.Tree, "result", 0)
	if got := consumer.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("initial generic export result = %#v, want number", got)
	}

	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return string
exports('identity', function(value) return tostring(value) end)
`)
	h.reindex()
	consumer = h.server.Documents[consumerURI]
	if got := consumer.InferType(resultID); got.Basics != TypeString {
		t.Fatalf("generic export result after provider reindex = %#v, want string", got)
	}
}
