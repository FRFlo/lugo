package lsp

import (
	"strings"
	"testing"
)

func TestFiveMGLMTransformAndInverseSignatures(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local matrix = glm.mat4(1)
local offset = glm.vec3(1, 2, 3)
local translated = glm.translate(matrix, --[[@translate_sig]]offset)
local rotated = glm.rotate(matrix, 1, --[[@rotate_sig]]offset)
local scaled = glm.scale(matrix, --[[@scale_sig]]offset)
local inverseQuat = glm.inverse(--[[@inverse_quat_sig]]glm.quat())
local inverseMat2 = glm.inverse(--[[@inverse_mat2_sig]]glm.mat2())
local inverseMat3 = glm.inverse(--[[@inverse_mat3_sig]]glm.mat3())
local inverseMat4 = glm.inverse(--[[@inverse_mat4_sig]]matrix)
`)
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	for _, tt := range []struct{ name, want string }{
		{"translated", "Matrix4x4"}, {"rotated", "Matrix4x4"}, {"scaled", "Matrix4x4"},
		{"inverseQuat", "Quat"}, {"inverseMat2", "Matrix2x2"},
		{"inverseMat3", "Matrix3x3"}, {"inverseMat4", "Matrix4x4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := doc.InferType(findIdentByOccurrence(t, doc.Tree, tt.name, 0))
			if name := fiveMGLMTestTypeName(got); name != tt.want {
				t.Fatalf("%s inferred as %q (%#v), want %q", tt.name, name, got, tt.want)
			}
		})
	}
	for _, tt := range []struct{ marker, want string }{
		{"translate_sig", "translate(m: Matrix4x4, translation: Vector3): Matrix4x4"},
		{"rotate_sig", "rotate(m: Matrix4x4, angle: number, axis: Vector3): Matrix4x4"},
		{"scale_sig", "scale(m: Matrix4x4, factors: Vector3): Matrix4x4"},
		{"inverse_quat_sig", "inverse(m: Quat): Quat"},
		{"inverse_mat2_sig", "inverse(m: Matrix2x2): Matrix2x2"},
		{"inverse_mat3_sig", "inverse(m: Matrix3x3): Matrix3x3"},
		{"inverse_mat4_sig", "inverse(m: Matrix4x4): Matrix4x4"},
	} {
		t.Run(tt.marker, func(t *testing.T) {
			help := h.signatureHelp(tt.marker)
			if help == nil || len(help.Signatures) == 0 {
				t.Fatalf("signature help for %s is empty", tt.marker)
			}
			if strings.HasPrefix(tt.marker, "inverse_") && (help.ActiveSignature < 0 || help.ActiveSignature >= len(help.Signatures) || !strings.Contains(help.Signatures[help.ActiveSignature].Label, tt.want)) {
				t.Fatalf("active signature for %s = %#v, want %q", tt.marker, help, tt.want)
			}
			for _, signature := range help.Signatures {
				if strings.Contains(signature.Label, tt.want) {
					return
				}
			}
			t.Fatalf("signature help for %s = %#v, want %q", tt.marker, help.Signatures, tt.want)
		})
	}
}
