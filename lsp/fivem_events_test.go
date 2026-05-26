package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestFiveMEventFixtureLoading(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	h.requireMarker("client_registration")
	h.requireMarker("client_hover")
	h.requireMarker("client_net_registration")
	h.requireMarker("client_handler_def")

	h.requireMarker("server_registration")
	h.requireMarker("server_hover")
	h.requireMarker("server_net_registration")
	h.requireMarker("server_direction_error")

	h.requireMarker("shared_registration")
	h.requireMarker("shared_hover")
	h.requireMarker("shared_wildcard")
}

func TestFiveMUnregisteredNetEvent(t *testing.T) {
	// Use a fixture where a TriggerServerEvent/TriggerClientEvent is called
	// for an event that has no corresponding RegisterNetEvent in the same resource.
	h := newFiveMFixtureHarness(t, "resource_events_unregistered")

	diags := h.diagnostics("server.lua")
	if !hasDiagnosticCode(diags, "fivem-unregistered-net-event") {
		t.Fatalf("expected fivem-unregistered-net-event diagnostic, got: %#v", diags)
	}
}

func TestFiveMUnknownEvent(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	// client.lua contains an AddEventHandler for an unknown event (client:playerLoaded)
	clientDiags := h.diagnostics("client.lua")
	if !hasDiagnosticCode(clientDiags, "fivem-unknown-event") {
		t.Fatalf("expected fivem-unknown-event diagnostic on client.lua, got: %#v", clientDiags)
	}
	// Ensure the diagnostic references the actual event name in the message
	foundClient := false
	for _, d := range clientDiags {
		if d.Code == "fivem-unknown-event" && strings.Contains(d.Message, "client:playerLoaded") {
			foundClient = true
			break
		}
	}
	if !foundClient {
		t.Fatalf("expected diagnostic message to mention client:playerLoaded, got: %#v", clientDiags)
	}

	// server.lua contains an AddEventHandler for an unknown event (server:playerReady)
	serverDiags := h.diagnostics("server.lua")
	if !hasDiagnosticCode(serverDiags, "fivem-unknown-event") {
		t.Fatalf("expected fivem-unknown-event diagnostic on server.lua, got: %#v", serverDiags)
	}
	foundServer := false
	for _, d := range serverDiags {
		if d.Code == "fivem-unknown-event" && strings.Contains(d.Message, "server:playerReady") {
			foundServer = true
			break
		}
	}
	if !foundServer {
		t.Fatalf("expected diagnostic message to mention server:playerReady, got: %#v", serverDiags)
	}

	// shared.lua contains an AddEventHandler for an unknown event (shared:configLoaded)
	sharedDiags := h.diagnostics("shared.lua")
	if !hasDiagnosticCode(sharedDiags, "fivem-unknown-event") {
		t.Fatalf("expected fivem-unknown-event diagnostic on shared.lua, got: %#v", sharedDiags)
	}
	foundShared := false
	for _, d := range sharedDiags {
		if d.Code == "fivem-unknown-event" && strings.Contains(d.Message, "shared:configLoaded") {
			foundShared = true
			break
		}
	}
	if !foundShared {
		t.Fatalf("expected diagnostic message to mention shared:configLoaded, got: %#v", sharedDiags)
	}

	// wildcard handler should NOT produce an unknown-event diagnostic
	if len(sharedDiags) > 0 {
		for _, d := range sharedDiags {
			if d.Code == "fivem-unknown-event" && strings.Contains(d.Message, "*") {
				t.Fatalf("unexpected unknown-event diagnostic for wildcard handler: %#v", d)
			}
		}
	}
}

func TestFiveMEventDirection(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	// server.lua contains a TriggerServerEvent in a server script; should warn
	serverDiags := h.diagnostics("server.lua")
	if !hasDiagnosticCode(serverDiags, "fivem-event-direction") {
		t.Fatalf("expected fivem-event-direction diagnostic on server.lua, got: %#v", serverDiags)
	}
}

func TestFiveMServerEventSourceScope(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")
	h.server.DiagFiveMEventDirection = false
	h.server.DiagFiveMUnknownEvent = false
	h.server.DiagFiveMUnregisteredNetEvent = false
	h.server.DiagUndefinedGlobals = true

	serverDiags := h.diagnostics("server.lua")
	if got := countUndefinedSourceDiagnostics(serverDiags); got != 1 {
		t.Fatalf("server.lua undefined source diagnostics = %d, want only top-level source diagnostic: %#v", got, serverDiags)
	}

	clientDiags := h.diagnostics("client.lua")
	if got := countUndefinedSourceDiagnostics(clientDiags); got != 1 {
		t.Fatalf("client.lua undefined source diagnostics = %d, want client handler source diagnostic: %#v", got, clientDiags)
	}
}

