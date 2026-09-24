package lsp

import (
	"strings"
	"testing"
)

type telemetryPanicWriter struct{}

func (telemetryPanicWriter) Write([]byte) (int, error) {
	panic("private workspace panic")
}

func TestDiagnosticTelemetryPropertiesAreAggregateOnly(t *testing.T) {
	properties := diagnosticTelemetryProperties([]Diagnostic{
		{Severity: SeverityError, Code: "parse-error"},
		{Severity: SeverityWarning, Code: "fivem-unknown-event"},
		{Severity: SeverityInformation, Code: "undefined-global"},
		{Severity: SeverityHint, Code: "unused-local"},
	})

	for key, want := range map[string]int{
		"total": 4, "error_count": 1, "warning_count": 1, "info_count": 1, "hint_count": 1,
		"parse_count": 1, "fivem_count": 1, "lua_count": 2,
	} {
		if got := properties[key]; got != want {
			t.Errorf("%s = %v, want %d", key, got, want)
		}
	}
}

func TestRefreshWorkspaceRecordsAnonymousPanicFinish(t *testing.T) {
	oldTelemetry := globalTelemetry
	t.Cleanup(func() { globalTelemetry = oldTelemetry })
	journal := &traceJournal{max: 4096}
	globalTelemetry = &Telemetry{journal: journal}

	h := newFiveMFixtureHarnessWithoutIndex(t)
	s := h.server
	s.Writer = telemetryPanicWriter{}
	s.workDoneProgressSupport = true
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("refreshWorkspace did not preserve its panic")
			}
		}()
		s.refreshWorkspace()
	}()

	journal.mu.Lock()
	data := string(journal.entries[len(journal.entries)-1].Data)
	journal.mu.Unlock()
	if !strings.Contains(data, "lugo.workspace_index.finish") || !strings.Contains(data, `"outcome":"panic"`) || !strings.Contains(data, "duration_ms") {
		t.Fatalf("panic finish telemetry = %s", data)
	}
	if strings.Contains(data, "private workspace panic") {
		t.Fatalf("panic telemetry leaked recovered value: %s", data)
	}
}

func TestWorkspaceAndDiagnosticTelemetryIsBoundedAndAnonymous(t *testing.T) {
	oldTelemetry := globalTelemetry
	t.Cleanup(func() { globalTelemetry = oldTelemetry })
	journal := &traceJournal{max: 16 << 10}
	globalTelemetry = &Telemetry{journal: journal}

	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("private-super-secret.lua", "function broken(\n")
	h.server.refreshWorkspace()

	journal.mu.Lock()
	var entries strings.Builder
	for _, entry := range journal.entries {
		entries.Write(entry.Data)
	}
	journal.mu.Unlock()
	output := entries.String()

	for _, required := range []string{
		"lugo.workspace_index.start",
		"lugo.workspace_index.finish",
		"lugo.diagnostics_workspace.start",
		"lugo.diagnostics_workspace.finish",
		"lugo.diagnostics_document.finish",
		"indexed", "unchanged", "failed", "bytes_bucket", "duration_ms",
		"error_count", "warning_count", "parse_count", "fivem_count", "lua_count",
	} {
		if !strings.Contains(output, required) {
			t.Errorf("telemetry missing %q: %s", required, output)
		}
	}
	if strings.Contains(output, "private-super-secret.lua") || strings.Contains(output, h.root) || strings.Contains(output, "function broken") {
		t.Fatalf("telemetry leaked workspace data: %s", output)
	}
}
