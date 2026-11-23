package lsp

import (
	"strings"
	"testing"
)

func TestFiveMPerformanceAdviceIsExplicitAndConservative(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		code     string
		contains []string
	}{
		{"known SQL migration", `MySQL.Sync.fetchAll('select 1')`, "fivem-performance-sync-sql", []string{"MySQL.Async.fetchAll", "callback"}},
		{"loop yield", `while true do work() end`, "fivem-performance-no-yield", []string{"Wait(delay)", "each repeating path"}},
		{"wait zero pacing", `while true do Wait(0) end`, "fivem-performance-wait-zero", []string{"positive Wait", "adaptive delay"}},
		{"search validity", `while true do GetGamePool('CPed'); Wait(500) end`, "fivem-performance-repeated-lookup", []string{"remains valid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, root := newFiveMProfileTestServer(t)
			doc := addFiveMTestDocument(t, s, root+"/client.lua", tt.source)
			var found bool
			for _, d := range s.buildFiveMPerformanceDiagnostics(doc) {
				if d.Code != tt.code {
					continue
				}
				found = true
				for _, phrase := range tt.contains {
					if !strings.Contains(d.Message, phrase) {
						t.Errorf("message %q does not contain %q", d.Message, phrase)
					}
				}
			}
			if !found {
				t.Fatalf("missing %s diagnostic", tt.code)
			}
		})
	}
}

func TestFiveMAsyncAlternativeRequiresKnownEquivalent(t *testing.T) {
	if got := fiveMSQLAsyncAlternative("MySQL.Sync.fetchAll"); got != "MySQL.Async.fetchAll" {
		t.Fatalf("recognized alternative = %q", got)
	}
	if got := fiveMSQLAsyncAlternative("custom.Sync.query"); got != "" {
		t.Fatalf("guessed alternative = %q", got)
	}
	if got := performanceSQLAdvice("custom.Sync.query"); !strings.Contains(got, "only after confirming") {
		t.Fatalf("unknown adapter advice = %q", got)
	}
}

func TestFiveMManyHandlersAdviceMentionsUnknownMutation(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	source := strings.Repeat("RegisterNetEvent('event')\n", 11)
	doc := addFiveMTestDocument(t, s, root+"/client.lua", source)
	for _, d := range s.buildFiveMPerformanceDiagnostics(doc) {
		if d.Code == "fivem-performance-many-handlers" {
			if !strings.Contains(d.Message, "cannot determine") || !strings.Contains(d.Message, "shared state") {
				t.Fatalf("handler advice = %q", d.Message)
			}
			return
		}
	}
	t.Fatal("missing handler-count diagnostic")
}
