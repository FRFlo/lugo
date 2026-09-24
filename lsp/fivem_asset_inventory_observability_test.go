package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFiveMAssetInventoryFilesystemFailureTelemetryIsBoundedAndPrivate(t *testing.T) {
	oldTelemetry := globalTelemetry
	journal := &traceJournal{max: 1 << 16}
	globalTelemetry = &Telemetry{Enabled: true, journal: journal}
	t.Cleanup(func() { globalTelemetry = oldTelemetry })

	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\nui_page 'web/index.html'\n")
	h.reindex()
	// Removing the resource root makes WalkDir report a filesystem failure while
	// leaving the manifest document available for the diagnostic pass.
	if err := os.RemoveAll(filepath.Join(h.root, "resource")); err != nil {
		t.Fatal(err)
	}
	_ = h.diagnostics("resource/fxmanifest.lua")

	journal.mu.Lock()
	defer journal.mu.Unlock()
	var recorded string
	for _, entry := range journal.entries {
		if strings.Contains(string(entry.Data), "lugo.fivem.asset_inventory.filesystem_failure") {
			recorded += string(entry.Data)
		}
	}
	if recorded == "" {
		t.Fatal("missing filesystem failure telemetry")
	}
	for _, sensitive := range []string{"resource", "fxmanifest.lua", "web/index.html", "no such file", "error"} {
		if strings.Contains(strings.ToLower(recorded), sensitive) {
			t.Errorf("telemetry contains sensitive/raw detail %q: %s", sensitive, recorded)
		}
	}
	for _, expected := range []string{`"failure_count":"1"`, `"not_found_count":"1"`, `"degraded":"true"`} {
		if !strings.Contains(recorded, expected) {
			t.Errorf("telemetry missing %s: %s", expected, recorded)
		}
	}
}
