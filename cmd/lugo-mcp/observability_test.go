package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPTraceIDsCorrelateNestedRequests(t *testing.T) {
	s := &server{observability: observability{enabled: true}}
	ctx, parent := s.trace(context.Background())
	child, childID := s.trace(ctx)
	got, ok := child.Value(traceContextKey{}).(traceContext)
	if !ok || got.id != childID || got.parent != parent {
		t.Fatalf("trace correlation = %#v, want child %q parent %q", got, childID, parent)
	}
}

func TestMCPBoundaryLoggingIsBoundedAndRedacted(t *testing.T) {
	s := &server{observability: observability{enabled: true}}
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	ctx, _ := s.trace(context.Background())
	s.logBoundary(ctx, "tool", "lugo_hover", "error", errors.New("secret/path.lua"))
	if strings.Contains(output.String(), "secret/path.lua") || strings.Contains(output.String(), "arguments") {
		t.Fatalf("boundary log leaked sensitive context: %s", output.String())
	}
	if len(output.Bytes()) > 512 {
		t.Fatalf("boundary log is not bounded: %d bytes", output.Len())
	}
}

func TestMCPOptOut(t *testing.T) {
	for _, value := range []string{"0", "false", "off", "no"} {
		t.Setenv("LUGO_MCP_TELEMETRY", value)
		if !telemetryOptedOut() {
			t.Fatalf("telemetry value %q was not disabled", value)
		}
	}
	os.Unsetenv("LUGO_MCP_TELEMETRY")
	if telemetryOptedOut() {
		t.Fatal("telemetry unexpectedly disabled by default")
	}
}

func TestMCPResourceBoundaryConvertsPanicToError(t *testing.T) {
	s := &server{observability: observability{enabled: false}}
	handler := s.resource(func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		panic("should not escape")
	}, "test")
	_, err := handler(context.Background(), &mcp.ReadResourceRequest{})
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("panic error = %v", err)
	}
}
