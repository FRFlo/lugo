package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFiveMEventCallbackInference(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "client.lua"), "AddEventHandler('example', function(cb) cb(42) end)")
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "server.lua"), "AddEventHandler('example', function(cb) cb('server') end)")
	addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "client.lua"), "TriggerEvent('example', function(value) print(value) end)")
	id := mustFindIdentNode(t, doc, "value")
	if got := doc.InferType(id); got.Basics != TypeNumber {
		t.Fatalf("callback type = %s, want number", got.Format())
	}
}

func TestFiveMEventCallbackInferenceUnionAndInvalidation(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	provider := filepath.Join(root, "provider", "client.lua")
	addFiveMTestDocument(t, s, provider, "AddEventHandler('example', function(cb) cb(42) end)\nAddEventHandler('example', function(cb) cb('text') end)")
	addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "client.lua"), "TriggerEvent('example', function(value) print(value) end)")
	id := mustFindIdentNode(t, doc, "value")
	if got := doc.InferType(id); got.Basics != TypeNumber|TypeString {
		t.Fatalf("callback type = %s, want number | string", got.Format())
	}
	addFiveMTestDocument(t, s, provider, "AddEventHandler('example', function(cb) cb(true) end)")
	if got := doc.InferType(id); got.Basics != TypeBoolean {
		t.Fatalf("callback type after provider edit = %s, want boolean", got.Format())
	}
}

func TestFiveMEventCallbackTableIdentity(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	provider := addFiveMTestDocument(t, s, filepath.Join(root, "provider", "client.lua"), "local object = { greet = function() end }\nAddEventHandler('get', function(cb) cb(object) end)")
	addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "client.lua"), "local value\nTriggerEvent('get', function(obj) value = obj end)\nprint(value)")
	id := mustFindIdentNode(t, doc, "value")
	got := doc.InferType(id)
	if got.DeclURI != provider.URI || got.DeclNode == 0 || got.Basics&TypeTable == 0 {
		t.Fatalf("assigned value lost provider identity: %#v", got)
	}
}

func TestFiveMEventNetworkPayloadInference(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	client := addFiveMTestDocument(t, s, filepath.Join(root, "res", "client.lua"), "TriggerServerEvent('toServer', 'hello')\nRegisterNetEvent('toClient', function(value) print(value) end)")
	server := addFiveMTestDocument(t, s, filepath.Join(root, "res", "server.lua"), "RegisterNetEvent('toServer', function(payload) print(payload) end)\nTriggerClientEvent('toClient', 4, true)")
	for _, tc := range []struct {
		doc   *Document
		ident string
		want  BasicType
	}{{server, "payload", TypeString}, {client, "value", TypeBoolean}} {
		got := tc.doc.InferType(mustFindIdentNode(t, tc.doc, tc.ident))
		if got.Basics != tc.want {
			t.Errorf("%s = %s, want %v", tc.ident, got.Format(), tc.want)
		}
	}
}

func TestFiveMEventCallbackNewProviderInvalidation(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'one.lua'\nclient_script 'two.lua'\n")
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "one.lua"), "AddEventHandler('example', function(cb) cb(42) end)")
	addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "client.lua"), "TriggerEvent('example', function(value) print(value) end)")
	id := mustFindIdentNode(t, doc, "value")
	if got := doc.InferType(id); got.Basics != TypeNumber {
		t.Fatalf("initial type = %s", got.Format())
	}
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "two.lua"), "AddEventHandler('example', function(cb) cb('text') end)")
	if got := doc.InferType(id); got.Basics != TypeNumber|TypeString {
		t.Fatalf("type after new provider = %s, want number | string", got.Format())
	}
}

func TestFiveMEventCallbackCompletion(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'shared.lua', 'methods.lua' }\n")
	h.writeWorkspaceFile("provider/shared.lua", "ESX = {}\nAddEventHandler('esx:getSharedObject', function(cb) cb(ESX) end)\n")
	h.writeWorkspaceFile("provider/methods.lua", "function ESX.Hello() end\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("consumer/client.lua", "local ESX\nTriggerEvent('esx:getSharedObject', function(obj) ESX = obj end)\nlocal result = ESX.--[[@completion]]Hello\n")
	h.reindex()
	if !completionHasLabel(h.completion("completion"), "Hello") {
		t.Fatal("missing ESX.Hello from callback completion")
	}
}

