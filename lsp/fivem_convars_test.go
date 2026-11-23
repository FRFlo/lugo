package lsp

import (
	"path/filepath"
	"testing"
)

func TestFiveMConvarWritesReadsAndDeclarations(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nconvar 'manifest_declared'\nserver_script 'server.lua'\nshared_script 'shared.lua'\n")
	h.writeWorkspaceFile("server.lua", `SetConvar("known", "one")
ExecuteCommand("set declared value")
GetConvar("known", "one")
GetConvar("declared", "value")
GetConvar("manifest_declared", "")
`)
	h.writeWorkspaceFile("shared.lua", `GetConvar("missing", "")`)
	h.reindex()
	if diags := h.diagnostics("server.lua"); hasDiagnosticCode(diags, "fivem-convar-unknown") {
		t.Fatalf("declared convars were reported unknown: %#v", diags)
	}
	if !hasDiagnosticCode(h.diagnostics("shared.lua"), "fivem-convar-unknown") {
		t.Fatal("expected unknown convar read diagnostic")
	}
}

func TestFiveMConvarConflictsAndScope(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", `SetConvar("mode", "one")
SetConvar("mode", "two")
GetConvarBool("mode", false)
GetConvar("mode", "three")
`)
	h.writeWorkspaceFile("client.lua", `SetConvar("client_only", "x")
SetConvarReplicated("replicated", "x")
`)
	h.reindex()
	server := assertFiveMDiagnosticsDeterministic(t, func() []Diagnostic { return h.diagnostics("server.lua") })
	if !hasDiagnosticCode(server, "fivem-convar-conflict") {
		t.Fatal("expected conflicting convar declaration diagnostic")
	}
	if !hasDiagnosticCode(server, "fivem-convar-type-conflict") {
		t.Fatal("expected conflicting convar type diagnostic")
	}
	if !hasDiagnosticCode(server, "fivem-convar-default-conflict") {
		t.Fatal("expected conflicting convar default diagnostic")
	}
	client := h.diagnostics("client.lua")
	if !hasDiagnosticCode(client, "fivem-convar-scope") {
		t.Fatalf("expected convar scope diagnostics, got %#v", client)
	}
}

func TestFiveMConvarDynamicNamesAreIgnored(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", `local name = "dynamic"
SetConvar(name, "value")
GetConvar(name, "")
`)
	h.reindex()
	if hasDiagnosticCode(h.diagnostics("server.lua"), "fivem-convar-unknown") {
		t.Fatal("dynamic convar name produced a diagnostic")
	}
}

func TestFiveMConvarQuickFixOffersExistingNamesAndManifestDeclaration(t *testing.T) {
	h := newFiveMFixtureHarness(t, "")
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\nconvar 'feature_mode'\n")
	h.writeWorkspaceFile("server.lua", `GetConvar("feature_mdoe", "")`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "server.lua"))]
	var diagnostic Diagnostic
	for _, candidate := range h.diagnostics("server.lua") {
		if candidate.Code == "fivem-convar-unknown" {
			diagnostic = candidate
			break
		}
	}
	if diagnostic.Code == "" {
		t.Fatal("expected unknown convar diagnostic")
	}
	actions := h.server.fiveMConvarQuickFix(doc, doc.URI, diagnostic)
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want rename candidate and manifest declaration: %#v", len(actions), actions)
	}
	if actions[0].Title != "Use existing convar 'feature_mode'" || actions[1].Title != "Declare convar 'feature_mdoe' in fxmanifest.lua" {
		t.Fatalf("unexpected alternatives: %#v", actions)
	}
	if actions[1].Edit.Changes[doc.URI] != nil || len(actions[1].Edit.Changes) != 1 {
		t.Fatalf("declaration should edit only manifest: %#v", actions[1].Edit)
	}
}

func TestFiveMConvarQuickFixUsesExistingTypeAndDefault(t *testing.T) {
	h := newFiveMFixtureHarness(t, "")
	h.writeWorkspaceFile("server.lua", `SetConvar("enabled", 1)
GetConvarBool("enabled", false)
SetConvar("retries", 3)
GetConvar("retries", 4)`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "server.lua"))]
	var typeDiag, defaultDiag Diagnostic
	for _, diagnostic := range h.diagnostics("server.lua") {
		switch diagnostic.Code {
		case "fivem-convar-type-conflict":
			typeDiag = diagnostic
		case "fivem-convar-default-conflict":
			defaultDiag = diagnostic
		}
	}
	if typeDiag.Code == "" || defaultDiag.Code == "" {
		t.Fatalf("expected type and default diagnostics: type=%#v default=%#v", typeDiag, defaultDiag)
	}
	typeActions := h.server.fiveMConvarQuickFix(doc, doc.URI, typeDiag)
	if len(typeActions) != 2 || typeActions[0].Title != "Read convar with GetConvarFloat() and its declared default" || typeActions[1].Title != "Read convar with GetConvarInt() and its declared default" {
		t.Fatalf("type alternatives = %#v", typeActions)
	}
	defaultActions := h.server.fiveMConvarQuickFix(doc, doc.URI, defaultDiag)
	if len(defaultActions) != 1 || defaultActions[0].Title != `Use existing default "3"` {
		t.Fatalf("default alternatives = %#v", defaultActions)
	}
}

func TestFiveMConvarQuickFixDeclarationConflictUsesExistingValue(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("server.lua", `SetConvar("mode", "safe")
SetConvar("mode", "unsafe")`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "server.lua"))]
	var diagnostic Diagnostic
	for _, candidate := range h.diagnostics("server.lua") {
		if candidate.Code == "fivem-convar-conflict" {
			diagnostic = candidate
			break
		}
	}
	if diagnostic.Code == "" {
		t.Fatal("expected conflicting declaration diagnostic")
	}
	actions := h.server.fiveMConvarQuickFix(doc, doc.URI, diagnostic)
	if len(actions) != 1 || actions[0].Title != `Use existing convar value "safe"` {
		t.Fatalf("declaration alternatives = %#v", actions)
	}
}
