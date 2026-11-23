package lsp

import "testing"

func TestFiveMOALVectorQuickFixSignatureSafety(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		want     bool
	}{
		{name: "number", typeName: "number", want: true},
		{name: "integer", typeName: "integer", want: true},
		{name: "union is ambiguous", typeName: "number|string"},
		{name: "string is incompatible", typeName: "string"},
		{name: "empty", typeName: ""},
		{name: "optional number", typeName: "number?", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fiveMNativeNumericParam(tt.typeName); got != tt.want {
				t.Fatalf("fiveMNativeNumericParam(%q) = %v, want %v", tt.typeName, got, tt.want)
			}
		})
	}
}

func TestFiveMOALVectorQuickFixRejectsStaleDiagnostic(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nuse_experimental_fxv2_oal 'yes'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "local pos = vector3(1, 2, 3)\nSetEntityCoords(PlayerPedId(), pos)\n")
	h.reindex()
	uri := h.server.pathToURI(h.root + "/resource/client.lua")
	var stale Diagnostic
	for _, diag := range h.diagnostics("resource/client.lua") {
		if diag.Code == "fivem-oal-vector-argument" {
			stale = diag
			break
		}
	}
	if stale.Code == "" {
		t.Fatal("fixture did not produce the OAL vector diagnostic")
	}

	h.writeWorkspaceFile("resource/client.lua", "local pos = 42\nSetEntityCoords(PlayerPedId(), pos)\n")
	h.reindex()
	doc := h.server.Documents[uri]
	if action, ok := h.server.oalVectorQuickFix(doc, uri, stale); ok {
		t.Fatalf("stale diagnostic produced an action: %#v", action)
	}
}

func TestFiveMOALVectorQuickFixRejectsCallExpression(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nuse_experimental_fxv2_oal 'yes'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "SetEntityCoords(PlayerPedId(), GetEntityCoords(PlayerPedId()))\n")
	h.reindex()
	uri := h.server.pathToURI(h.root + "/resource/client.lua")
	doc := h.server.Documents[uri]
	for _, diag := range h.diagnostics("resource/client.lua") {
		if diag.Code != "fivem-oal-vector-argument" {
			continue
		}
		if action, ok := h.server.oalVectorQuickFix(doc, uri, diag); ok {
			t.Fatalf("call expression with effects produced an action: %#v", action)
		}
		return
	}
	t.Fatal("fixture did not produce the OAL vector diagnostic")
}

func TestFiveMOALVectorQuickFixExpandsResolvedLocalVector3(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nuse_experimental_fxv2_oal 'yes'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("resource/client.lua", "local pos = vector3(1, 2, 3)\nSetEntityCoords(PlayerPedId(), pos)\n")
	h.reindex()
	uri := h.server.pathToURI(h.root + "/resource/client.lua")
	doc := h.server.Documents[uri]
	var diag Diagnostic
	for _, candidate := range h.diagnostics("resource/client.lua") {
		if candidate.Code == "fivem-oal-vector-argument" {
			diag = candidate
			break
		}
	}
	if diag.Code == "" {
		t.Fatal("fixture did not produce the OAL vector diagnostic")
	}
	action, ok := h.server.oalVectorQuickFix(doc, uri, diag)
	if !ok || action.Edit == nil {
		t.Fatalf("expected OAL vector quickfix, got %#v", action)
	}
	edits := action.Edit.Changes[uri]
	if len(edits) != 1 || edits[0].NewText != "pos.x, pos.y, pos.z" {
		t.Fatalf("quickfix edits = %#v, want pos.x, pos.y, pos.z", edits)
	}
}
