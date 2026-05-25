package lsp

import (
	"testing"
)

func TestFiveMManifestDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarness(t, "manifest_authoring")

	diags := h.diagnostics("authoring_resource/fxmanifest.lua")
	for _, tc := range []struct {
		marker string
		code   string
	}{
		{marker: "manifest_reserved", code: "fivem-manifest-reserved-directive"},
		{marker: "manifest_invalid_local", code: "fivem-manifest-invalid-construct"},
		{marker: "manifest_unknown_runtime", code: "fivem-manifest-unknown-directive"},
	} {
		if !hasDiagnosticAtMarker(h, diags, tc.marker, tc.code) {
			t.Fatalf("manifest diagnostics missing %s at %s: %+v", tc.code, tc.marker, diags)
		}
	}

	if hasAnyDiagnosticAtMarker(h, diags, "manifest_custom_metadata") {
		t.Fatalf("custom manifest metadata should not be diagnosed: %+v", diags)
	}
}

func hasDiagnosticAtMarker(h *fiveMFixtureHarness, diags []Diagnostic, markerName, code string) bool {
	marker := h.requireMarker(markerName)

	for _, diag := range diags {
		if diag.Code == code && positionInRange(marker.Position, diag.Range) {
			return true
		}
	}

	return false
}

func hasAnyDiagnosticAtMarker(h *fiveMFixtureHarness, diags []Diagnostic, markerName string) bool {
	marker := h.requireMarker(markerName)

	for _, diag := range diags {
		if positionInRange(marker.Position, diag.Range) {
			return true
		}
	}

	return false
}

func positionInRange(pos Position, r Range) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}

	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}

	if pos.Line == r.End.Line && pos.Character >= r.End.Character {
		return false
	}

	return true
}
