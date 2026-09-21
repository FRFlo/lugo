package lsp

import (
	"strings"
	"testing"
)

func TestVirtualBuiltinEventDefinitionUsesReadableURI(t *testing.T) {
	if !strings.HasPrefix(builtinFiveMEventsURI, "builtin://") || !strings.Contains(builtinFiveMEventsURI, "fivem/events") {
		t.Fatalf("builtin event URI = %q, want readable virtual catalog URI", builtinFiveMEventsURI)
	}
}

func TestVirtualNativeDefinitionIsNavigable(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_natives")
	locations := h.definition("native_client_call")
	if len(locations) == 0 {
		t.Fatal("native definition returned no locations")
	}
	if !strings.HasPrefix(locations[0].URI, embeddedStdlibURIPrefix) || !strings.Contains(locations[0].URI, "natives_client.lua") {
		t.Fatalf("native definition URI = %q, want resolvable stdlib resource", locations[0].URI)
	}
}