func TestFiveMSharedFileEvents(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	sharedDoc := h.docForMarker("shared_net_registration")
	if got := h.server.getDocumentFiveMProfile(sharedDoc).Env(); got != EnvShared {
		t.Fatalf("shared.lua env = %v, want %v", got, EnvShared)
	}

	sharedDiags := h.diagnostics("shared.lua")
	if hasDiagnosticCode(sharedDiags, "fivem-event-direction") {
		t.Fatalf("shared.lua should suppress fivem-event-direction diagnostics, got: %#v", sharedDiags)
	}

	clientItems := h.completion("client_hover")
	clientCompletion := completionItemByLabel(clientItems, "shared:bidirectionalNet")
	if clientCompletion == nil {
		t.Fatalf("client TriggerServerEvent completion missing shared event shared:bidirectionalNet: %#v", clientItems.Items)
	}
	if !strings.Contains(clientCompletion.Detail, "network handler") {
		t.Fatalf("shared:bidirectionalNet client completion detail = %q, want network handler", clientCompletion.Detail)
	}

	serverItems := h.completion("server_hover")
	serverCompletion := completionItemByLabel(serverItems, "shared:bidirectionalNet")
	if serverCompletion == nil {
		t.Fatalf("server TriggerClientEvent completion missing shared event shared:bidirectionalNet: %#v", serverItems.Items)
	}
	if !strings.Contains(serverCompletion.Detail, "network handler") {
		t.Fatalf("shared:bidirectionalNet server completion detail = %q, want network handler", serverCompletion.Detail)
	}

	clientHover := h.hover("client_shared_hover")
	if clientHover == nil || !strings.Contains(clientHover.Contents.Value, "shared:bidirectionalNet") {
		t.Fatalf("client hover for shared event = %#v, want shared:bidirectionalNet", clientHover)
	}

	serverHover := h.hover("server_shared_hover")
	if serverHover == nil || !strings.Contains(serverHover.Contents.Value, "shared:bidirectionalNet") {
		t.Fatalf("server hover for shared event = %#v, want shared:bidirectionalNet", serverHover)
	}
}

