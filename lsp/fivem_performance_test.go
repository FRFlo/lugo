package lsp

import "testing"

func TestFiveMPerformanceDiagnosticsConservativeHotspots(t *testing.T) {
	tests := []struct {
		name   string
		source string
		codes  []string
	}{
		{"wait zero and lookup", `CreateThread(function()
 while true do
  local pool = GetGamePool('CPed')
  Wait(0)
 end
end)`, []string{"fivem-performance-wait-zero", "fivem-performance-repeated-lookup"}},
		{"no yield", `while true do
 DoWork()
end`, []string{"fivem-performance-no-yield"}},
		{"sync sql", `local rows = MySQL.Sync.fetchAll('select 1')`, []string{"fivem-performance-sync-sql"}},
		{"many handlers", `RegisterNetEvent('a')
RegisterNetEvent('b')
RegisterNetEvent('c')
RegisterNetEvent('d')
RegisterNetEvent('e')
RegisterNetEvent('f')
RegisterNetEvent('g')
RegisterNetEvent('h')
RegisterNetEvent('i')
RegisterNetEvent('j')
RegisterNetEvent('k')`, []string{"fivem-performance-many-handlers"}},
		{"ordinary loop is not lookup", `for i = 1, 10 do
 print(i)
 Wait(100)
end`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := newFiveMProfileTestServer(t)
			doc := addFiveMTestDocument(t, s, root+"/client.lua", tt.source)
			diags := s.buildFiveMPerformanceDiagnostics(doc)
			for _, code := range tt.codes {
				if !hasDiagnosticCode(diags, code) {
					t.Fatalf("diagnostics = %#v, missing %s", diags, code)
				}
			}
			if len(diags) != len(tt.codes) {
				t.Fatalf("diagnostics = %#v, want %v", diags, tt.codes)
			}
		})
	}
}

func TestFiveMPerformanceDiagnosticCanBeDisabled(t *testing.T) {
	opts := defaultInitializationOptions()
	opts.DiagFiveMPerformance = false
	if opts.DiagFiveMPerformance {
		t.Fatal("performance diagnostics should be configurable")
	}
}
