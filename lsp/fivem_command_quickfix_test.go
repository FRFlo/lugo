package lsp

import (
	"strings"
	"testing"
)

func TestFiveMMissingDeclarationOffersExplicitSkeletonAlternatives(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("client.lua", "RegisterKeyMapping('inspect', 'Inspect', 'keyboard', 'I')\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/client.lua")]
	if doc == nil {
		t.Fatal("client.lua document not indexed")
	}
	var diag Diagnostic
	for _, candidate := range h.diagnostics("client.lua") {
		if candidate.Code == "fivem-command-missing-declaration" {
			diag = candidate
			break
		}
	}
	if diag.Code == "" {
		t.Fatal("expected missing declaration diagnostic")
	}

	actions := h.server.fiveMCommandQuickFixes(doc, doc.URI, diag)
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want restricted and unrestricted choices: %#v", len(actions), actions)
	}
	if !strings.Contains(actions[0].Title, "restricted (ACE permission required)") || !strings.Contains(actions[1].Title, "unrestricted") {
		t.Fatalf("actions do not clearly describe the permission choice: %#v", actions)
	}
	for i, action := range actions {
		if action.Edit == nil || len(action.Edit.Changes[doc.URI]) != 1 {
			t.Fatalf("action should edit only the current Lua file: %#v", action)
		}
		edit := action.Edit.Changes[doc.URI][0]
		if edit.Range.Start != edit.Range.End || edit.Range.Start != getRange(doc.Tree, uint32(len(doc.Source())), uint32(len(doc.Source()))).Start {
			t.Fatalf("skeleton should be appended without replacing existing mappings: %#v", edit)
		}
		if !strings.Contains(edit.NewText, "RegisterCommand('inspect'") || !strings.Contains(edit.NewText, "TODO: implement command behavior") {
			t.Fatalf("action should insert an editable command skeleton: %#v", action)
		}
		want := "false"
		if i == 0 {
			want = "true"
		}
		if !strings.HasSuffix(edit.NewText, ", "+want+")\n") {
			t.Fatalf("action restricted argument = %q, want %q", edit.NewText, want)
		}
	}
}

func TestFiveMExecuteCommandQuickFixUsesOnlyCommandToken(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", "ExecuteCommand('inspect target')\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/server.lua")]
	var diag Diagnostic
	for _, candidate := range h.diagnostics("server.lua") {
		if candidate.Code == "fivem-command-missing-declaration" {
			diag = candidate
			break
		}
	}
	if diag.Code == "" {
		t.Fatal("expected missing declaration diagnostic for ExecuteCommand")
	}
	actions := h.server.fiveMCommandQuickFixes(doc, doc.URI, diag)
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want restricted and unrestricted alternatives", len(actions))
	}
	for _, action := range actions {
		edit := action.Edit.Changes[doc.URI][0]
		if !strings.Contains(edit.NewText, "RegisterCommand('inspect'") || strings.Contains(edit.NewText, "inspect target") {
			t.Fatalf("action used the full ExecuteCommand string instead of its command token: %#v", action)
		}
	}
}

func TestFiveMCommandQuickFixesSkipUnresolvedChoices(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", "RegisterCommand('kick', function() end, true)\nRegisterKeyMapping('first', 'First', 'keyboard', 'K')\nRegisterKeyMapping('second', 'Second', 'keyboard', 'K')\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/server.lua")]
	for _, diag := range h.diagnostics("server.lua") {
		if diag.Code == "fivem-command-missing-ace" || diag.Code == "fivem-key-mapping-conflict" {
			if got := h.server.fiveMCommandQuickFixes(doc, doc.URI, diag); len(got) != 0 {
				t.Fatalf("unexpected action guessing a principal or deleting a conflicting mapping for %s: %#v", diag.Code, got)
			}
		}
	}
}