func TestFiveMEventNetworkSplitRegistration(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "client.lua"), "TriggerServerEvent('message', 42)")
	server := addFiveMTestDocument(t, s, filepath.Join(root, "res", "server.lua"), "RegisterNetEvent('message')\nAddEventHandler('message', function(payload) print(payload) end)")
	if got := server.InferType(mustFindIdentNode(t, server, "payload")); got.Basics != TypeNumber {
		t.Fatalf("payload = %s, want number", got.Format())
	}
}

func TestFiveMEventCallbackCrossResourceMemberTypeDiagnostic(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'shared.lua', 'fields.lua' }\n")
	h.writeWorkspaceFile("provider/shared.lua", "ESX = {}\nAddEventHandler('get', function(cb) cb(ESX) end)\n")
	h.writeWorkspaceFile("provider/fields.lua", "ESX.Counter = 42\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("consumer/client.lua", "local ESX\nTriggerEvent('get', function(obj) ESX = obj end)\nlocal result = ESX.Counter()\n")
	h.reindex()
	if got := h.diagnostics("consumer/client.lua"); !hasDiagnosticCode(got, "call-non-function") {
		t.Fatalf("expected non-function diagnostic, got %#v", got)
	}
}

func TestFiveMEventHandlerMissingPayloadIsNil(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "res", "main.lua"), "TriggerEvent('event', 1)\nTriggerEvent('event')\nAddEventHandler('event', function(value) print(value) end)")
	if got := doc.InferType(mustFindIdentNode(t, doc, "value")); got.Basics != TypeNumber|TypeNil {
		t.Fatalf("payload = %s, want number | nil", got.Format())
	}
}

func TestFiveMEventNamedHandler(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "res", "main.lua"), "local function handler(cb) cb(42) end\nAddEventHandler('event', handler)\nTriggerEvent('event', function(value) print(value) end)")
	if got := doc.InferType(mustFindIdentNode(t, doc, "value")); got.Basics != TypeNumber {
		t.Fatalf("callback value = %s, want number", got.Format())
	}
}

func TestFiveMEventNamedCallback(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "res", "main.lua"), "AddEventHandler('event', function(cb) cb(42) end)\nlocal function callback(value) print(value) end\nTriggerEvent('event', callback)")
	if got := doc.InferType(mustFindIdentNode(t, doc, "value")); got.Basics != TypeNumber {
		t.Fatalf("callback value = %s, want number", got.Format())
	}
}

func TestFiveMEventCallbackSharedGlobalRuntimeFields(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nshared_script 'shared.lua'\nclient_scripts { 'client.lua', 'handler.lua' }\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("provider/shared.lua", "ESX = {}\n")
	h.writeWorkspaceFile("provider/client.lua", "ESX = nil\nfunction ESX.ClientMethod() end\n")
	h.writeWorkspaceFile("provider/server.lua", "function ESX.ServerMethod() end\n")
	h.writeWorkspaceFile("provider/handler.lua", "AddEventHandler('get', function(cb) cb(ESX) end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("consumer/client.lua", "ESX = nil\nTriggerEvent('get', function(obj) ESX = obj end)\nlocal result = ESX.--[[@shared_esx]]ClientMethod\n")
	h.reindex()
	items := h.completion("shared_esx")
	if !completionHasLabel(items, "ClientMethod") || completionHasLabel(items, "ServerMethod") {
		t.Fatalf("wrong runtime fields: %#v", items.Items)
	}
}

func TestFiveMEventCallbackProviderRemovalInvalidates(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_scripts { 'one.lua', 'two.lua' }\n")
	first := addFiveMTestDocument(t, s, filepath.Join(root, "provider", "one.lua"), "AddEventHandler('example', function(cb) cb(42) end)")
	addFiveMTestDocument(t, s, filepath.Join(root, "provider", "two.lua"), "AddEventHandler('example', function(cb) cb('text') end)")
	addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "consumer", "client.lua"), "TriggerEvent('example', function(value) print(value) end)")
	id := mustFindIdentNode(t, doc, "value")
	if got := doc.InferType(id); got.Basics != TypeNumber|TypeString {
		t.Fatalf("initial type = %s", got.Format())
	}
	s.clearDocument(first.URI)
	if got := doc.InferType(id); got.Basics != TypeString {
		t.Fatalf("type after removal = %s, want string", got.Format())
	}
}

