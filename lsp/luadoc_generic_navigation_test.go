package lsp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLuaDocGenericClassLSPNavigationAndCompletion(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("types.lua", `---@class Box<T>
---@field value T
local Box = {}
`)
	h.writeWorkspaceFile("use.lua", `---@type Box<string>
local box = {}
local first = box.--[[@generic_field_hover]]value
local second = box.--[[@generic_field_definition]]value
local third = box.--[[@generic_field_reference]]value
box.--[[@generic_field_completion]]
`)
	h.reindex()

	hover := h.hover("generic_field_hover")
	if hover == nil || !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("generic field hover = %#v, want string", hover)
	}

	locations := h.definition("generic_field_definition")
	if len(locations) == 0 {
		t.Error("generic field definition returned no locations")
	} else {
		typesURI := h.server.pathToURI(filepath.Join(h.root, "types.lua"))
		if locations[0].URI != typesURI {
			t.Errorf("generic field definition URI = %q, want %q", locations[0].URI, typesURI)
		}
	}

	references := h.references("generic_field_reference", false)
	useURI := h.server.pathToURI(filepath.Join(h.root, "use.lua"))
	foundUseReference := false
	for _, reference := range references {
		if reference.URI == useURI {
			foundUseReference = true
			break
		}
	}
	if !foundUseReference {
		t.Errorf("generic field references = %#v, want a use.lua reference", references)
	}

	completion := h.completion("generic_field_completion")
	foundValue := false
	for _, item := range completion.Items {
		if item.Label == "value" {
			foundValue = true
			break
		}
	}
	if !foundValue {
		t.Errorf("generic class completion omitted value: %#v", completion.Items)
	}
}
