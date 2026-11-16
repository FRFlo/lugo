package lsp

import (
	"strings"
	"testing"
)

func TestFiveMGenericExportEvictionVariantsAndReindex(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider string
		bridge   string
		manifest string
	}{
		{"inline", "---@generic T\n---@param value T\n---@return T\nexports('identity', function(value) return value end)\n", "", ""},
		{"manifest", "---@generic T\n---@param value T\n---@return T\nfunction identity(value) return value end\n", "", "client_export 'identity'\n"},
		{"cross_document_alias", "---@generic T\n---@param value T\n---@return T\nfunction identity(value) return value end\n", "local first = identity\nlocal second = first\nexports('identity', second)\n", ""},
		{"parameter_order", "---@generic T\n---@param value T\n---@param ignored string\n---@return T\nfunction identity(ignored, value) return value end\n", "exports('identity', identity)\n", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script { 'provider.lua', 'bridge.lua' }\n"+test.manifest)
			h.writeWorkspaceFile("provider/provider.lua", test.provider)
			h.writeWorkspaceFile("provider/bridge.lua", test.bridge)
			h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nclient_script 'consumer.lua'\n")
			call := "local result = exports.provider:identity(42)\n"
			if test.name == "parameter_order" {
				call = "local result = exports.provider:identity('ignored', 42)\n"
			}
			h.writeWorkspaceFile("consumer/consumer.lua", call)
			h.reindex()
			consumerURI := h.server.pathToURI(h.root + "/consumer/consumer.lua")
			h.server.OpenFiles[consumerURI] = true
			checkType := func(want BasicType) {
				t.Helper()
				consumer := h.server.Documents[consumerURI]
				consumer.TypeCache.Drop()
				result := findIdentByOccurrence(t, consumer.Tree, "result", 0)
				if got := consumer.InferType(result); got.Basics != want {
					t.Fatalf("generic export type = %#v, want %v", got, want)
				}
			}
			checkType(TypeNumber)
			for range 2 {
				evictClosedDocumentCaches(h.server)
				checkType(TypeNumber)
			}
			// Reindex must replace the retained contract, even when the old
			// document has already lost its source and the consumer is open.
			h.writeWorkspaceFile("provider/provider.lua", strings.Replace(test.provider, "---@return T", "---@return string", 1))
			h.reindex()
			checkType(TypeString)
			evictClosedDocumentCaches(h.server)
			checkType(TypeString)
		})
	}
}

func TestFiveMGenericExportAliasCycleAfterEvictionTerminates(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'provider.lua'\n")
	h.writeWorkspaceFile("provider/provider.lua", "first = second\nsecond = first\nexports('cycle', first)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nclient_script 'consumer.lua'\n")
	h.writeWorkspaceFile("consumer/consumer.lua", "local result = exports.provider:cycle(42)\n")
	h.reindex()
	consumerURI := h.server.pathToURI(h.root + "/consumer/consumer.lua")
	h.server.OpenFiles[consumerURI] = true
	for range 2 {
		evictClosedDocumentCaches(h.server)
		consumer := h.server.Documents[consumerURI]
		consumer.TypeCache.Drop()
		result := findIdentByOccurrence(t, consumer.Tree, "result", 0)
		if got := consumer.InferType(result); got.Basics == TypeNumber || got.Basics == TypeString {
			t.Fatalf("cyclic alias resolved after source eviction: %#v", got)
		}
	}
}