func TestFiveMEventNamedHandlerAcrossFiles(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_scripts { 'handler.lua', 'register.lua' }\nclient_script 'client.lua'\n")
	handler := addFiveMTestDocument(t, s, filepath.Join(root, "res", "handler.lua"), "function handleMessage(payload) print(payload) end")
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "register.lua"), "RegisterNetEvent('message', handleMessage)")
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "client.lua"), "TriggerServerEvent('message', 42)")
	if got := handler.InferType(mustFindIdentNode(t, handler, "payload")); got.Basics != TypeNumber {
		t.Fatalf("handler payload = %s, want number", got.Format())
	}
}

func TestFiveMEventNamedCallbackAcrossFiles(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_scripts { 'callback.lua', 'events.lua' }\n")
	callback := addFiveMTestDocument(t, s, filepath.Join(root, "res", "callback.lua"), "function receiveValue(value) print(value) end")
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "events.lua"), "AddEventHandler('event', function(cb) cb(42) end)\nTriggerEvent('event', receiveValue)")
	if got := callback.InferType(mustFindIdentNode(t, callback, "value")); got.Basics != TypeNumber {
		t.Fatalf("callback value = %s, want number", got.Format())
	}
}

func TestFiveMEventConfigurationFiltersAndInvalidates(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("server.cfg", "ensure first\nensure consumer\n")
	for _, name := range []string{"first", "second", "consumer"} {
		h.writeWorkspaceFile(name+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	}
	h.writeWorkspaceFile("first/main.lua", "AddEventHandler('example', function(cb) cb(42) end)")
	h.writeWorkspaceFile("second/main.lua", "AddEventHandler('example', function(cb) cb('text') end)")
	h.writeWorkspaceFile("consumer/main.lua", "TriggerEvent('example', function(value) print(value) end)")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "main.lua"))]
	id := mustFindIdentNode(t, doc, "value")
	if got := doc.InferType(id); got.Basics != TypeNumber {
		t.Fatalf("configured type = %s, want number", got.Format())
	}
	h.writeWorkspaceFile("server.cfg", "ensure first\nensure second\nensure consumer\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "server.cfg")))
	if got := doc.InferType(id); got.Basics != TypeNumber|TypeString {
		t.Fatalf("type after cfg change = %s, want number | string", got.Format())
	}
	h.writeWorkspaceFile("server.cfg", "ensure first\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "server.cfg")))
	if got := doc.InferType(id); got.Basics != TypeNumber|TypeString {
		t.Fatalf("unlisted consumer fallback = %s, want number | string", got.Format())
	}
	h.writeWorkspaceFile("server.cfg", "ensure first\nensure unknown_resource\nensure consumer\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "server.cfg")))
	if got := doc.InferType(id); got.Basics != TypeNumber|TypeString {
		t.Fatalf("incomplete config fallback = %s, want number | string", got.Format())
	}
}

func TestFiveMEventConfigurationIncludesDependenciesAndGroups(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("server.cfg", "ensure [apps]\n")
	h.writeWorkspaceFile("[apps]/consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'provider'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("[apps]/consumer/main.lua", "TriggerEvent('example', function(value) print(value) end)")
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("provider/main.lua", "AddEventHandler('example', function(cb) cb(42) end)")
	h.writeWorkspaceFile("unused/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("unused/main.lua", "AddEventHandler('example', function(cb) cb('text') end)")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "[apps]", "consumer", "main.lua"))]
	if got := doc.InferType(mustFindIdentNode(t, doc, "value")); got.Basics != TypeNumber {
		t.Fatalf("dependency type = %s, want number", got.Format())
	}
}

func TestFiveMEventConfigurationDistinguishesStartupAndDeferredCalls(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("server.cfg", "ensure consumer\nensure provider\n")
	for _, name := range []string{"provider", "consumer"} {
		h.writeWorkspaceFile(name+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	}
	h.writeWorkspaceFile("provider/main.lua", "AddEventHandler('example', function(cb) cb(42) end)")
	h.writeWorkspaceFile("consumer/main.lua", "TriggerEvent('example', function(immediate) print(immediate) end)\nCreateThread(function() TriggerEvent('example', function(deferred) print(deferred) end) end)")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "main.lua"))]
	if got := doc.InferType(mustFindIdentNode(t, doc, "immediate")); reusableType(got) {
		t.Fatalf("immediate callback = %s, want unknown before provider startup", got.Format())
	}
	if got := doc.InferType(mustFindIdentNode(t, doc, "deferred")); got.Basics != TypeNumber {
		t.Fatalf("deferred callback = %s, want number", got.Format())
	}
}

func TestFiveMEventCallbackStructuralUnionCompletionAndDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	for _, name := range []string{"first", "second", "consumer"} {
		h.writeWorkspaceFile(name+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	}
	h.writeWorkspaceFile("first/main.lua", "AddEventHandler('object', function(cb) cb({ first = function() end, shared = 1 }) end)")
	h.writeWorkspaceFile("second/main.lua", "AddEventHandler('object', function(cb) cb({ second = true, shared = 2 }) end)")
	h.writeWorkspaceFile("consumer/main.lua", "local object\nTriggerEvent('object', function(obj) object = obj end)\nlocal x = object.--[[@union_fields]]first\nobject.shared()\nobject.first()\n")
	h.reindex()
	doc := h.docForMarker("union_fields")
	inferred := doc.InferType(mustFindIdentNode(t, doc, "object"))
	if len(inferred.Alternatives) != 2 {
		t.Fatalf("expected two object alternatives, got %#v", inferred)
	}
	items := h.completion("union_fields")
	if !completionHasLabel(items, "first") || !completionHasLabel(items, "second") || !completionHasLabel(items, "shared") {
		t.Fatalf("missing possible union fields: %#v", items.Items)
	}
	count := 0
	for _, diag := range h.diagnostics("consumer/main.lua") {
		if diag.Code == "call-non-function" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one definite non-function diagnostic, got %d", count)
	}
}

func TestFiveMEventCallbackClassUnion(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	for _, name := range []string{"first", "second", "consumer"} {
		h.writeWorkspaceFile(name+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	}
	h.writeWorkspaceFile("first/main.lua", "---@class FirstClass\n---@field first string\n---@field common number\nlocal FirstClass = {}\n---@type FirstClass\nlocal value\nAddEventHandler('object', function(cb) cb(value) end)\n")
	h.writeWorkspaceFile("second/main.lua", "---@class SecondClass\n---@field second boolean\n---@field common number\nlocal SecondClass = {}\n---@type SecondClass\nlocal value\nAddEventHandler('object', function(cb) cb(value) end)\n")
	h.writeWorkspaceFile("consumer/main.lua", "local object\nTriggerEvent('object', function(obj) object = obj end)\nlocal result = object.--[[@class_union]]common\n")
	h.reindex()
	doc := h.docForMarker("class_union")
	typ := doc.InferType(mustFindIdentNode(t, doc, "object"))
	if len(typ.Alternatives) != 2 || !strings.Contains(typ.Format(), "FirstClass") || !strings.Contains(typ.Format(), "SecondClass") {
		t.Fatalf("class union = %#v (%s)", typ, typ.Format())
	}
	items := h.completion("class_union")
	if !completionHasLabel(items, "first") || !completionHasLabel(items, "second") || !completionHasLabel(items, "common") {
		t.Fatalf("missing class union fields: %#v", items.Items)
	}
}

func TestFiveMEventCallbackSingleClassCompletionAndDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("provider/main.lua", "---@class ReturnedClass\n---@field count number\nlocal ReturnedClass = {}\n---@type ReturnedClass\nlocal value\nAddEventHandler('object', function(cb) cb(value) end)\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("consumer/main.lua", "local object\nTriggerEvent('object', function(obj) object = obj end)\nlocal result = object.--[[@single_class]]count\nobject.count()\n")
	h.reindex()
	if !completionHasLabel(h.completion("single_class"), "count") {
		t.Fatal("missing field from returned class")
	}
	if got := h.diagnostics("consumer/main.lua"); !hasDiagnosticCode(got, "call-non-function") {
		t.Fatalf("missing numeric field diagnostic: %#v", got)
	}
}

func TestFiveMEventCallbackArgumentTypeDiagnostic(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_scripts { 'shared.lua', 'methods.lua' }\n")
	h.writeWorkspaceFile("provider/shared.lua", "API = {}\nAddEventHandler('get', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("provider/methods.lua", "---@param count number\nfunction API.Send(count) end\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("consumer/main.lua", "local api\nTriggerEvent('get', function(value) api = value end)\napi.Send('wrong')\napi.Send(42)\n")
	h.reindex()
	var count int
	for _, diag := range h.diagnostics("consumer/main.lua") {
		if diag.Code == "argument-type-mismatch" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one argument type diagnostic, got %d", count)
	}
}

func TestFiveMEventCallbackArgumentTypeUnion(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	for _, name := range []string{"first", "second", "consumer"} {
		h.writeWorkspaceFile(name+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	}
	h.writeWorkspaceFile("first/main.lua", "API = {}\n---@param v string\nfunction API.Send(v) end\nAddEventHandler('get', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("second/main.lua", "API = {}\n---@param v number\nfunction API.Send(v) end\nAddEventHandler('get', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("consumer/main.lua", "local api\nTriggerEvent('get', function(value) api = value end)\napi.Send(true)\napi.Send(42)\n")
	h.reindex()
	var count int
	for _, diag := range h.diagnostics("consumer/main.lua") {
		if diag.Code == "argument-type-mismatch" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one definite mismatch across signatures, got %d", count)
	}
}

func TestFiveMEventCallbackArgumentTypeUnknownAlternative(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	for _, name := range []string{"first", "second", "consumer"} {
		h.writeWorkspaceFile(name+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	}
	h.writeWorkspaceFile("first/main.lua", "API = {}\n---@param v number\nfunction API.Send(v) end\nAddEventHandler('get', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("second/main.lua", "API = {}\nAddEventHandler('get', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("consumer/main.lua", "local api\nTriggerEvent('get', function(value) api = value end)\napi.Send('unknown')\n")
	h.reindex()
	if got := h.diagnostics("consumer/main.lua"); hasDiagnosticCode(got, "argument-type-mismatch") {
		t.Fatalf("unexpected mismatch with unknown alternative: %#v", got)
	}
}

func TestFiveMEventCallbackMethodArgumentTypeDiagnostic(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_scripts { 'shared.lua', 'methods.lua' }\n")
	h.writeWorkspaceFile("provider/shared.lua", "API = {}\nAddEventHandler('get', function(cb) cb(API) end)\n")
	h.writeWorkspaceFile("provider/methods.lua", "---@param count number\nfunction API:Send(count) end\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("consumer/main.lua", "local api\nTriggerEvent('get', function(value) api = value end)\napi:Send('wrong')\napi:Send(42)\n")
	h.reindex()
	var count int
	for _, diag := range h.diagnostics("consumer/main.lua") {
		if diag.Code == "argument-type-mismatch" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one method argument type diagnostic, got %d", count)
	}
}

func TestFiveMEventPayloadTypeDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	h.writeWorkspaceFile("res/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("res/server.lua", "---@param count number\nfunction handleCount(count) end\nRegisterNetEvent('count', handleCount)\n")
	h.writeWorkspaceFile("res/client.lua", "TriggerServerEvent('count', 'bad')\nTriggerServerEvent('count', 42)\n")
	h.reindex()
	var count int
	for _, diag := range h.diagnostics("res/client.lua") {
		if diag.Code == "fivem-event-argument-type" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one event argument mismatch, got %d", count)
	}
}

func TestFiveMEventPayloadTypeDiagnosticsMultipleHandlers(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	h.writeWorkspaceFile("res/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("res/server.lua", "---@param value number\nfunction handleNumber(value) end\n---@param value string\nfunction handleString(value) end\nRegisterNetEvent('event', handleNumber)\nRegisterNetEvent('event', handleString)\n")
	h.writeWorkspaceFile("res/client.lua", "TriggerServerEvent('event', 42)\nTriggerServerEvent('event', true)\n")
	h.reindex()
	var count int
	for _, diag := range h.diagnostics("res/client.lua") {
		if diag.Code == "fivem-event-argument-type" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one mismatch against both handlers, got %d", count)
	}
}

func TestFiveMEventSeparateCallbackAssignmentsPreserveUnion(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	for _, name := range []string{"first", "second", "consumer"} {
		h.writeWorkspaceFile(name+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	}
	h.writeWorkspaceFile("first/main.lua", "AddEventHandler('one', function(cb) cb({first = 1}) end)\n")
	h.writeWorkspaceFile("second/main.lua", "AddEventHandler('two', function(cb) cb({second = 2}) end)\n")
	h.writeWorkspaceFile("consumer/main.lua", "local object\nTriggerEvent('one', function(value) object = value end)\nTriggerEvent('two', function(value) object = value end)\nlocal x = object.--[[@separate_union]]first\n")
	h.reindex()
	doc := h.docForMarker("separate_union")
	typ := doc.InferType(mustFindIdentNode(t, doc, "object"))
	if len(typ.Alternatives) != 2 {
		t.Fatalf("expected two alternatives, got %#v", typ)
	}
	items := h.completion("separate_union")
	if !completionHasLabel(items, "first") || !completionHasLabel(items, "second") {
		t.Fatalf("missing completion from separate events: %#v", items.Items)
	}
}

func TestFiveMEventPayloadTypeDiagnosticsInlineHandler(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = true
	h.writeWorkspaceFile("res/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("res/server.lua", "---@param count number\nRegisterNetEvent('count', function(count) end)\n")
	h.writeWorkspaceFile("res/client.lua", "TriggerServerEvent('count', 'bad')\n")
	h.reindex()
	if got := h.diagnostics("res/client.lua"); !hasDiagnosticCode(got, "fivem-event-argument-type") {
		t.Fatalf("expected mismatch against inline handler, got %#v", got)
	}
}

func TestFiveMEventArgumentDiagnosticsFollowPayloadSetting(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagTypeCheck = false
	h.server.DiagFiveMEventPayload = true
	h.writeWorkspaceFile("res/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("res/server.lua", "---@param count number\nfunction handleCount(count) end\nRegisterNetEvent('count', handleCount)\n")
	h.writeWorkspaceFile("res/client.lua", "TriggerServerEvent('count', 'bad')\n")
	h.reindex()
	if got := h.diagnostics("res/client.lua"); !hasDiagnosticCode(got, "fivem-event-argument-type") {
		t.Fatalf("expected type mismatch when FiveM payload diagnostics enabled: %#v", got)
	}
	h.server.DiagFiveMEventPayload = false
	if got := h.diagnostics("res/client.lua"); hasDiagnosticCode(got, "fivem-event-argument-type") {
		t.Fatalf("unexpected type mismatch when FiveM payload diagnostics disabled: %#v", got)
	}
}

func TestFiveMEventTableMemberHandlerAndCallback(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_scripts { 'handlers.lua', 'events.lua' }\n")
	handler := addFiveMTestDocument(t, s, filepath.Join(root, "res", "handlers.lua"), "Handlers = {}\nfunction Handlers.answer(cb) cb(42) end\nfunction Handlers.receive(value) print(value) end\n")
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "events.lua"), "AddEventHandler('event', Handlers.answer)\nTriggerEvent('event', Handlers.receive)\n")
	if got := handler.InferType(mustFindIdentNode(t, handler, "value")); got.Basics != TypeNumber {
		t.Fatalf("table-member callback value = %s, want number", got.Format())
	}
}

func TestFiveMEventTableMemberCallbackNameCollision(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_scripts { 'handlers.lua', 'events.lua' }\n")
	handler := addFiveMTestDocument(t, s, filepath.Join(root, "res", "handlers.lua"), "First = {}\nSecond = {}\nfunction First.receive(value) print(value) end\nfunction Second.receive(otherValue) print(otherValue) end\n")
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "events.lua"), "AddEventHandler('event', function(cb) cb(42) end)\nTriggerEvent('event', First.receive)\n")
	if got := handler.InferType(mustFindIdentNode(t, handler, "value")); got.Basics != TypeNumber {
		t.Fatalf("first callback = %s, want number", got.Format())
	}
	if got := handler.InferType(mustFindIdentNode(t, handler, "otherValue")); reusableType(got) {
		t.Fatalf("unrelated callback = %s, want unknown", got.Format())
	}
}

func TestFiveMEventStaticNamesAndMutations(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	doc := addFiveMTestDocument(t, s, filepath.Join(root, "res", "main.lua"), "local event = 'prefix:' .. 'name'\nAddEventHandler(event, function(cb) cb(42) end)\nTriggerEvent(event, function(value) print(value) end)\nlocal dynamic = 'before'\ndynamic = 'after'\nTriggerEvent(dynamic)\n")
	if got := doc.InferType(mustFindIdentNode(t, doc, "value")); got.Basics != TypeNumber {
		t.Fatalf("static event callback = %s, want number", got.Format())
	}
	for _, event := range doc.FiveMEvents {
		if event.Name == "before" || event.Name == "after" {
			t.Fatalf("mutable event name should remain unknown: %#v", event)
		}
	}
}

func TestFiveMEventClientPayloadTypeSkipsTarget(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMEventPayload = true
	h.writeWorkspaceFile("res/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("res/client.lua", "---@param count number\nRegisterNetEvent('count', function(count) end)\n")
	h.writeWorkspaceFile("res/server.lua", "TriggerClientEvent('count', -1, 'bad')\nTriggerClientEvent('count', -1, 42)\n")
	h.reindex()
	var count int
	for _, diag := range h.diagnostics("res/server.lua") {
		if diag.Code == "fivem-event-argument-type" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one payload mismatch, excluding target, got %d", count)
	}
}

func TestFiveMLatentEventPayloadInferenceAndDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMEventPayload = true
	h.writeWorkspaceFile("res/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("res/server.lua", "---@param serverValue string\nRegisterNetEvent('server:event', function(serverValue) print(serverValue) end)\nTriggerLatentClientEvent('client:event', -1, 50000, true)\n")
	h.writeWorkspaceFile("res/client.lua", "---@param clientValue boolean\nRegisterNetEvent('client:event', function(clientValue) print(clientValue) end)\nTriggerLatentServerEvent('server:event', 50000, 'hello')\nTriggerLatentServerEvent('server:event', 50000, 99)\n")
	h.reindex()
	client := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "res", "client.lua"))]
	server := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "res", "server.lua"))]
	if got := client.InferType(mustFindIdentNode(t, client, "clientValue")); got.Basics != TypeBoolean {
		t.Fatalf("client value = %s, want boolean", got.Format())
	}
	if got := server.InferType(mustFindIdentNode(t, server, "serverValue")); got.Basics != TypeString {
		t.Fatalf("server value = %s, want string annotation", got.Format())
	}
	var mismatches int
	for _, diag := range h.diagnostics("res/client.lua") {
		if diag.Code == "fivem-event-argument-type" {
			mismatches++
		}
		if diag.Code == "fivem-event-payload" {
			t.Fatalf("latent bandwidth counted as payload: %#v", diag)
		}
	}
	if mismatches != 1 {
		t.Fatalf("expected one latent payload mismatch, got %d", mismatches)
	}
	for _, diag := range h.diagnostics("res/server.lua") {
		if diag.Code == "fivem-event-payload" || diag.Code == "fivem-event-argument-type" {
			t.Fatalf("client target or bandwidth counted as payload: %#v", diag)
		}
	}
}

func TestFiveMNetworkHandlerInferenceOverridesGenericStubOnly(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("res/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("res/server.lua", "RegisterNetEvent('value', function(payload) print(payload) end)\n---@param explicit any\nRegisterNetEvent('explicit', function(explicit) print(explicit) end)\n")
	h.writeWorkspaceFile("res/client.lua", "TriggerLatentServerEvent('value', 50000, true)\nTriggerServerEvent('explicit', 42)\n")
	h.reindex()
	server := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "res", "server.lua"))]
	if got := server.InferType(mustFindIdentNode(t, server, "payload")); got.Basics != TypeBoolean {
		t.Fatalf("inferred payload = %s, want boolean", got.Format())
	}
	if got := server.InferType(mustFindIdentNode(t, server, "explicit")); got.Basics != TypeAny {
		t.Fatalf("explicit payload = %s, want any", got.Format())
	}
}

func TestFiveMNetworkHandlerInferenceInvalidatesOnTriggerEdit(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("res/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("res/server.lua", "RegisterNetEvent('value', function(payload) print(payload) end)\n")
	h.writeWorkspaceFile("res/client.lua", "TriggerServerEvent('value', 42)\n")
	h.reindex()
	server := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "res", "server.lua"))]
	id := mustFindIdentNode(t, server, "payload")
	if got := server.InferType(id); got.Basics != TypeNumber {
		t.Fatalf("initial payload = %s", got.Format())
	}
	h.writeWorkspaceFile("res/client.lua", "TriggerServerEvent('value', 'changed')\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "res", "client.lua")))
	if got := server.InferType(id); got.Basics != TypeString {
		t.Fatalf("updated payload = %s, want string", got.Format())
	}
}

func TestFiveMEventConfigurationStopAndRestart(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("server.cfg", "ensure provider\nstop provider\nensure consumer\n")
	for _, name := range []string{"provider", "consumer"} {
		h.writeWorkspaceFile(name+"/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	}
	h.writeWorkspaceFile("provider/main.lua", "AddEventHandler('example', function(cb) cb(42) end)\n")
	h.writeWorkspaceFile("consumer/main.lua", "TriggerEvent('example', function(value) print(value) end)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "main.lua"))]
	id := mustFindIdentNode(t, doc, "value")
	if got := doc.InferType(id); reusableType(got) {
		t.Fatalf("stopped provider should not contribute: %s", got.Format())
	}
	h.writeWorkspaceFile("server.cfg", "ensure provider\nstop provider\nrestart provider\nensure consumer\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "server.cfg")))
	if got := doc.InferType(id); got.Basics != TypeNumber {
		t.Fatalf("restarted provider type = %s, want number", got.Format())
	}
}

func TestFiveMEventIndexedMemberHandlerAndCallback(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "fxmanifest.lua"), "fx_version 'cerulean'\ngame 'gta5'\nserver_scripts { 'handlers.lua', 'events.lua' }\n")
	handler := addFiveMTestDocument(t, s, filepath.Join(root, "res", "handlers.lua"), "Handlers = {}\nfunction Handlers.answer(cb) cb(42) end\nfunction Handlers.receive(value) print(value) end\n")
	addFiveMTestDocument(t, s, filepath.Join(root, "res", "events.lua"), "AddEventHandler('event', Handlers['answer'])\nTriggerEvent('event', Handlers['receive'])\n")
	if got := handler.InferType(mustFindIdentNode(t, handler, "value")); got.Basics != TypeNumber {
		t.Fatalf("indexed-member callback = %s, want number", got.Format())
	}
}
