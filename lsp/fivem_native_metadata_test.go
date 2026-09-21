package lsp

import (
	"strings"
	"testing"
)

func TestFiveMNativeMetadataAssistance(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_natives")

	hover := h.hover("native_client_call")
	if hover == nil || !strings.Contains(hover.Contents.Value, "Native") || !strings.Contains(hover.Contents.Value, "PLAYER") || !strings.Contains(hover.Contents.Value, "client") || !strings.Contains(hover.Contents.Value, "entity handle") {
		t.Fatalf("native hover lacks namespace/context/note: %#v", hover)
	}

	deprecated := h.hover("native_native_deprecated")
	if deprecated == nil || !strings.Contains(deprecated.Contents.Value, "deprecated") {
		t.Fatalf("native hover lacks deprecation metadata: %#v", deprecated)
	}

	help := h.signatureHelp("native_native_signature")
	if help == nil || len(help.Signatures) == 0 {
		t.Fatal("native signature help is empty")
	}
	if !strings.Contains(help.Signatures[0].Label, ": ") || !strings.Contains(help.Signatures[0].Label, "integer") {
		t.Fatalf("native signature lacks return metadata: %#v", help.Signatures[0])
	}

	items := h.completion("native_native_completion")
	var native *CompletionItem
	for i := range items.Items {
		if items.Items[i].Label == "PlayerPedId" {
			native = &items.Items[i]
			break
		}
	}
	if native == nil || !strings.Contains(native.Detail, "native PLAYER") || native.Documentation == nil || !strings.Contains(native.Documentation.Value, "client") {
		t.Fatalf("native completion lacks metadata: %#v", native)
	}
}
