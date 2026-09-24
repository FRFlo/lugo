package lsp

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/posthog/posthog-go"
)

func TestTelemetryOptOut(t *testing.T) {
	old := globalTelemetry
	defer func() { globalTelemetry = old }()
	t.Setenv("LUGO_TELEMETRY", "false")
	t.Setenv("LUGO_TRACE_JOURNAL", t.TempDir()+"/trace.ndjson")
	t.Setenv("LUGO_POSTHOG_TOKEN", "test")
	got, err := InitTelemetry("test")
	if err != nil || got.Enabled || got.PHClient != nil || got.journal != nil {
		t.Fatalf("opt-out retained telemetry state: %#v, %v", got, err)
	}
	got.Close()
}

func TestTelemetryLocalOnlyKeepsJournalWithoutRemoteClient(t *testing.T) {
	old := globalTelemetry
	t.Cleanup(func() { globalTelemetry = old })
	path := t.TempDir() + "/trace.ndjson"
	t.Setenv("LUGO_TELEMETRY", "true")
	t.Setenv("LUGO_TELEMETRY_LOCAL_ONLY", "true")
	t.Setenv("LUGO_TRACE_JOURNAL", path)
	got, err := InitTelemetry("test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.PHClient != nil || got.journal == nil {
		t.Fatalf("local-only telemetry state = %#v", got)
	}
	RecordTelemetry(context.Background(), "local.test", map[string]any{"status": "ok"})
	if err := got.Flush(); err != nil {
		t.Fatal(err)
	}
	got.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "local.test") {
		t.Fatalf("local journal missed event: %s", data)
	}
}

func TestRedactTelemetryText(t *testing.T) {
	got := RedactTelemetryText("token=secret password: hunter2 file:///home/user/project/main.lua textDocument/hover")
	if strings.Contains(got, "secret") || strings.Contains(got, "hunter2") || strings.Contains(got, "main.lua") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redaction failed: %q", got)
	}
}

func TestTraceJournalBounded(t *testing.T) {
	j := &traceJournal{max: 32}
	for i := 0; i < 20; i++ {
		j.append(map[string]string{"value": "012345678901234567890"})
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.bytes > j.max {
		t.Fatalf("journal is %d bytes, limit %d", j.bytes, j.max)
	}
}

func TestTraceJournalFailure(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "journal")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	j := &traceJournal{file: f, max: 128}
	j.append(map[string]string{"event": "failure"})
	if j.err == nil {
		t.Fatal("write failure was not recorded")
	}
	if err := j.close(); err == nil {
		t.Fatal("close should report journal failure")
	}
}

func TestTraceJournalPersistentFileRemainsBounded(t *testing.T) {
	path := t.TempDir() + "/trace.ndjson"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	j := &traceJournal{file: f, max: 128}
	for i := 0; i < 20; i++ {
		j.append(map[string]string{"event": "bounded", "value": strings.Repeat("x", 32)})
	}
	if err := j.close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > int64(j.max) {
		t.Fatalf("persistent journal is %d bytes, limit %d", info.Size(), j.max)
	}
}

