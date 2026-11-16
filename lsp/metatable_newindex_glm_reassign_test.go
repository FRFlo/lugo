package lsp

import "testing"

func TestReassignedGLMConstructorDoesNotPanicDuringFieldInference(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `glm.vec3 = function(...) return "custom" end
local result = glm.vec3(1, 2, 3)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	_ = doc.InferType(findIdentByOccurrence(t, doc.Tree, "result", 0))
}
