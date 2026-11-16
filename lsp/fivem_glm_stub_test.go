package lsp

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFiveMGLMRuntimeConstructorsAreNavigable(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local namespaceCompletion = glm.--[[@glm_constructor_completion]]
local globalCompletion = vect--[[@global_constructor_completion]]
local globalVectorHover = --[[@global_vector_hover]]vector(1, 2, 3)
local globalVectorDefinition = --[[@global_vector_definition]]vector(1, 2, 3)
local globalHover = --[[@global_vector1_hover]]vector1()
local globalDefinition = --[[@global_vector1_definition]]vector1()
local namespaceHover = glm.--[[@glm_vec_hover]]vec()
local namespaceDefinition = glm.--[[@glm_vec_definition]]vec()
`)
	h.reindex()

	namespaceLabels := labelsFromItems(h.completion("glm_constructor_completion").Items)
	for _, name := range []string{
		"vec", "vec1", "vec2", "vec3", "vec4",
		"ivec", "ivec1", "ivec2", "ivec3", "ivec4",
		"bvec", "bvec1", "bvec2", "bvec3", "bvec4",
		"mat", "mat2", "mat2x2", "mat2x3", "mat2x4",
		"mat3", "mat3x2", "mat3x3", "mat3x4",
		"mat4", "mat4x2", "mat4x3", "mat4x4", "qua", "quat",
	} {
		if !slices.Contains(namespaceLabels, name) {
			t.Errorf("glm completion labels = %#v, missing runtime constructor %q", namespaceLabels, name)
		}
	}
	globalLabels := labelsFromItems(h.completion("global_constructor_completion").Items)
	for _, name := range []string{"vector", "vector1"} {
		if !slices.Contains(globalLabels, name) {
			t.Errorf("global constructor completion labels = %#v, missing %q", globalLabels, name)
		}
	}

	hover := h.hover("global_vector1_hover")
	if hover == nil || !strings.Contains(hover.Contents.Value, "vector1") {
		t.Errorf("vector1 hover = %#v, want its runtime constructor signature", hover)
	}
	vectorHover := h.hover("global_vector_hover")
	if vectorHover == nil || !strings.Contains(vectorHover.Contents.Value, "vector") {
		t.Errorf("vector hover = %#v, want its runtime constructor signature", vectorHover)
	}
	definitions := h.definition("global_vector1_definition")
	if len(definitions) == 0 || !strings.HasSuffix(filepath.Base(definitions[0].URI), "fivem_glm.lua") {
		t.Errorf("vector1 definitions = %#v, want the embedded fivem_glm.lua declaration", definitions)
	}
	vectorDefinitions := h.definition("global_vector_definition")
	if len(vectorDefinitions) == 0 || !strings.HasSuffix(filepath.Base(vectorDefinitions[0].URI), "fivem_glm.lua") {
		t.Errorf("vector definitions = %#v, want the embedded fivem_glm.lua declaration", vectorDefinitions)
	}
	namespaceHover := h.hover("glm_vec_hover")
	if namespaceHover == nil || !strings.Contains(namespaceHover.Contents.Value, "glm.vec") {
		t.Errorf("glm.vec hover = %#v, want its constructor signature", namespaceHover)
	}
	namespaceDefinitions := h.definition("glm_vec_definition")
	if len(namespaceDefinitions) == 0 || !strings.HasSuffix(filepath.Base(namespaceDefinitions[0].URI), "fivem_glm.lua") {
		t.Errorf("glm.vec definitions = %#v, want the embedded fivem_glm.lua declaration", namespaceDefinitions)
	}
}
