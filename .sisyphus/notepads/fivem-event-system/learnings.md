# FiveM Unknown Event Diagnostics - Learnings

- Implemented a new diagnostic: fivem-unknown-event (Information) that flags AddEventHandler events that have no corresponding built-in or TriggerEvent/Trigger* in the workspace.
- Diagnostic is gated behind feature flags: FeatureFiveM and DiagFiveMUnknownEvent.
- The diagnostic checks each doc's FiveMEvents for AddHandler entries, skips wildcard events, and resolves against the global EventsBuiltin map and other documents' TriggerEvent definitions.
- Test coverage added: TestFiveMUnknownEventDiagnostic validates diagnostics for resource_events fixture (client/server/shared AddEventHandler events) and ensures wildcard handler does not produce diagnostics.
Add diagnostic for unregistered FiveM network events (TriggerServerEvent/TriggerClientEvent) within the same resource.
- Implemented diagFiveMUnregisteredNetEvent in lsp/diagnostics.go gated by FeatureFiveM and DiagFiveMUnregisteredNetEvent.
- Implemented diag logic to scan doc.FiveMEvents and resource graph to find missing RegisterNetEvent within the same resource.
- Added unit test TestFiveMUnregisteredNetEvent and fixture resource_events_unregistered/server.lua to exercise the case.
- Updated test harness to verify code 'fivem-unregistered-net-event' diagnostic appears when unregistered events are triggered.

- Task 12: FiveM event find-references should resolve from doc.fiveMEventAtOffset and return doc.fiveMEventNameRange for every matching doc.FiveMEvents entry across s.Documents, gated by FeatureFiveM. The fixture esource_events has four shared:requestSync references: client trigger, server RegisterNetEvent, server TriggerServerEvent, and shared AddEventHandler.

- Task 15: Shared-file FiveM events rely on the existing EnvShared classification and completion filtering (EnvShared allowed for both TriggerServerEvent and TriggerClientEvent). Regression coverage lives in lsp/fivem_events_test.go::TestFiveMSharedFileEvents, using shared:bidirectionalNet from lsp/testdata/fivem/resource_events/shared.lua to verify no direction diagnostic plus client/server completion and hover visibility.

- Task 17 added BenchmarkFiveMEventScanning in lsp/fivem_perf_test.go. It builds a realistic 320-group FiveM Lua source with RegisterNetEvent, AddEventHandler, TriggerEvent, TriggerServerEvent, and TriggerClientEvent calls, then measures the existing updateDocument/finalizeDocumentUpdate path and asserts 1,600 scanned events. Verification on Windows amd64: go test -run=^$ -bench=BenchmarkFiveM -count=1 ./lsp passed; BenchmarkFiveMEventScanning-12 reported 2,845,565 ns/op, 5,425,576 B/op, 1,670 allocs/op.

- Task 16: Manifest changes for an existing FiveM resource must clear affected documents' FiveMEvents when their FiveMProfileCached state is invalidated; initial manifest registration (oldRes == nil) should not clear event scans because Lua files may already have been finalized during initial indexing.

- Final Wave F2 fixes applied:
  - Ran `gofmt -w` on lsp/diagnostics.go, lsp/server.go, lsp/fivem.go, lsp/features.go to fix space indentation (Go standard uses tabs).
  - Fixed regex compilation inside loop in handleCodeLens (features.go): replaced `regexp.MustCompile()` called per document with simple `strings.Count()` for TriggerEvent/TriggerServerEvent/TriggerClientEvent pattern matching. Removed unused "regexp" import.

- 2026-05-05: FiveM event completion deduplication must add RegisterNetEvent entries before AddEventHandler entries so network handler detail wins deterministically when both declarations share an event name. FiveM CodeLens should derive counts from the existing document fallback path and skip zero-reference lenses rather than keeping a separate Server event index.