func TestTelemetryCloseAndFlushAreSafe(t *testing.T) {
	tel := &Telemetry{}
	if err := tel.Flush(); err != nil {
		t.Fatal(err)
	}
	tel.Close()
	tel.Close()
	if err := tel.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestPanicContextIsJournalled(t *testing.T) {
	old := globalTelemetry
	defer func() { globalTelemetry = old }()
	j := &traceJournal{max: 4096}
	globalTelemetry = &Telemetry{Enabled: true, journal: j}
	ctx, tc := StartTrace(context.Background())
	CapturePanicContext(ctx, "boom", "test")
	j.mu.Lock()
	data := string(j.entries[0].Data)
	j.mu.Unlock()
	if !strings.Contains(data, tc.TraceID) || !strings.Contains(data, tc.SpanID) {
		t.Fatalf("panic lost context: %s", data)
	}
}

func TestInitializationTraceContextIsInheritedByRequests(t *testing.T) {
	s := NewServer("test")
	s.applyInitializationOptions(InitializationOptions{
		TelemetryEnabled: true,
		TelemetryTraceID: "extension-trace",
		TelemetrySpanID:  "extension-span",
	})
	if s.trace.TraceID != "extension-trace" || s.trace.SpanID != "extension-span" {
		t.Fatalf("trace context = %#v", s.trace)
	}
}

func TestTelemetryDeliveryFailureIsAccountedWithoutRecursiveEvent(t *testing.T) {
	j := &traceJournal{max: 4096}
	tel := &Telemetry{Enabled: true, journal: j, enqueue: func(posthog.Message) error { return errors.New("queue full") }}
	tel.Record(context.Background(), "test.event", nil)
	if got := tel.DeliveryFailures(); got != 1 {
		t.Fatalf("delivery failures = %d, want 1", got)
	}
	j.mu.Lock()
	entries := len(j.entries)
	j.mu.Unlock()
	if entries != 1 {
		t.Fatalf("delivery failure recursively recorded %d journal entries", entries)
	}
}

func TestTelemetryOptOutPurgesJournal(t *testing.T) {
	path := t.TempDir() + "/trace.ndjson"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	j := &traceJournal{file: f, path: path, max: 4096}
	tel := &Telemetry{Enabled: true, journal: j}
	old := globalTelemetry
	globalTelemetry = tel
	defer func() { globalTelemetry = old }()
	tel.Record(context.Background(), "test.event", map[string]any{"value": "present"})
	SetTelemetryEnabled(false)
	if tel.Enabled || !tel.optedOut || len(j.entries) != 0 {
		t.Fatalf("opt-out did not purge telemetry: %#v", tel)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("journal remains after opt-out: %v", err)
	}
	tel.Record(context.Background(), "test.after_opt_out", nil)
	if len(j.entries) != 0 {
		t.Fatal("opt-out accepted a new local event")
	}
}

func TestCrashStackUsesSeparateBoundFromMetadata(t *testing.T) {
	stack := redactCrashStack("token=secret " + strings.Repeat("x", maxCrashStackBytes+1))
	if strings.Contains(stack, "secret") || len(stack) <= maxMetadataTextBytes || len(stack) > maxCrashStackBytes+len("…") {
		t.Fatalf("crash-stack redaction/bound failed: length=%d", len(stack))
	}
}

func TestCrashEnvelopeKeepsBoundedRecentContextAndStack(t *testing.T) {
	old := globalTelemetry
	defer func() { globalTelemetry = old }()
	j := &traceJournal{max: 1 << 20}
	tel := &Telemetry{Enabled: true, journal: j, build: processBuildMetadata("test")}
	globalTelemetry = tel
	for i := 0; i < maxRecentTraceActivities+5; i++ {
		tel.Record(WithTraceContext(context.Background(), TraceContext{TraceID: "trace" + string(rune('a'+i))}), "test.event", nil)
	}
	CapturePanicContext(context.Background(), "boom", "test")
	j.mu.Lock()
	data := string(j.entries[len(j.entries)-1].Data)
	j.mu.Unlock()
	if !strings.Contains(data, "recent_trace_context") || strings.Contains(data, "tracea") {
		t.Fatalf("crash context was not bounded: %s", data)
	}
	if !strings.Contains(data, "go_version") || !strings.Contains(data, "stack") {
		t.Fatalf("crash metadata missing: %s", data)
	}
}

func TestRecordTelemetryRedactsPathsAndBoundsValues(t *testing.T) {
	j := &traceJournal{max: 4096}
	tel := &Telemetry{Enabled: true, journal: j}
	ctx, tc := StartTrace(context.Background())
	tel.Record(ctx, "test.event", map[string]any{
		"path":  `C:\workspace\secret.lua`,
		"value": strings.Repeat("x", 2048),
	})
	j.mu.Lock()
	data := string(j.entries[0].Data)
	j.mu.Unlock()
	if strings.Contains(data, "secret.lua") || strings.Contains(data, strings.Repeat("x", 600)) {
		t.Fatalf("record leaked unbounded or sensitive data: %s", data)
	}
	if !strings.Contains(data, tc.TraceID) {
		t.Fatalf("record lost trace context: %s", data)
	}
}
