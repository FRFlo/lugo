package lsp

import (
	"fmt"
	"strings"
	"testing"
)

func TestFiveMGenericExportFollowsTransitiveLocalAliasesAndInvalidates(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script { 'provider.lua', 'aliases.lua' }\n")
	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return T
function identity(value) return value end
`)
	h.writeWorkspaceFile("provider/aliases.lua", `local alias1 = identity
local alias2 = alias1
exports('alias', alias2)
`)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nclient_script 'consumer.lua'\n")
	h.writeWorkspaceFile("consumer/consumer.lua", `local result = exports.provider:alias(42)
`)
	h.reindex()

	consumerURI := h.server.pathToURI(h.root + "/consumer/consumer.lua")
	consumer := h.server.Documents[consumerURI]
	resultID := findIdentByOccurrence(t, consumer.Tree, "result", 0)
	if got := consumer.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("transitively aliased generic export result = %#v, want number", got)
	}

	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return string
function identity(value) return tostring(value) end
`)
	h.reindex()
	consumer = h.server.Documents[consumerURI]
	resultID = findIdentByOccurrence(t, consumer.Tree, "result", 0)
	if got := consumer.InferType(resultID); got.Basics != TypeString {
		t.Fatalf("transitively aliased generic export after provider reindex = %#v, want string", got)
	}
}

func TestFiveMGenericExportAliasCycleTerminates(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script { 'provider.lua', 'aliases.lua' }\n")
	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return T
function identity(value) return value end
`)
	h.writeWorkspaceFile("provider/aliases.lua", `local first = second
local second = first
exports('cycle', first)
`)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nclient_script 'consumer.lua'\n")
	h.writeWorkspaceFile("consumer/consumer.lua", `local result = exports.provider:cycle(42)
`)
	h.reindex()
	consumerURI := h.server.pathToURI(h.root + "/consumer/consumer.lua")
	consumer := h.server.Documents[consumerURI]
	resultID := findIdentByOccurrence(t, consumer.Tree, "result", 0)
	got := consumer.InferType(resultID)
	if got.Basics == TypeNumber || got.Basics == TypeString {
		t.Fatalf("cyclic export alias unexpectedly resolved as a concrete generic: %#v", got)
	}
}

func TestFiveMGenericExportFollowsAliasChainLongerThanStackBuffer(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script { 'provider.lua', 'aliases.lua' }\n")
	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return T
function identity(value) return value end
`)
	var aliases strings.Builder
	for i := 1; i <= 80; i++ {
		value := "identity"
		if i > 1 {
			value = fmt.Sprintf("alias%d", i-1)
		}
		fmt.Fprintf(&aliases, "local alias%d = %s\n", i, value)
	}
	aliases.WriteString("exports('long', alias80)\n")
	h.writeWorkspaceFile("provider/aliases.lua", aliases.String())
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nclient_script 'consumer.lua'\n")
	h.writeWorkspaceFile("consumer/consumer.lua", `local result = exports.provider:long(42)
`)
	h.reindex()
	consumerURI := h.server.pathToURI(h.root + "/consumer/consumer.lua")
	consumer := h.server.Documents[consumerURI]
	resultID := findIdentByOccurrence(t, consumer.Tree, "result", 0)
	if got := consumer.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("generic export behind 80 local aliases = %#v, want number", got)
	}
}

func TestFiveMGenericExportInferenceSurvivesClosedProviderEviction(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'provider.lua'\n")
	h.writeWorkspaceFile("provider/provider.lua", `---@generic T
---@param value T
---@return T
function identity(value) return value end
exports('identity', identity)
`)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nclient_script 'consumer.lua'\n")
	h.writeWorkspaceFile("consumer/consumer.lua", `local result = exports.provider:identity(42)
`)
	h.reindex()
	providerURI := h.server.pathToURI(h.root + "/provider/provider.lua")
	consumerURI := h.server.pathToURI(h.root + "/consumer/consumer.lua")
	consumer := h.server.Documents[consumerURI]
	resultID := findIdentByOccurrence(t, consumer.Tree, "result", 0)
	if got := consumer.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("generic export before provider eviction = %#v, want number", got)
	}

	h.server.OpenFiles[consumerURI] = true
	delete(h.server.OpenFiles, providerURI)
	evictClosedDocumentCaches(h.server)
	consumer.TypeCache.Drop()
	if got := consumer.InferType(resultID); got.Basics != TypeNumber {
		t.Fatalf("generic export after closed provider eviction = %#v, want number", got)
	}
}
