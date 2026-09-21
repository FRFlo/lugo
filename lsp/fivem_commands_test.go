package lsp

import (
	"reflect"
	"testing"
)

func assertFiveMDiagnosticsDeterministic(t *testing.T, diagnostics func() []Diagnostic) []Diagnostic {
	t.Helper()
	want := diagnostics()
	for i := 0; i < 20; i++ {
		if got := diagnostics(); !reflect.DeepEqual(got, want) {
			t.Fatalf("diagnostics differed on run %d:\n got: %#v\nwant: %#v", i+2, got, want)
		}
	}
	for i := 1; i < len(want); i++ {
		previous, current := want[i-1].Range, want[i].Range
		if current.Start.Line < previous.Start.Line || (current.Start.Line == previous.Start.Line && current.Start.Character < previous.Start.Character) {
			t.Fatalf("diagnostics are not sorted by location: %#v", want)
		}
	}
	return want
}

func TestFiveMCommandAndKeyMappingDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", `RegisterCommand("kick", function() end, true)
RegisterKeyMapping("kick", "Kick", "keyboard", "K")
RegisterKeyMapping("other", "Other", "keyboard", "K")
ExecuteCommand("missing")
`)
	h.reindex()
	diags := h.diagnostics("server.lua")
	for _, code := range []string{"fivem-command-missing-ace", "fivem-command-missing-declaration", "fivem-key-mapping-conflict"} {
		if !hasDiagnosticCode(diags, code) {
			t.Errorf("expected %s diagnostic, got %#v", code, diags)
		}
	}
}

func TestFiveMCommandDuplicateDeclarationsArePreservedAndDeterministic(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", `RegisterCommand("kick", function() end, true)
RegisterCommand("kick", function() end, true)
`)
	h.reindex()
	diags := assertFiveMDiagnosticsDeterministic(t, func() []Diagnostic { return h.diagnostics("server.lua") })
	count := 0
	for _, diag := range diags {
		if diag.Code == "fivem-command-missing-ace" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("missing ACE diagnostics = %d, want 2: %#v", count, diags)
	}
}

func TestFiveMCommandDynamicNamesAreIgnored(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n")
	h.writeWorkspaceFile("server.lua", "local name = 'dynamic'\nRegisterCommand(name, function() end)\nExecuteCommand(name)\n")
	h.reindex()
	for _, diag := range h.diagnostics("server.lua") {
		if diag.Code == "fivem-command-missing-declaration" || diag.Code == "fivem-command-missing-ace" {
			t.Fatalf("dynamic command produced diagnostic: %#v", diag)
		}
	}
}
