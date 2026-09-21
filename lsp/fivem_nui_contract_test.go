package lsp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestNUIContractNamesUseLiteralHandlers(t *testing.T) {
	src := []byte(`
fetch('https://${GetParentResourceName()}/open_menu')
window.addEventListener('message', (event) => {
  if (event.data.action === 'show_menu') {}
})
fetch('https://${GetParentResourceName()}/' + action)
`)
	names := nuiJSNames(src, false)
	got := make(map[string]bool)
	for _, name := range names {
		got[name.name] = true
	}
	for _, want := range []string{"open_menu", "show_menu"} {
		if !got[want] {
			t.Fatalf("literal handler %q was not indexed: %#v", want, names)
		}
	}
	if got["action"] {
		t.Fatal("dynamic callback name was indexed")
	}
}

func TestNUIContractDiagnosticRange(t *testing.T) {
	src := []byte("RegisterNUICallback('missing', function() end)")
	diag := nuiDiag(src, strings.Index(string(src), "missing"), strings.Index(string(src), "missing")+len("missing"), "fivem-nui-missing-handler", "missing")
	if diag.Range.Start.Character == 0 || diag.Code != "fivem-nui-missing-handler" {
		t.Fatalf("unexpected diagnostic: %#v", diag)
	}
}

func TestNUIContractDiagnosticsClearCleanAsset(t *testing.T) {
	h := newNUIContractDiagnosticHarness(t)
	assetURI := h.server.pathToURI(h.root + "/resource/ui.js")

	h.server.refreshWorkspace()
	published, diags := nuiPublishedDiagnostics(t, h, assetURI)
	if !published || !hasDiagnosticCode(diags, "fivem-nui-unused-handler") {
		t.Fatalf("initial NUI asset diagnostics = published:%t diagnostics:%#v, want fivem-nui-unused-handler", published, diags)
	}

	h.writeWorkspaceFile("resource/client.lua", "RegisterNUICallback('orphan', function() end)\n")
	h.server.refreshWorkspace()

	published, diags = nuiPublishedDiagnostics(t, h, assetURI)
	if !published || len(diags) != 0 {
		t.Fatalf("clean NUI asset diagnostics = published:%t diagnostics:%#v, want an explicit clear", published, diags)
	}
}

func TestNUIContractDiagnosticsClearDeletedAsset(t *testing.T) {
	h := newNUIContractDiagnosticHarness(t)
	assetPath := h.root + "/resource/ui.js"
	assetURI := h.server.pathToURI(assetPath)

	h.server.refreshWorkspace()
	published, diags := nuiPublishedDiagnostics(t, h, assetURI)
	if !published || !hasDiagnosticCode(diags, "fivem-nui-unused-handler") {
		t.Fatalf("initial NUI asset diagnostics = published:%t diagnostics:%#v, want fivem-nui-unused-handler", published, diags)
	}

	if err := os.Remove(assetPath); err != nil {
		t.Fatalf("remove NUI asset: %v", err)
	}
	h.server.refreshWorkspace()

	published, diags = nuiPublishedDiagnostics(t, h, assetURI)
	if !published || len(diags) != 0 {
		t.Fatalf("deleted NUI asset diagnostics = published:%t diagnostics:%#v, want an explicit clear", published, diags)
	}
}

func newNUIContractDiagnosticHarness(t *testing.T) *fiveMFixtureHarness {
	t.Helper()
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n")
	h.writeWorkspaceFile("resource/client.lua", "RegisterNUICallback('known', function() end)\n")
	h.writeWorkspaceFile("resource/ui.js", "fetch('https://${GetParentResourceName()}/orphan')\n")
	return h
}

func nuiPublishedDiagnostics(t *testing.T, h *fiveMFixtureHarness, uri string) (bool, []Diagnostic) {
	t.Helper()
	for _, msg := range h.readRPCMessages() {
		var notification struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(msg, &notification); err != nil || notification.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(notification.Params, &params); err != nil {
			t.Fatalf("decode diagnostics notification: %v", err)
		}
		if params.URI == uri {
			return true, params.Diagnostics
		}
	}
	return false, nil
}
