package lsp

import (
	"path/filepath"
	"testing"
)

func TestEventDirectionQuickFix(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/server.lua", "TriggerServerEvent('custom:event', 7)\n")
	h.writeWorkspaceFile("resource/client.lua", "TriggerClientEvent('custom:event', target(), 7)\n")
	h.reindex()
	for _, tc := range []struct {
		file string
		want bool
	}{{"server.lua", true}, {"client.lua", false}} {
		uri := h.server.pathToURI(filepath.Join(h.root, "resource", tc.file))
		doc := h.server.Documents[uri]
		var found bool
		for _, d := range h.diagnostics("resource/" + tc.file) {
			if d.Code != "fivem-event-direction" {
				continue
			}
			found = true
			a, ok := h.server.eventDirectionQuickFix(doc, uri, d)
			if ok != tc.want {
				t.Fatalf("%s action=%#v ok=%v", tc.file, a, ok)
			}
			if ok && a.Edit.Changes[uri][0].NewText != "TriggerEvent" {
				t.Fatalf("bad edit: %#v", a.Edit)
			}
		}
		if !found {
			t.Fatalf("%s missing diagnostic", tc.file)
		}
	}
}

func TestUnknownEventQuickFix(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "TriggerServerEvent('custom:hello', 1)\n")
	h.writeWorkspaceFile("resource/server.lua", "AddEventHandler('custom:helo', function() end)\n")
	h.writeWorkspaceFile("other/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("other/client.lua", "TriggerServerEvent('custom:helix')\n")
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "server.lua"))
	doc := h.server.Documents[uri]
	for _, d := range h.diagnostics("resource/server.lua") {
		if d.Code != "fivem-unknown-event" {
			continue
		}
		a, ok := h.server.unknownEventQuickFix(doc, uri, d)
		if !ok {
			t.Fatal("expected suggestion")
		}
		if got := a.Edit.Changes[uri][0].NewText; got != "'custom:hello'" {
			t.Fatalf("got %q", got)
		}
		return
	}
	t.Fatal("missing diagnostic")
}

func TestUnknownEventQuickFixIgnoresOtherResources(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "TriggerServerEvent('custom:hello', 1)\n")
	h.writeWorkspaceFile("resource/server.lua", "AddEventHandler('custom:helo', function() end)\n")
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "server.lua"))
	var diag Diagnostic
	for _, d := range h.diagnostics("resource/server.lua") {
		if d.Code == "fivem-unknown-event" {
			diag = d
			break
		}
	}
	if diag.Code == "" {
		t.Fatal("missing unknown-event diagnostic")
	}

	// A trigger with the handler's misspelled name in a different resource must
	// not suppress the same-resource spelling suggestion.
	h.writeWorkspaceFile("other/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("other/client.lua", "TriggerServerEvent('custom:helo')\n")
	h.reindex()
	doc := h.server.Documents[uri]
	a, ok := h.server.unknownEventQuickFix(doc, uri, diag)
	if !ok {
		t.Fatal("expected suggestion despite trigger in another resource")
	}
	if got := a.Edit.Changes[uri][0].NewText; got != "'custom:hello'" {
		t.Fatalf("got %q", got)
	}
}

func TestUnregisteredEventQuickFix(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "TriggerServerEvent('custom:hello', 1)\n")
	h.writeWorkspaceFile("resource/server.lua", "AddEventHandler('custom:hello', function() end)\n")
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "client.lua"))
	doc := h.server.Documents[uri]
	target := h.server.pathToURI(filepath.Join(h.root, "resource", "server.lua"))
	for _, d := range h.diagnostics("resource/client.lua") {
		if d.Code != "fivem-unregistered-net-event" {
			continue
		}
		a, ok := h.server.unregisteredEventQuickFix(doc, uri, d)
		if !ok {
			t.Fatal("expected suggestion")
		}
		if got := a.Edit.Changes[target][0].NewText; got != "RegisterNetEvent('custom:hello')\n" {
			t.Fatalf("got %q", got)
		}
		return
	}
	t.Fatal("missing diagnostic")
}

func TestUnregisteredEventQuickFixIgnoresOtherResources(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "TriggerServerEvent('custom:hello', 1)\n")
	h.writeWorkspaceFile("resource/server.lua", "AddEventHandler('custom:hello', function() end)\n")
	h.reindex()
	uri := h.server.pathToURI(filepath.Join(h.root, "resource", "client.lua"))
	target := h.server.pathToURI(filepath.Join(h.root, "resource", "server.lua"))
	var diag Diagnostic
	for _, d := range h.diagnostics("resource/client.lua") {
		if d.Code == "fivem-unregistered-net-event" {
			diag = d
			break
		}
	}
	if diag.Code == "" {
		t.Fatal("missing unregistered-event diagnostic")
	}

	// Registration in a different resource does not register this resource's
	// receiving handler for the purpose of this action.
	h.writeWorkspaceFile("other/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("other/server.lua", "RegisterNetEvent('custom:hello')\n")
	h.reindex()
	doc := h.server.Documents[uri]
	a, ok := h.server.unregisteredEventQuickFix(doc, uri, diag)
	if !ok {
		t.Fatal("expected registration action despite registration in another resource")
	}
	if got := a.Edit.Changes[target][0].NewText; got != "RegisterNetEvent('custom:hello')\n" {
		t.Fatalf("got %q", got)
	}
}
