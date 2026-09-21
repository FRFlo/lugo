package lsp

import (
	"testing"
)

func TestFiveMEntityLifecycleDiagnosticsAreDeterministic(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	doc := addFiveMTestDocument(t, s, root+"/client.lua", `local first = NetworkGetEntityFromNetworkId(1)
local second = NetworkGetEntityFromNetworkId(2)
NetworkRequestControlOfEntity(second)
NetworkRequestControlOfEntity(first)
`)
	diags := assertFiveMDiagnosticsDeterministic(t, func() []Diagnostic {
		return s.buildFiveMEntityLifecycleDiagnostics(doc)
	})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %#v, want 2", diags)
	}
}

func TestFiveMEntityLifecycleDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		codes  []string
	}{
		{"control before existence check", `local entity = NetworkGetEntityFromNetworkId(42)
NetworkRequestControlOfEntity(entity)
`, []string{"fivem-entity-use-before-existence-check"}},
		{"checked and deleted", `local entity = NetworkGetEntityFromNetworkId(42)
if DoesEntityExist(entity) then
 NetworkRequestControlOfEntity(entity)
 DeleteEntity(entity)
end
`, nil},
		{"network entity does not require deletion", `local entity = NetworkGetEntityFromNetworkId(42)
if DoesEntityExist(entity) then
 print(entity)
end
`, nil},
		{"network entity lookup alone does not recommend deletion", `local entity = NetworkGetEntityFromNetworkId(42)
`, nil},
		{"dynamic network id ignored", `local entity = NetworkGetEntityFromNetworkId(netId)
NetworkRequestControlOfEntity(entity)
`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := newFiveMProfileTestServer(t)
			doc := addFiveMTestDocument(t, s, root+"/client.lua", tt.source)
			diags := s.buildFiveMEntityLifecycleDiagnostics(doc)
			for _, code := range tt.codes {
				if !hasDiagnosticCode(diags, code) {
					t.Fatalf("diagnostics = %#v, missing %s", diags, code)
				}
			}
			if len(diags) != len(tt.codes) {
				t.Fatalf("diagnostics = %#v, want codes %v", diags, tt.codes)
			}
		})
	}
}
