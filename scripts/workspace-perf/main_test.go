package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTelemetryJournalPathAndWorkspaceGuard(t *testing.T) {
	workspace := t.TempDir()
	journal := filepath.Join(workspace, "trace.ndjson")
	t.Setenv("LUGO_TRACE_JOURNAL", journal)

	if got := telemetryJournalPath(); got != journal {
		t.Fatalf("telemetryJournalPath() = %q, want %q", got, journal)
	}
	if err := ensureOutsideWorkspace(workspace, telemetryJournalPath()); err == nil {
		t.Fatal("journal inside workspace was accepted")
	}

	outside := filepath.Join(t.TempDir(), "trace.ndjson")
	t.Setenv("LUGO_TRACE_JOURNAL", outside)
	if err := ensureOutsideWorkspace(workspace, telemetryJournalPath()); err != nil {
		t.Fatalf("journal outside workspace rejected: %v", err)
	}
}

func TestEnsureOutsideWorkspaceResolvesSymlinkedOutputParents(t *testing.T) {
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "reports")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	link := filepath.Join(outside, "workspace-link")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(link, "nested", "report.sarif")
	if err := ensureOutsideWorkspace(workspace, output); err == nil {
		t.Fatal("output through symlink into workspace was accepted")
	}
}
