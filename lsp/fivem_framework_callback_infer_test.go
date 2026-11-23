package lsp

import (
	"path/filepath"
	"testing"
)

func TestFiveMFrameworkServerCallbackResponse(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("framework/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("framework/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('getApi', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("framework/client.lua", "API.TriggerServerCallback = function(name, callback, ...) end\n")
	h.writeWorkspaceFile("framework/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name](source, cb, ...) end\nRegisterNetEvent('relay', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("consumer/server.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', function(source, cb) cb({ answer = 42 }) end)\n")
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) local result = reply.--[[@response]]answer end)\n")
	h.reindex()
	doc := h.docForMarker("response")
	typ := doc.InferType(mustFindIdentNode(t, doc, "reply"))
	if typ.Basics&TypeTable == 0 || typ.DeclNode == 0 {
		t.Fatalf("response type = %#v, want returned table", typ)
	}
	if !completionHasLabel(h.completion("response"), "answer") {
		t.Fatal("missing returned table field")
	}
}

func TestFiveMFrameworkServerCallbackUsesDispatcherPosition(t *testing.T) {
	for _, tc := range []struct {
		name, handler string
		wantTable     bool
	}{
		{"extra_player", "function(player, source, cb) cb({ answer = 42 }) end", true},
		{"wrong_position", "function(source, cb, extra) cb({ answer = 42 }) end", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("framework/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
			h.writeWorkspaceFile("framework/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('getApi', function(cb) cb(API) end)\n")
			h.writeWorkspaceFile("framework/client.lua", "API.TriggerServerCallback = function(name, callback, ...) end\n")
			h.writeWorkspaceFile("framework/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name](player, source, cb, ...) end\nRegisterNetEvent('relay', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
			h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
			h.writeWorkspaceFile("consumer/server.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', "+tc.handler+")\n")
			h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end)\n")
			h.reindex()
			doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "client.lua"))]
			typ := doc.InferType(mustFindIdentNode(t, doc, "reply"))
			if got := typ.Basics&TypeTable != 0 && typ.DeclNode != 0; got != tc.wantTable {
				t.Fatalf("response type = %#v, want table = %v", typ, tc.wantTable)
			}
		})
	}
}

func TestFiveMFrameworkServerCallbackRequestInferenceAndEdit(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("framework/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("framework/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('getApi', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("framework/client.lua", "API.TriggerServerCallback = function(name, callback, ...) end\n")
	h.writeWorkspaceFile("framework/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name](source, cb, ...) end\nRegisterNetEvent('relay', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("consumer/server.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', function(source, cb, request) cb(request) end)\n")
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end, true)\n")
	h.reindex()
	server := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "server.lua"))]
	client := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "client.lua"))]
	request := mustFindIdentNode(t, server, "request")
	reply := mustFindIdentNode(t, client, "reply")
	if got := server.InferType(request); got.Basics != TypeBoolean {
		t.Fatalf("request type = %s, want boolean", got.Format())
	}
	if got := client.InferType(reply); got.Basics != TypeBoolean {
		t.Fatalf("reply type = %s, want boolean", got.Format())
	}
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end, 'changed')\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "consumer", "client.lua")))
	if got := server.InferType(request); got.Basics != TypeString {
		t.Fatalf("edited request type = %s, want string", got.Format())
	}
}

func TestFiveMFrameworkServerCallbackDispatcherAppearsLater(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("framework/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_scripts { 'server.lua', 'relay.lua' }\n")
	h.writeWorkspaceFile("framework/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('getApi', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("framework/client.lua", "---@param name string\n---@param callback fun(...: any)\nfunction API.TriggerServerCallback(name, callback, ...) end\n")
	h.writeWorkspaceFile("framework/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name](source, cb, ...) end\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("consumer/server.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', function(source, cb) cb(42) end)\n")
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "client.lua"))]
	reply := mustFindIdentNode(t, doc, "reply")
	if got := doc.InferType(reply); got.Basics == TypeNumber {
		t.Fatalf("unexpected type before dispatcher is known: %s", got.Format())
	}
	h.writeWorkspaceFile("framework/relay.lua", "RegisterNetEvent('relay', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "framework", "relay.lua")))
	if got := doc.InferType(reply); got.Basics != TypeNumber {
		t.Fatalf("reply after dispatcher appeared = %s, want number", got.Format())
	}
}

func TestFiveMFrameworkServerCallbackNamedResponse(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("framework/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("framework/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('getApi', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("framework/client.lua", "API.TriggerServerCallback = function(name, callback, ...) end\n")
	h.writeWorkspaceFile("framework/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name](source, cb, ...) end\nRegisterNetEvent('relay', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'callback.lua', 'client.lua' }\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("consumer/server.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', function(source, cb) cb(42) end)\n")
	h.writeWorkspaceFile("consumer/callback.lua", "function handleAnswer(reply) print(reply) end\n")
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', handleAnswer)\n")
	h.reindex()
	callback := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "callback.lua"))]
	if got := callback.InferType(mustFindIdentNode(t, callback, "reply")); got.Basics != TypeNumber {
		t.Fatalf("named response = %s, want number", got.Format())
	}
}

