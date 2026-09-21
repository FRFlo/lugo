package lsp

import (
	"bytes"
	"io"
	"os"
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
	s.writeCISARIF(dir + "/ci.json")
	if _, err := os.Stat(dir + "/report.sarif"); err != nil {
		t.Fatalf("SARIF report was not written: %v", err)
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

	want := "::error file=a%2Cb%3Ac,line=1,col=1::100%25 failed, next"
	if !strings.Contains(output.String(), want) || strings.Contains(output.String(), "\nnext") {
		t.Fatalf("annotation was not safely escaped: %q", output.String())
	}
}
