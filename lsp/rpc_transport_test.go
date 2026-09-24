package lsp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/posthog/posthog-go"
)

type failingRPCWriter struct{ err error }

func (w failingRPCWriter) Write([]byte) (int, error) { return 0, w.err }

type shortRPCWriter struct{}

func (shortRPCWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestWriteProtocolResponseRecordsWriteFailures(t *testing.T) {
	old := globalTelemetry
	defer func() { globalTelemetry = old }()

	for _, tt := range []struct {
		name       string
		writer     io.Writer
		shortWrite bool
	}{
		{name: "failing writer", writer: failingRPCWriter{errors.New("token=supersecret " + strings.Repeat("x", 1024))}},
		{name: "short writer", writer: shortRPCWriter{}, shortWrite: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			journal := &traceJournal{max: 4096}
			globalTelemetry = &Telemetry{Enabled: true, journal: journal}

			err := writeProtocolResponse(context.Background(), tt.writer, "parse", Response{RPC: "2.0", ID: nil})
			if err == nil {
				t.Fatal("writeProtocolResponse returned nil error")
			}
			if tt.shortWrite && !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("writeProtocolResponse error = %v, want io.ErrShortWrite", err)
			}

			journal.mu.Lock()
			if len(journal.entries) != 1 {
				journal.mu.Unlock()
				t.Fatalf("journal entries = %d, want 1", len(journal.entries))
			}
			entry := string(journal.entries[0].Data)
			journal.mu.Unlock()
			if !strings.Contains(entry, `"event":"lsp_transport_write_failed"`) || !strings.Contains(entry, `"response_path":"parse"`) {
				t.Fatalf("unexpected transport event: %s", entry)
			}
			if strings.Contains(entry, "supersecret") || strings.Contains(entry, strings.Repeat("x", 600)) {
				t.Fatalf("transport event was not redacted and bounded: %s", entry)
			}
		})
	}
}

func TestWriteProtocolResponseDoesNotPanicWhenTelemetryFails(t *testing.T) {
	old := globalTelemetry
	defer func() { globalTelemetry = old }()
	globalTelemetry = &Telemetry{
		Enabled: true,
		enqueue: func(posthog.Message) error {
			panic("telemetry unavailable")
		},
	}

	if err := writeProtocolResponse(context.Background(), failingRPCWriter{errors.New("write failed")}, "parse", Response{}); err == nil {
		t.Fatal("writeProtocolResponse returned nil error")
	}
}

func TestWriteMessageRecordsUnclassifiedFailures(t *testing.T) {
	old := globalTelemetry
	defer func() { globalTelemetry = old }()
	journal := &traceJournal{max: 4096}
	globalTelemetry = &Telemetry{journal: journal}

	err := WriteMessage(failingRPCWriter{errors.New("secret-path /home/alice/project/file.lua")}, Response{})
	if err == nil {
		t.Fatal("WriteMessage returned nil error")
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if len(journal.entries) != 1 {
		t.Fatalf("journal entries = %d, want 1", len(journal.entries))
	}
	entry := string(journal.entries[0].Data)
	if !strings.Contains(entry, `"event":"lsp_transport_write_failed"`) || !strings.Contains(entry, `"response_path":"unclassified"`) {
		t.Fatalf("missing generic write failure telemetry: %s", entry)
	}
	for _, sensitive := range []string{"secret-path", "/home/alice", "project", "file.lua"} {
		if strings.Contains(entry, sensitive) {
			t.Fatalf("write failure telemetry leaked %q: %s", sensitive, entry)
		}
	}
}

func TestProtocolResponsePathsRecordWriteFailures(t *testing.T) {
	for _, tt := range []struct {
		name  string
		body  string
		path  string
		setup func(*Server)
	}{
		{name: "parse", body: `{`, path: "parse"},
		{name: "invalid", body: `{"jsonrpc":"1.0","id":1,"method":"x"}`, path: "invalid"},
		{
			name: "cancelled", body: `{"jsonrpc":"2.0","id":1,"method":"x"}`, path: "cancel",
			setup: func(s *Server) { s.canceledRequests["1"] = struct{}{} },
		},
		{name: "unknown", body: `{"jsonrpc":"2.0","id":1,"method":"unknown/method"}`, path: "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			old := globalTelemetry
			defer func() { globalTelemetry = old }()
			journal := &traceJournal{max: 4096}
			globalTelemetry = &Telemetry{Enabled: true, journal: journal}

			s := NewServer("test")
			s.Reader = bufio.NewReader(strings.NewReader(rpcFrame(tt.body)))
			s.Writer = failingRPCWriter{errors.New("write failed")}
			if tt.setup != nil {
				tt.setup(s)
			}
			if err := s.Start(); err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			journal.mu.Lock()
			defer journal.mu.Unlock()
			for _, entry := range journal.entries {
				if strings.Contains(string(entry.Data), `"event":"lsp_transport_write_failed"`) && strings.Contains(string(entry.Data), `"response_path":"`+tt.path+`"`) {
					return
				}
			}
			t.Fatalf("no write failure event for %q: %#v", tt.path, journal.entries)
		})
	}
}

func rpcFrame(body string) string {
	return "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
}
