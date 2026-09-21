package lsp

import (
	"testing"
)

func TestFiveMEntityLifecycleDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		codes  []string
	}{
		{"use before existence check", `local entity = NetworkGetEntityFromNetworkId(42)
NetworkRequestControlOfEntity(entity)
`, []string{"fivem-entity-use-before-existence-check", "fivem-entity-missing-cleanup"}},
		{"checked and deleted", `local entity = NetworkGetEntityFromNetworkId(42)
if DoesEntityExist(entity) then
 NetworkRequestControlOfEntity(entity)
 DeleteEntity(entity)
end
`, nil},
		{"missing cleanup", `local entity = NetworkGetEntityFromNetworkId(42)
if DoesEntityExist(entity) then
 print(entity)
end
`, []string{"fivem-entity-missing-cleanup"}},
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
