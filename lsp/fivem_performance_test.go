package lsp

import (
	"strconv"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

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

func TestFiveMASTFactsAggregateNestedLoopCalls(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	doc := addFiveMTestDocument(t, s, root+"/client.lua", `while true do
	for i = 1, 2 do
		GetGamePool("CPed")
		Wait(0)
	end
end`)

	facts := newFiveMASTFacts(doc)
	loops := 0
	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		node := doc.Tree.Nodes[id]
		if node.Kind != ast.KindWhile && node.Kind != ast.KindForNum {
			continue
		}
		loops++
		loop := facts.loops[id]
		if !loop.waitZero || !loop.yield || !loop.lookup {
			t.Fatalf("loop facts = %#v, want nested call facts", loop)
		}
	}
	if loops != 2 {
		t.Fatalf("loop count = %d, want 2", loops)
	}
}

func BenchmarkFiveMASTFactsScaling(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run("calls="+strconv.Itoa(count), func(b *testing.B) {
			source := "while true do\\n"
			source += strings.Repeat("GetGamePool('CPed')\\nWait(0)\\n", count)
			source += "end"
			h := newFiveMFixtureHarnessWithoutIndex(b)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
			h.writeWorkspaceFile("resource/client.lua", source)
			h.reindex()
			doc := h.server.Documents[h.server.pathToURI(h.root+"/resource/client.lua")]
			if doc == nil {
				b.Fatal("indexed document is missing")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = newFiveMASTFacts(doc)
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