func TestFiveMFrameworkServerCallbackRequestTypeDiagnostic(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMEventPayload = true
	h.writeWorkspaceFile("framework/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("framework/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('getApi', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("framework/client.lua", "API.TriggerServerCallback = function(name, callback, ...) end\n")
	h.writeWorkspaceFile("framework/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name](source, cb, ...) end\nRegisterNetEvent('relay', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("consumer/server.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\n---@param source number\n---@param cb function\n---@param request number\nAPI.RegisterServerCallback('answer', function(source, cb, request) cb(request) end)\n")
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end, 'bad')\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end, 42)\n")
	h.reindex()
	var mismatches int
	for _, diagnostic := range h.diagnostics("consumer/client.lua") {
		if diagnostic.Code == "fivem-event-argument-type" {
			mismatches++
		}
	}
	if mismatches != 1 {
		t.Fatalf("expected one callback request mismatch, got %d", mismatches)
	}
}

func TestFiveMFrameworkServerCallbackDoesNotMixProviders(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	for _, provider := range []string{"first", "second"} {
		h.writeWorkspaceFile(provider+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
		h.writeWorkspaceFile(provider+"/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('get:"+provider+"', function(cb) cb(API) end)\n")
		h.writeWorkspaceFile(provider+"/client.lua", "API.TriggerServerCallback = function(name, callback, ...) end\n")
		h.writeWorkspaceFile(provider+"/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name](source, cb, ...) end\nRegisterNetEvent('relay:"+provider+"', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
	}
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("consumer/server.lua", "local API\nTriggerEvent('get:second', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', function(source, cb) cb(42) end)\n")
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('get:first', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "client.lua"))]
	if got := doc.InferType(mustFindIdentNode(t, doc, "reply")); got.Basics == TypeNumber {
		t.Fatalf("unrelated provider leaked response type: %s", got.Format())
	}
}

func TestFiveMFrameworkServerCallbackMultipleReceiversAndInvalidation(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("framework/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("framework/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('getApi', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("framework/client.lua", "API.TriggerServerCallback = function(name, callback, ...) end\n")
	h.writeWorkspaceFile("framework/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name](source, cb, ...) end\nRegisterNetEvent('relay', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_scripts { 'first.lua', 'second.lua' }\n")
	h.writeWorkspaceFile("consumer/first.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', function(source, cb) cb(42) end)\n")
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "client.lua"))]
	reply := mustFindIdentNode(t, doc, "reply")
	if got := doc.InferType(reply); got.Basics != TypeNumber {
		t.Fatalf("initial response = %s, want number", got.Format())
	}
	h.writeWorkspaceFile("consumer/second.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', function(source, cb) cb('text') end)\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "consumer", "second.lua")))
	if got := doc.InferType(reply); got.Basics != TypeNumber|TypeString {
		t.Fatalf("responses after new receiver = %s, want number | string", got.Format())
	}
}

func TestFiveMFrameworkCallbackUsesActualDispatcherArgumentType(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("framework/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("framework/shared.lua", "API = {}\nAPI.ServerCallbacks = {}\nAddEventHandler('getApi', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("framework/client.lua", "API.TriggerServerCallback = function(name, callback, ...) end\n")
	h.writeWorkspaceFile("framework/server.lua", "API.RegisterServerCallback = function(name, callback) API.ServerCallbacks[name] = callback end\nAPI.TriggerServerCallback = function(name, requestId, source, cb, ...) API.ServerCallbacks[name]('actual-string', cb, ...) end\nRegisterNetEvent('relay', function(name, requestId) API.TriggerServerCallback(name, requestId, source, function(...) end) end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("consumer/server.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.RegisterServerCallback('answer', function(source, cb) print(source) cb(42) end)\n")
	h.writeWorkspaceFile("consumer/client.lua", "local API\nTriggerEvent('getApi', function(obj) API = obj end)\nAPI.TriggerServerCallback('answer', function(reply) print(reply) end)\n")
	h.reindex()
	server := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "server.lua"))]
	if got := server.InferType(mustFindIdentNode(t, server, "source")); got.Basics != TypeString {
		t.Fatalf("dispatcher argument = %s, want string", got.Format())
	}
}

func TestFiveMFrameworkCallbacksAreNotRawEventContracts(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "res", "main.lua"), "API.RegisterServerCallback('callback:name', function(source, cb) end)\n")
	if len(doc.FiveMEvents) != 1 || doc.FiveMEvents[0].Kind != FiveMFrameworkRegisterServerCallback {
		t.Fatalf("callback was not indexed: %#v", doc.FiveMEvents)
	}
	if got := fiveMEventContractSymbols(doc); len(got) != 0 {
		t.Fatalf("callback misclassified as raw event contract: %#v", got)
	}
	node := doc.Tree.Nodes[doc.FiveMEvents[0].NodeID]
	if _, ok := doc.fiveMEventAtOffset(node.Start + 1); ok {
		t.Fatal("callback was misclassified as a raw event at its source location")
	}
}