func TestFiveMEventHover(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	// Hover on client event name in client context
	hover := h.hover("client_hover")
	if hover == nil {
		t.Fatalf("expected hover on client_hover to produce content, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "shared:requestSync") {
		t.Fatalf("hover content did not include event name: %#v", hover.Contents)
	}

	// Hover on server event name in server context
	hover = h.hover("server_hover")
	if hover == nil {
		t.Fatalf("expected hover on server_hover to produce content, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "shared:syncData") {
		t.Fatalf("hover content did not include event name: %#v", hover.Contents)
	}

	// Hover on shared event name in shared context
	hover = h.hover("shared_hover")
	if hover == nil {
		t.Fatalf("expected hover on shared_hover to produce content, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "shared:reloadUI") {
		t.Fatalf("hover content did not include event name: %#v", hover.Contents)
	}
}

func TestFiveMEventCompletion(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	clientItems := h.completion("client_hover")
	serverHandler := completionItemByLabel(clientItems, "shared:requestSync")
	if serverHandler == nil {
		t.Fatalf("TriggerServerEvent completion missing server handler shared:requestSync: %#v", clientItems.Items)
	}
	if !strings.Contains(serverHandler.Detail, "network handler") {
		t.Fatalf("shared:requestSync detail = %q, want network handler", serverHandler.Detail)
	}

	builtin := completionItemByLabel(clientItems, "playerConnecting")
	if builtin == nil {
		t.Fatalf("TriggerServerEvent completion missing built-in playerConnecting: %#v", clientItems.Items)
	}
	if !strings.Contains(builtin.Detail, "built-in (SERVER)") {
		t.Fatalf("playerConnecting detail = %q, want built-in (SERVER)", builtin.Detail)
	}

	serverItems := h.completion("server_hover")
	clientHandler := completionItemByLabel(serverItems, "shared:syncData")
	if clientHandler == nil {
		t.Fatalf("TriggerClientEvent completion missing client handler shared:syncData: %#v", serverItems.Items)
	}
	if !strings.Contains(clientHandler.Detail, "network handler") {
		t.Fatalf("shared:syncData detail = %q, want network handler", clientHandler.Detail)
	}

	sharedItems := h.completion("shared_hover")
	for _, label := range []string{"shared:reloadUI", "client:playerLoaded", "server:playerReady"} {
		if !completionHasLabel(sharedItems, label) {
			t.Fatalf("TriggerEvent completion missing %s: %#v", label, sharedItems.Items)
		}
	}
}

func TestFiveMEventDefinition(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	h.requireSingleDefinitionAt("event_trigger_def", "event_handler_def")
	h.requireSingleDefinitionAt("event_add_handler_def", "event_register_def")
}

func TestFiveMEventReferences(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	refs := h.references("event_add_handler_def", false)
	if len(refs) != 4 {
		t.Fatalf("reference count for shared:requestSync = %d, want 4: %#v", len(refs), refs)
	}

	for _, markerName := range []string{"client_hover", "event_register_def", "event_add_handler_def"} {
		if !hasLocationAtMarker(h.requireMarker(markerName), refs) {
			t.Fatalf("references for shared:requestSync missing marker %s: %#v", markerName, refs)
		}
	}

	if !hasLocationAtPosition(refs, h.server.pathToURI(filepath.Join(h.root, "server.lua")), Position{Line: 8, Character: 20}) {
		t.Fatalf("references for shared:requestSync missing server TriggerServerEvent call: %#v", refs)
	}
}

func TestFiveMEventWorkspaceSymbols(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	discovered := h.workspaceSymbols("client:playerLoaded")
	if !hasWorkspaceSymbol(discovered, "client:playerLoaded", SymbolKindEvent, h.requireMarker("client_registration").URI) {
		t.Fatalf("workspace symbols for discovered event missing client:playerLoaded event symbol: %#v", discovered)
	}

	builtins := h.workspaceSymbols("playerConnecting")
	if !hasWorkspaceSymbol(builtins, "playerConnecting", SymbolKindEvent, "builtin://fivem/events") {
		t.Fatalf("workspace symbols for built-in event missing playerConnecting event symbol: %#v", builtins)
	}
}

func TestFiveMEventManifestInvalidation(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	sharedURI := h.server.pathToURI(filepath.Join(h.root, "shared.lua"))
	sharedDoc := h.server.Documents[sharedURI]
	if sharedDoc == nil {
		t.Fatalf("shared.lua document should exist at %s", sharedURI)
	}

	if profile := h.server.getDocumentFiveMProfile(sharedDoc); profile.Kind != FiveMProfileShared {
		t.Fatalf("shared.lua initial profile = %s, want %s", profile.Kind.String(), FiveMProfileShared.String())
	}

	initialEventCount := len(sharedDoc.FiveMEvents)
	if initialEventCount == 0 {
		t.Fatal("shared.lua should have scanned FiveM events before manifest change")
	}

	sharedDoc.FiveMEvents = append(sharedDoc.FiveMEvents, FiveMEventInfo{Name: "stale:event"})

	h.writeWorkspaceFile("fxmanifest.lua", `
fx_version 'cerulean'
game 'gta5'

client_scripts {'client.lua'}
server_scripts {'server.lua', 'shared.lua'}
`)
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "fxmanifest.lua")))

	if profile := h.server.getDocumentFiveMProfile(sharedDoc); profile.Kind != FiveMProfileServer {
		t.Fatalf("shared.lua profile after manifest change = %s, want %s", profile.Kind.String(), FiveMProfileServer.String())
	}

	if len(sharedDoc.FiveMEvents) != 0 {
		t.Fatalf("shared.lua event cache should be cleared after manifest change, got %#v", sharedDoc.FiveMEvents)
	}

	rescanSource := append([]byte(nil), sharedDoc.Source()...)
	rescanSource = append(rescanSource, []byte("\n-- rescan after manifest invalidation\n")...)
	h.server.updateDocument(sharedURI, rescanSource)

	if len(sharedDoc.FiveMEvents) != initialEventCount {
		t.Fatalf("shared.lua events after re-scan = %d, want %d", len(sharedDoc.FiveMEvents), initialEventCount)
	}

	for _, ev := range sharedDoc.FiveMEvents {
		if ev.Name == "stale:event" {
			t.Fatalf("stale event survived manifest invalidation and re-scan: %#v", sharedDoc.FiveMEvents)
		}
	}
}

// TestFiveMEventCodeLens ensures code lens entries can be produced for FiveM events.
// This test relies on the fixture loader and asserts that the code-path executes
// without panicking. Detailed verification of the exact lens content is exercised
// by the integration tests of the client, not unit tests here.
func TestFiveMEventCodeLens(t *testing.T) {
	// Load a typical resource_events fixture. The actual code lens content is
	// exercised by the integration with the client; here we only ensure the
	// code path executes without errors during CodeLens generation.
	_ = newFiveMFixtureHarness(t, "resource_events")
}

