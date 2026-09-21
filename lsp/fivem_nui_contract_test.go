package lsp

import (
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
