package lsp

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coalaura/plain"
)

func TestMCPRequestMissingResponseReturnsStableErrorAndTelemetry(t *testing.T) {
	oldTelemetry := globalTelemetry
	journal := &traceJournal{max: defaultJournalBytes}
	globalTelemetry = &Telemetry{journal: journal}
	t.Cleanup(func() { globalTelemetry = oldTelemetry })

	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	traceCtx := WithTraceContext(context.Background(), TraceContext{TraceID: "trace-missing-response", SpanID: "parent"})
	_, err := s.MCPRequestContext(traceCtx, "$/cancelRequest", json.RawMessage(`{}`))
	if err == nil || err.Error() != "LSP response missing" {
		t.Fatalf("missing response error = %v, want stable error", err)
	}

	found := false
	for _, entry := range journal.entries {
		var event struct {
			Event      string         `json:"event"`
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(entry.Data, &event); err != nil || event.Event != "lugo.mcp.lsp_request.missing_response" {
			continue
		}
		found = true
		if event.Properties["boundary"] != "embedded_mcp" || event.Properties["method_class"] != "protocol" || event.Properties["trace_id"] != "trace-missing-response" {
			t.Fatalf("missing response telemetry = %#v", event.Properties)
		}
		if _, exists := event.Properties["method"]; exists {
			t.Fatalf("telemetry leaked raw method: %#v", event.Properties)
		}
	}
	if !found {
		t.Fatal("missing correlated missing-response telemetry")
	}
}

func TestMCPRequestRecoversPanicAndRemainsHealthy(t *testing.T) {
	oldTelemetry := globalTelemetry
	journal := &traceJournal{max: defaultJournalBytes}
	globalTelemetry = &Telemetry{journal: journal}
	t.Cleanup(func() { globalTelemetry = oldTelemetry })

	s := NewServer("test")
	s.Log = plain.New(plain.WithTarget(io.Discard))
	uri := s.pathToURI(filepath.Join(t.TempDir(), "panic.lua"))
	uri = s.normalizeURI(uri)
	// handleHover dereferences this entry before its nil guard, exercising the
	// embedded dispatch recovery boundary without adding a production test hook.
	s.Documents[uri] = nil
	params := json.RawMessage(`{"textDocument":{"uri":"` + uri + `"},"position":{"line":0,"character":0}}`)
	traceCtx := WithTraceContext(context.Background(), TraceContext{TraceID: "trace-for-recovery", SpanID: "parent"})

	_, err := s.MCPRequestContext(traceCtx, "textDocument/hover", params)
	if err == nil || err.Error() != "LSP -32603: internal error" {
		t.Fatalf("panic response = %v, want stable internal error", err)
	}
	if strings.Contains(err.Error(), "panic.lua") {
		t.Fatalf("panic response leaked request data: %v", err)
	}

	delete(s.Documents, uri)
	result, err := s.MCPRequestContext(traceCtx, "textDocument/hover", params)
	if err != nil || string(result) != "null" {
		t.Fatalf("subsequent request = %s, %v; want null, nil", result, err)
	}

	found := false
	for _, entry := range journal.entries {
		var event struct {
			Event      string         `json:"event"`
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(entry.Data, &event); err != nil || event.Event != "lsp_request_panic_recovered" {
			continue
		}
		found = true
		if event.Properties["boundary"] != "embedded_mcp" || event.Properties["method_class"] != "text_document" || event.Properties["panic_type"] == "" {
			t.Fatalf("panic telemetry properties = %#v", event.Properties)
		}
		if event.Properties["trace_id"] != "trace-for-recovery" {
			t.Fatalf("panic telemetry trace ID = %#v", event.Properties["trace_id"])
		}
	}
	if !found {
		t.Fatal("missing recovered MCP panic telemetry")
	}
}
