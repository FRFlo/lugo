package lsp

import (
	"strings"
	"testing"
)

func TestBoundedNUIFailureCount(t *testing.T) {
	if got := boundedNUIFailureCount(0); got != 1 {
		t.Fatalf("first failure count = %d, want 1", got)
	}
	if got := boundedNUIFailureCount(999); got != 1000 {
		t.Fatalf("failure count = %d, want cap 1000", got)
	}
	if got := boundedNUIFailureCount(1000); got != 1000 {
		t.Fatalf("failure count exceeded cap: %d", got)
	}
}

func TestNUITraversalFailureTelemetryIsAnonymous(t *testing.T) {
	oldTelemetry := globalTelemetry
	t.Cleanup(func() { globalTelemetry = oldTelemetry })
	journal := &traceJournal{max: 4096}
	globalTelemetry = &Telemetry{journal: journal}

	h := newFiveMFixtureHarnessWithoutIndex(t)
	privatePath := h.root + "/private-resource-missing"
	doc := &Document{Server: h.server, Path: privatePath + "/client.lua", URI: h.server.pathToURI(privatePath + "/client.lua")}
	doc.FiveMProfile = FiveMExecutionProfile{ResourceRoot: privatePath}
	doc.FiveMProfileCached = true
	h.server.nuiResourceFiles(doc)

	journal.mu.Lock()
	var output strings.Builder
	for _, entry := range journal.entries {
		output.Write(entry.Data)
	}
	journal.mu.Unlock()
	data := output.String()
	if !strings.Contains(data, "lugo.fivem_nui_scan.failure") || !strings.Contains(data, `"traversal_failures":"1"`) {
		t.Fatalf("missing traversal failure telemetry: %s", data)
	}
	if strings.Contains(data, privatePath) || strings.Contains(data, "no such file") || strings.Contains(data, "client.lua") {
		t.Fatalf("failure telemetry leaked filesystem details: %s", data)
	}
}
