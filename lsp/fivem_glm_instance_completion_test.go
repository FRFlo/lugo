package lsp

import (
	"slices"
	"strings"
	"testing"
)

func TestFiveMGLMVectorMetatableMethodCompletion(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `---@type Vector2
local v2 = {}
---@type Vector4
local v4 = {}
---@type Quat
local q = {}
v2:--[[@vector2_methods]]
v4:--[[@vector4_methods]]
q:--[[@quat_methods]]
`)
	h.reindex()

	for _, tc := range []struct {
		marker  string
		methods []string
	}{
		{marker: "vector2_methods", methods: []string{"dot", "length", "length2", "normalize", "clamp", "lerp", "cross"}},
		{marker: "vector4_methods", methods: []string{"dot", "length", "length2", "normalize", "clamp", "lerp"}},
		{marker: "quat_methods", methods: []string{"lerp", "cross", "inverse"}},
	} {
		t.Run(tc.marker, func(t *testing.T) {
			labels := labelsFromItems(h.completion(tc.marker).Items)
			for _, method := range tc.methods {
				if !slices.Contains(labels, method) {
					t.Errorf("%s completion = %#v, missing runtime glm method %q", tc.marker, labels, method)
				}
			}
			if tc.marker == "quat_methods" {
				for _, method := range []string{"dot", "length", "length2", "normalize", "clamp"} {
					if slices.Contains(labels, method) {
						t.Errorf("%s completion = %#v, includes vector-only method %q", tc.marker, labels, method)
					}
				}
			}
			for _, method := range []string{"clone", "normalizeAngle", "zero"} {
				if slices.Contains(labels, method) {
					t.Errorf("%s completion = %#v, includes unregistered runtime method %q", tc.marker, labels, method)
				}
			}
		})
	}

	v2Labels := labelsFromItems(h.completion("vector2_methods").Items)
	if slices.Contains(v2Labels, "z") || slices.Contains(v2Labels, "w") {
		t.Fatalf("Vector2 completion included out-of-dimension components: %#v", v2Labels)
	}
	v4Labels := labelsFromItems(h.completion("vector4_methods").Items)
	for _, field := range []string{"x", "y", "z", "w"} {
		if !slices.Contains(v4Labels, field) {
			t.Errorf("Vector4 completion = %#v, missing inherited component %q", v4Labels, field)
		}
	}
	quatLabels := labelsFromItems(h.completion("quat_methods").Items)
	for _, field := range []string{"x", "y", "z", "w"} {
		if !slices.Contains(quatLabels, field) {
			t.Errorf("Quat completion = %#v, missing component %q", quatLabels, field)
		}
	}
	for _, field := range []string{"r", "g", "b", "a"} {
		if slices.Contains(v2Labels, field) {
			t.Errorf("Vector2 completion included out-of-dimension alias %q: %#v", field, v2Labels)
		}
	}
}