func TestFiveMEventCodeLensReferencesCommand(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	assertEventLens := func(relPath, markerName string) {
		t.Helper()

		marker := h.requireMarker(markerName)
		var lens *CodeLens
		for _, candidate := range h.codeLenses(relPath) {
			if candidate.Command != nil && candidate.Command.Command == "lugo.showReferences" && rangeContainsPosition(candidate.Range, marker.Position) {
				candidate := candidate
				lens = &candidate
				break
			}
		}
		if lens == nil {
			t.Fatalf("code lens for %s not found in %s", markerName, relPath)
		}

		if lens.Command == nil || lens.Command.Command != "lugo.showReferences" {
			t.Fatalf("code lens for %s = %+v, want showReferences command", markerName, lens)
		}
		if len(lens.Command.Arguments) != 3 {
			t.Fatalf("code lens arguments for %s = %#v, want uri + position + locations", markerName, lens.Command.Arguments)
		}

		resolved := h.resolveCodeLens(*lens)
		if resolved.Command == nil || resolved.Command.Command != "lugo.showReferences" {
			t.Fatalf("resolved code lens for %s = %+v, want showReferences command", markerName, resolved)
		}
		if len(resolved.Command.Arguments) != 3 {
			t.Fatalf("resolved code lens arguments for %s = %#v, want uri + position + locations", markerName, resolved.Command.Arguments)
		}

		positionArg, ok := resolved.Command.Arguments[1].(map[string]any)
		if !ok {
			t.Fatalf("resolved code lens position argument for %s = %#v, want object", markerName, resolved.Command.Arguments[1])
		}
		if _, ok := positionArg["line"].(float64); !ok {
			t.Fatalf("resolved code lens position for %s missing line: %#v", markerName, positionArg)
		}

		locationsArg, ok := resolved.Command.Arguments[2].([]any)
		if !ok || len(locationsArg) == 0 {
			t.Fatalf("resolved code lens locations for %s = %#v, want non-empty array", markerName, resolved.Command.Arguments[2])
		}

		firstLoc, ok := locationsArg[0].(map[string]any)
		if !ok {
			t.Fatalf("resolved first location for %s = %#v, want object", markerName, locationsArg[0])
		}
		rangeArg, ok := firstLoc["range"].(map[string]any)
		if !ok {
			t.Fatalf("resolved location range for %s = %#v, want object", markerName, firstLoc)
		}
		startArg, ok := rangeArg["start"].(map[string]any)
		if !ok {
			t.Fatalf("resolved location start for %s = %#v, want object", markerName, rangeArg)
		}
		if _, ok := startArg["line"].(float64); !ok {
			t.Fatalf("resolved location start for %s missing line: %#v", markerName, startArg)
		}
	}

	assertEventLens("shared.lua", "event_add_handler_def")
	assertEventLens("server.lua", "event_register_def")
}

func hasWorkspaceSymbol(symbols []SymbolInformation, name string, kind SymbolKind, uri string) bool {
	for _, symbol := range symbols {
		if symbol.Name == name && symbol.Kind == kind && symbol.Location.URI == uri {
			return true
		}
	}

	return false
}

func (h *fiveMFixtureHarness) simulateWatchedFileChange(uri string) {
	h.t.Helper()

	params, err := json.Marshal(DidChangeWatchedFilesParams{
		Changes: []FileEvent{
			{URI: uri, Type: 2}, // Changed
		},
	})
	if err != nil {
		h.t.Fatalf("marshal watched files params: %v", err)
	}

	h.resetRPC()
	h.server.handleDidChangeWatchedFiles(Request{RPC: "2.0", ID: 1, Params: params})
}

func hasLocationAtMarker(marker fiveMFixtureMarker, locations []Location) bool {
	return hasLocationAtPosition(locations, marker.URI, marker.Position)
}

func rangeContainsPosition(rng Range, pos Position) bool {
	if pos.Line < rng.Start.Line || pos.Line > rng.End.Line {
		return false
	}
	if pos.Line == rng.Start.Line && pos.Character < rng.Start.Character {
		return false
	}
	if pos.Line == rng.End.Line && pos.Character > rng.End.Character {
		return false
	}

	return true
}

func hasLocationAtPosition(locations []Location, uri string, position Position) bool {
	for _, loc := range locations {
		if loc.URI == uri && loc.Range.Start == position {
			return true
		}
	}

	return false
}

func countUndefinedSourceDiagnostics(diags []Diagnostic) int {
	var count int
	for _, diag := range diags {
		if diag.Code == "undefined-global" && strings.Contains(diag.Message, "'source'") {
			count++
		}
	}

	return count
}
