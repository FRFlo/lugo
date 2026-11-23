package lsp

import "testing"

func TestFiveMWaitZeroQuickFixOffersExplicitDelays(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMPerformance = true
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("client.lua", "while true do\n  Wait(0)\nend\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/client.lua")]
	var diag Diagnostic
	for _, candidate := range h.diagnostics("client.lua") {
		if candidate.Code == "fivem-performance-wait-zero" {
			diag = candidate
			break
		}
	}
	if diag.Code == "" {
		t.Fatal("expected Wait(0) diagnostic")
	}
	actions := h.codeActions("client.lua", diag.Range, []Diagnostic{diag})
	want := map[string]string{"Set Wait delay to 100 ms": "100", "Set Wait delay to 1000 ms": "1000"}
	for _, action := range actions {
		delay, ok := want[action.Title]
		if !ok {
			continue
		}
		edit, ok := action.Edit.Changes[doc.URI]
		if !ok || len(edit) != 1 || edit[0].NewText != delay {
			t.Fatalf("action %q does not replace only the literal delay: %#v", action.Title, action)
		}
		delete(want, action.Title)
	}
	if len(want) != 0 {
		t.Fatalf("missing delay actions: %#v", want)
	}
}

func TestFiveMPerformanceQuickFixDoesNotGuessYieldForOtherDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMPerformance = true
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("client.lua", "while true do\n  DoWork()\nend\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/client.lua")]
	for _, diag := range h.diagnostics("client.lua") {
		if diag.Code == "fivem-performance-no-yield" && len(fiveMPerformanceQuickFixes(doc, doc.URI, diag)) != 0 {
			t.Fatalf("unexpected automatic yield insertion for uncertain control flow: %#v", diag)
		}
	}
}

func TestFiveMWaitZeroQuickFixSkipsNestedFunctionWait(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMPerformance = true
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("client.lua", "while true do\n  local f = function() Wait(0) end\nend\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/client.lua")]
	for _, diag := range h.diagnostics("client.lua") {
		if diag.Code == "fivem-performance-wait-zero" && len(fiveMPerformanceQuickFixes(doc, doc.URI, diag)) != 0 {
			t.Fatalf("unexpected action for Wait in nested function: %#v", diag)
		}
	}
}
