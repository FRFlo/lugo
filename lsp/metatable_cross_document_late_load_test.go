package lsp

import (
	"path/filepath"
	"testing"
)

func TestMetatableTypeCacheInvalidatesWhenProviderLoadsLater(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("read.lua", `local result = object.--[[@late_provider_read]]answer
`)
	h.reindex()
	readDoc := h.docForMarker("late_provider_read")
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics != TypeUnknown {
		t.Fatalf("before provider load = %#v, want unknown", got)
	}

	setupURI := h.server.pathToURI(filepath.Join(h.root, "setup.lua"))
	h.server.updateDocument(setupURI, []byte(`object = {}
setmetatable(object, { __index = { answer = 42 } })
`))
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("after provider load = %#v, want number", got)
	}
}

func TestMetatableTypeCacheInvalidatesWhenManifestHidesProvider(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script { 'client.lua', '@provider/shared.lua' }\n")
	h.writeWorkspaceFile("consumer/client.lua", `local result = object.--[[@manifest_provider_read]]answer
`)
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'shared.lua'\n")
	h.writeWorkspaceFile("provider/shared.lua", `object = {}
setmetatable(object, { __index = { answer = 42 } })
`)
	h.reindex()
	readDoc := h.docForMarker("manifest_provider_read")
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics&TypeNumber == 0 {
		t.Fatalf("before manifest change = %#v, want number", got)
	}

	manifestURI := h.server.pathToURI(filepath.Join(h.root, "consumer", "fxmanifest.lua"))
	h.server.updateDocument(manifestURI, []byte("fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n"))
	if got := inferLastNamedMember(t, readDoc, "answer"); got.Basics != TypeUnknown {
		t.Fatalf("after manifest change = %#v, want unknown", got)
	}
}
