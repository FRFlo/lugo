package lsp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIPolicyFiltersCodesAndSeverityForFailure(t *testing.T) {
	var output bytes.Buffer
	s := &Server{Writer: &output}
	setCIPolicy(s, CIPolicy{FailOnSeverity: "warning", ExcludeCodes: []string{"style"}, MaxWarnings: 1})

	s.printCIDiagnostics("file:///workspace/main.lua", []Diagnostic{
		{Severity: SeverityInformation, Code: "info", Message: "informational"},
		{Severity: SeverityWarning, Code: "style", Message: "ignored style"},
		{Severity: SeverityWarning, Code: "lint", Message: "counts"},
		{Severity: SeverityWarning, Code: "lint", Message: "exceeds budget"},
	})

	state := getCIState(s)
	if state.failures != 2 {
		t.Fatalf("failures = %d, want 2 (the two warnings meet the failure threshold)", state.failures)
	}
	if strings.Contains(output.String(), "ignored style") {
		t.Fatal("excluded diagnostic was emitted")
	}
}

func TestCISARIFUsesUnsignedDiagnosticPositions(t *testing.T) {
	dir := t.TempDir()
	s := &Server{Writer: io.Discard}
	setCIPolicy(s, CIPolicy{SARIFPath: "report.sarif"})
	s.printCIDiagnostics("file:///workspace/main.lua", []Diagnostic{{
		Severity: SeverityError, Code: "bad", Message: "broken",
		Range: Range{Start: Position{Line: 4, Character: 2}},
	}})
	if err := s.writeCISARIF(dir + "/ci.json"); err != nil {
		t.Fatalf("writeCISARIF() error = %v", err)
	}
	if _, err := os.Stat(dir + "/report.sarif"); err != nil {
		t.Fatalf("SARIF report was not written: %v", err)
	}
}

func TestCISARIFFailureReturnsErrorAndRecordsSafeTelemetry(t *testing.T) {
	oldTelemetry := globalTelemetry
	t.Cleanup(func() { globalTelemetry = oldTelemetry })
	journal := &traceJournal{max: 4096}
	globalTelemetry = &Telemetry{Enabled: true, journal: journal}

	dir := t.TempDir()
	s := &Server{Writer: io.Discard}
	setCIPolicy(s, CIPolicy{SARIFPath: "."})
	if err := s.writeCISARIF(filepath.Join(dir, "ci.json")); err == nil {
		t.Fatal("writeCISARIF() error = nil, want write failure")
	}

	journal.mu.Lock()
	data := string(journal.entries[len(journal.entries)-1].Data)
	journal.mu.Unlock()
	if !strings.Contains(data, "lugo.ci.sarif_failure") || strings.Contains(data, dir) {
		t.Fatalf("SARIF failure telemetry was missing or unsafe: %s", data)
	}
}

func TestRunCIFailsWhenSARIFCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ci.json")
	if err := os.WriteFile(configPath, []byte(`{"ciPolicy":{"sarifPath":"."}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s := NewServer("test-version")
	s.Writer = io.Discard
	if got := s.RunCI(configPath); got != 1 {
		t.Fatalf("RunCI() = %d, want 1 after SARIF write failure", got)
	}
}

func TestCIGitHubAnnotationEscapesControlAndDelimiterCharacters(t *testing.T) {
	var output bytes.Buffer
	s := &Server{Writer: &output}
	setCIPolicy(s, CIPolicy{})
	s.printCIDiagnostics("file:///a,b:c", []Diagnostic{{
		Severity: SeverityError,
		Code:     "bad",
		Message:  "100% failed,\nnext\rline",
	}})

	path := s.uriToPath("file:///a,b:c")
	want := fmt.Sprintf("::error file=%s,line=1,col=1::100%%25 failed, next%%0Dline", ciEscape(path, true))
	if !strings.Contains(output.String(), want) || strings.Contains(output.String(), "\nnext") {
		t.Fatalf("annotation was not safely escaped: %q", output.String())
	}
}