func TestFiveMGLMClampAndConstructorSignatures(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'main.lua'\n")
	h.writeWorkspaceFile("resource/main.lua", `local v = vec3()
local clamped = v:clamp(--[[@vector_clamp_signature]])
local dynamic = glm.vec(--[[@glm_vec_variadic_signature]])
local globalDynamic = vec(--[[@global_vec_variadic_signature]])
local globalClamped = glm.--[[@glm_clamp_hover]]clamp(v, vec3(), vec3())
local globalCross = glm.--[[@glm_cross_hover]]cross(q, v)
local namespace = glm.--[[@no_glm_vector]]vec
local q = quat()
local vectorQuatCross = v:cross(q)
local quatVectorCross = q:cross(--[[@quat_vector_cross_signature]])
local quatQuatCross = q:cross(q)
local vector2Cross = vec2():cross(vec2())
`)
	h.reindex()

	clampHelp := h.signatureHelp("vector_clamp_signature")
	if clampHelp == nil || len(clampHelp.Signatures) == 0 {
		t.Fatal("Vector3:clamp signature help is empty")
	}
	var clampHasVectorBounds bool
	for _, signature := range clampHelp.Signatures {
		clampHasVectorBounds = clampHasVectorBounds || strings.Contains(signature.Label, "min: number|Vector3") && strings.Contains(signature.Label, "max: number|Vector3")
	}
	if !clampHasVectorBounds {
		t.Fatalf("Vector3:clamp signatures = %#v, want same-shape vector bounds", clampHelp.Signatures)
	}
	clampHover := h.hover("glm_clamp_hover")
	if clampHover == nil || !strings.Contains(clampHover.Contents.Value, "Vector3, max: Vector3") {
		t.Fatalf("glm.clamp hover = %#v, want same-shape vector bound overloads", clampHover)
	}
	crossHover := h.hover("glm_cross_hover")
	if crossHover == nil || !strings.Contains(crossHover.Contents.Value, "Quat, b: Vector3") || !strings.Contains(crossHover.Contents.Value, "Quat, b: Quat") {
		t.Fatalf("glm.cross hover = %#v, want Quat/Vector3 and Quat/Quat forms", crossHover)
	}
	quatCrossHelp := h.signatureHelp("quat_vector_cross_signature")
	if quatCrossHelp == nil || len(quatCrossHelp.Signatures) == 0 {
		t.Fatalf("Quat:cross signatures = %#v, want quat/Vector3 operands", quatCrossHelp)
	}
	var quatCrossHasVector3 bool
	for _, signature := range quatCrossHelp.Signatures {
		quatCrossHasVector3 = quatCrossHasVector3 || strings.Contains(signature.Label, "Vector3")
	}
	if !quatCrossHasVector3 {
		t.Fatalf("Quat:cross signatures = %#v, want Vector3 operand and result union", quatCrossHelp.Signatures)
	}

	vecHelp := h.signatureHelp("glm_vec_variadic_signature")
	if vecHelp == nil || len(vecHelp.Signatures) == 0 {
		t.Fatal("glm.vec signature help is empty")
	}
	var vecHasThreeArgs bool
	for _, signature := range vecHelp.Signatures {
		vecHasThreeArgs = vecHasThreeArgs || strings.Count(signature.Label, "number") >= 3
	}
	if !vecHasThreeArgs {
		t.Fatalf("glm.vec signatures = %#v, want variadic dimension overloads", vecHelp.Signatures)
	}
	globalVecHelp := h.signatureHelp("global_vec_variadic_signature")
	if globalVecHelp == nil || len(globalVecHelp.Signatures) == 0 {
		t.Fatal("global vec signature help is empty")
	}
	var globalVecHasThreeArgs bool
	for _, signature := range globalVecHelp.Signatures {
		globalVecHasThreeArgs = globalVecHasThreeArgs || strings.Count(signature.Label, "number") >= 3
	}
	if !globalVecHasThreeArgs {
		t.Fatalf("global vec signatures = %#v, want variadic dimension overloads", globalVecHelp.Signatures)
	}

	labels := labelsFromItems(h.completion("no_glm_vector").Items)
	if slices.Contains(labels, "vector") {
		t.Fatalf("glm namespace completion includes nonexistent glm.vector: %#v", labels)
	}
	doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/main.lua")]
	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "vectorQuatCross", want: "Vector3"},
		{name: "quatQuatCross", want: "Quat"},
	} {
		got := doc.InferType(findIdentByOccurrence(t, doc.Tree, tc.name, 0))
		if got.CustomName != tc.want {
			t.Errorf("%s inferred as %#v, want %s", tc.name, got, tc.want)
		}
	}
	if got := doc.InferType(findIdentByOccurrence(t, doc.Tree, "vector2Cross", 0)); got.Basics != TypeNumber {
		t.Errorf("Vector2:cross inferred as %#v, want number", got)
	}
}
