# FiveM Event Intelligence System for Lugo

## TL;DR

> **Quick Summary**: Build a comprehensive FiveM event intelligence system into the lugo LSP. Scan `AddEventHandler`/`RegisterNetEvent`/`TriggerEvent`/`TriggerServerEvent`/`TriggerClientEvent` calls across the workspace, track events per-resource with cross-resource resolution, and wire into all six LSP features (hover, completion, go-to-def, find-refs, diagnostics, code lens). Follows the existing `FiveMLuaExports` + `FiveMResourceGraph` patterns with zero additional allocation in the parser hot path.
>
> **Deliverables**:
> - Event scanner integrated into `finalizeDocumentUpdate` (workspace.go)
> - Per-document event data on `Document` struct (document.go)
> - Cross-resource event tracking via `FiveMResourceGraphNode` (fivem.go)
> - ~15 built-in game event definitions (fivem.go)
> - Three new diagnostics: direction mismatch, unregistered net event, unknown event
> - Six LSP features: hover, completion, go-to-def, find-refs, workspace symbols, code lens
> - TDD test suite with fixture-based harness (fivem_events_test.go)
>
> **Estimated Effort**: Large (17 implementation + 4 verification = 21 tasks)
> **Parallel Execution**: YES — 4 waves, max 8 concurrent
> **Critical Path**: Task 1 → Task 5 → Task 9-14 → Task 17 → F1-F4

---

## Context

### Original Request
Build an event handling system for lugo to "make event easier to subscribe and create across a project (a workspace)." Comprehensive event intelligence — discovery, validation, type inference, and navigation.

### Interview Summary

**Key Discussions**:
- **LSP Features**: All six (hover, completion, go-to-def, find-refs, workspace symbols, code lens) included
- **Discovery Method**: Scan all Lua files for `AddEventHandler`/`RegisterNetEvent`/`TriggerEvent*` calls — no annotations or manifest directives
- **Built-in Events**: ~15 core lifecycle events (playerConnecting, playerJoining, playerDropped, entityCreating, entityCreated, entityRemoved, weaponDamageEvent, onResourceStarting/Start/Stop, playerSpawned, characterUnloaded, gameEventTriggered, entityDamaged, sessionInitialized, chat:addMessage)
- **Data Structure**: Extend existing `FiveMLuaExports` + `FiveMResourceGraph` patterns — no new allocs in parser hot path
- **Index Scope**: Workspace-wide (cross-resource discovery)
- **Test Strategy**: TDD with existing Go test harness (`fiveMFixtureHarness`)
- **NUI Callbacks**: Out of scope
- **Payload Types**: Basic type inference from handler annotations
- **Direction Validation**: Warnings (not errors) — account for shared scripts
- **RegisterNetEvent Gating**: Validate within-resource
- **CRITICAL CONSTRAINT**: Zero additional memory allocation in parser/resolver. Integrate with existing arena architecture. Event analysis data (strings) can allocate like `FiveMLuaExport.Name` does.

**Research Findings**:
- Events are currently stdlib globals in `lsp/stdlib/fivem/shared.lua` — NOT specially analyzed
- `FiveMLuaExports` scan pattern at `workspace.go:743-769` is the 1:1 template
- `FiveMResourceGraphNode` at `fivem.go:392-403` is the cross-resource index pattern
- `unquoteLuaString()` at `fivem.go:805-829` handles all Lua string quoting variants
- Test harness: `newFiveMFixtureHarness()` at `fivem_fixture_harness_test.go:172`
- Diagnostic flags pattern: `server.go:117-119` + `messages.go:151-153`

### Metis Review

**Identified Gaps** (addressed):
- **String vs byte offsets for event names**: Follow `FiveMLuaExport.Name string` pattern — analysis data can allocate
- **Cross-resource index location**: Use option (c) — per-document `FiveMEvents` on `Document` + event slices on `FiveMResourceGraphNode`
- **Built-in events format**: Hardcoded Go `map[string]FiveMBuiltinEvent` constant — simplest, zero runtime overhead
- **Payload type inference**: Tier 2 — infer from handler function parameter annotations
- **Wildcard handlers (`*`)**: Ignore in MVP — treat as valid-anywhere marker but don't suppress diagnostics
- **Shared file ambiguity**: Events in shared files registered for both client AND server sides, direction warnings suppressed for shared context
- **Manifest invalidation**: Re-scan events when `FiveMProfileCached` is invalidated (follow existing export pattern)

---

## Work Objectives

### Core Objective
Enable workspace-wide FiveM event intelligence in the lugo LSP: discover all event registrations and triggers, validate correctness (direction, gating, existence), provide navigation (go-to-def, find-refs), and offer smart completions and hover information.

### Concrete Deliverables
- `lsp/document.go`: `FiveMEventKind`, `FiveMEventInfo`, `FiveMEventRef` types + `FiveMEvents` field on `Document`
- `lsp/fivem.go`: `FiveMBuiltinEvent` type + `EventsBuiltin` constant + event fields on `FiveMResourceGraphNode`
- `lsp/workspace.go`: Event scanner block in `finalizeDocumentUpdate()` after line 769
- `lsp/diagnostics.go`: Three new diagnostic publishers (direction, unregistered-net, unknown-event)
- `lsp/server.go`: Three new diagnostic config flags; builtin events map on `Server`
- `lsp/messages.go`: Three new diagnostic option fields for client configuration
- `lsp/features.go`: Hover, completion, go-to-def, find-refs, code lens handlers for event names
- `lsp/symbols.go`: Workspace symbol enumeration for events; cross-resource event resolution helpers
- `lsp/fivem_events_test.go`: TDD test suite (fixture-based)
- `lsp/testdata/fivem/resource_events/`: Test fixtures (manifest, client, server, shared files)

### Definition of Done
- [ ] `go test ./lsp/ -run TestFiveMEvent -count=1` → PASS (all event-related tests)
- [ ] `go test ./lsp/ -run TestFiveMFixtureHarness -count=1` → PASS (existing fixtures not broken)
- [ ] `go vet ./lsp/` → no new warnings
- [ ] All three diagnostic codes fire on correct fixtures
- [ ] Hover/completion/goto-def/refs resolve correctly across resources

### Must Have
- Event discovery via AST scan in `finalizeDocumentUpdate()`
- Three diagnostics: `fivem-event-direction`, `fivem-unregistered-net-event`, `fivem-unknown-event`
- Hover showing event type, subset, and known handlers
- Completion listing known event names after `AddEventHandler("` or `TriggerEvent("`
- Go-to-definition jumping from trigger to handler and vice versa
- Find-references showing all handlers and triggers for an event
- Workspace symbols including event names
- Code Lens showing reference counts
- ~15 built-in game events with subset and description
- TDD test suite with fixture harness
- All new diagnostics gated behind feature + individual toggle flags
- Shared file events visible from both client and server contexts
- Manifest change triggers event data re-scan

### Must NOT Have (Guardrails)
- **NO** NUI callback tracking (`RegisterNUICallback`, `__cfx_nui:*` events) — out of scope
- **NO** `TriggerLatentServerEvent`/`TriggerLatentClientEvent` handling — same event names, separate transport
- **NO** `RemoveEventHandler` tracking
- **NO** exhaustive built-in event catalog (limited to ~15 core events)
- **NO** network event gating validation across resource boundaries (within-resource only)
- **NO** new heap allocations in the parser hot path (analysis data allocations OK)
- **NO** separate event index map on `Server` — use existing structures
- **NO** modification to `ast/`, `parser/`, `lexer/`, `token/`, or `semantic/` packages
- **NO** invalidating existing `FiveMLuaExports` or `FiveMResourceGraph` behavior

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** - ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (`testing` package, `fiveMFixtureHarness`, fixture testdata)
- **Automated tests**: TDD (RED → GREEN → REFACTOR per task)
- **Framework**: Go `testing` package with fixture-based harness

### QA Policy
Every task MUST include agent-executed QA scenarios. Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.txt`.
- **API/Backend**: Use Bash (`go test`) — run specific test functions, assert PASS/FAIL
- **Library/Module**: Use Bash (`go vet`) — verify no warnings introduced

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — foundation + test fixtures):
├── Task 1: Event data structures + Document/ResourceGraph integration [quick]
├── Task 2: Built-in event definitions (~15 core events) [quick]
├── Task 3: Diagnostic config flags [quick]
└── Task 4: Test fixture setup [quick]

Wave 2 (After Wave 1 — scanner + diagnostics, MAX PARALLEL):
├── Task 5: Event call scanner (workspace.go) — TDD [deep]
├── Task 6: Event direction diagnostic — TDD [deep]
├── Task 7: Unregistered net event diagnostic — TDD [deep]
└── Task 8: Unknown event diagnostic — TDD [deep]

Wave 3 (After Wave 2 — LSP features, MAX PARALLEL):
├── Task 9: Hover on event name strings — TDD [deep]
├── Task 10: Completion for event names — TDD [deep]
├── Task 11: Go-to-definition on event names — TDD [deep]
├── Task 12: Find-references on event names — TDD [deep]
├── Task 13: Workspace symbols for events — TDD [deep]
├── Task 14: Code Lens for events — TDD [deep]

Wave 4 (After Wave 3 — edge cases + perf, MAX PARALLEL):
├── Task 15: Shared file ambiguity handling — TDD [deep]
├── Task 16: Manifest invalidation re-scans events — TDD [deep]
└── Task 17: Perf budget for event scanning — TDD [deep]

Wave FINAL (After ALL tasks — 4 parallel reviews, then user okay):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)
→ Present results → Get explicit user okay
```

**Critical Path**: Task 1 → Task 5 → Task 9 → Task 17 → F1-F4 → user okay
**Parallel Speedup**: ~65% faster than sequential
**Max Concurrent**: 10 (Wave 3)

### Agent Dispatch Summary

- **1**: **4** — T1-T4 → `quick`
- **2**: **4** — T5-T8 → `deep`
- **3**: **6** — T9-T14 → `deep`
- **4**: **3** — T15-T17 → `deep`
- **FINAL**: **4** — F1 → `oracle`, F2 → `unspecified-high`, F3 → `unspecified-high`, F4 → `deep`

---

## TODOs

- [x] 1. Event data structures + Document/ResourceGraph integration

  **What to do**:
  - Define `FiveMEventKind` iota in `lsp/document.go` (after `FiveMLuaExport` struct):
    ```go
    type FiveMEventKind int
    const (
        FiveMEventAddHandler FiveMEventKind = iota
        FiveMEventRegisterNet
        FiveMEventTriggerLocal
        FiveMEventTriggerServer
        FiveMEventTriggerClient
    )
    ```
  - Define `FiveMEventInfo` struct in `lsp/document.go`:
    ```go
    type FiveMEventInfo struct {
        Name      string
        Kind      FiveMEventKind
        NodeID    ast.NodeID
        HandlerID ast.NodeID // only for AddHandler/RegisterNetEvent
    }
    ```
  - Define `FiveMEventRef` struct in `lsp/fivem.go` (for cross-resource references):
    ```go
    type FiveMEventRef struct {
        Name string
        URI  string
        Kind FiveMEventKind
    }
    ```
  - Add `FiveMEvents []FiveMEventInfo` field to `Document` struct (after line 43: `FiveMLuaExports []FiveMLuaExport`)
  - Add event slices to `FiveMResourceGraphNode` struct (after line 403):
    ```go
    ServerHandlers []FiveMEventRef
    ClientHandlers []FiveMEventRef
    SharedHandlers []FiveMEventRef
    ServerTriggers []FiveMEventRef
    ClientTriggers []FiveMEventRef
    SharedTriggers []FiveMEventRef
    ```
  - Add `FiveMEventIndex map[ast.HashKey][]*FiveMEventRef` to `Server` struct (after line 119) for O(1) event name lookup across workspace — populated during finalization, NOT at query time
  - Write test: `fivem_events_test.go` → `TestFiveMEventDataStructures` — create Document, append FiveMEventInfo, verify fields are correct, verify slice reset via `[:0]` works

  **Must NOT do**:
  - Do NOT add fields to `ast/`, `parser/`, or `semantic/` packages
  - Do NOT modify the `Tree` or `Node` struct
  - Do NOT create a standalone event index struct — use existing `Document` and `FiveMResourceGraphNode`

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Pure data structure definitions — no complex logic, no external dependencies
  - **Skills**: [`svelte-core-bestpractices`]
    - `svelte-core-bestpractices`: NOT NEEDED — this is pure Go struct definitions
    - No skills needed — this is straightforward Go type addition

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 2, 3, 4)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 5, Task 6, Task 7, Task 8
  - **Blocked By**: None (can start immediately)

  **References**:
  - `lsp/document.go:124-127` — `FiveMLuaExport` struct pattern (copy this structure)
  - `lsp/document.go:20-47` — `Document` struct (add `FiveMEvents` after line 43)
  - `lsp/fivem.go:392-403` — `FiveMResourceGraphNode` struct (add event slices after line 403)
  - `lsp/server.go:100-125` — `Server` struct (add `FiveMEventIndex` after line 119)
  - `ast/hash.go` — `HashKey` type for event name lookups (check exact import path)

  **Acceptance Criteria**:
  - [ ] `FiveMEventKind` iota defined with 5 constants
  - [ ] `FiveMEventInfo` struct defined with 4 fields
  - [ ] `FiveMEventRef` struct defined in fivem.go with 3 fields
  - [ ] `Document.FiveMEvents` field compiles
  - [ ] `FiveMResourceGraphNode` has 6 new event slice fields
  - [ ] `Server.FiveMEventIndex` field compiles (map type)
  - [ ] `go vet ./lsp/` → no new warnings
  - [ ] Data structure test passes: `go test ./lsp/ -run TestFiveMEventDataStructures -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Event info struct creation and field access
    Tool: Bash (go test)
    Preconditions: Test file fivem_events_test.go exists with TestFiveMEventDataStructures
    Steps:
      1. Create FiveMEventInfo{Name: "test:event", Kind: FiveMEventAddHandler, NodeID: 42, HandlerID: 99}
      2. Assert info.Name == "test:event"
      3. Assert info.Kind == FiveMEventAddHandler
      4. Assert info.NodeID == 42
      5. Assert info.HandlerID == 99
    Expected Result: All assertions pass, test function returns without t.Fatal
    Evidence: .sisyphus/evidence/task-1-data-structures.txt

  Scenario: Document FiveMEvents slice reuse via [:0]
    Tool: Bash (go test)
    Preconditions: Document struct has FiveMEvents field
    Steps:
      1. Create Document, append 3 FiveMEventInfo entries to FiveMEvents
      2. Assert len(doc.FiveMEvents) == 3
      3. Reset: doc.FiveMEvents = doc.FiveMEvents[:0]
      4. Assert len(doc.FiveMEvents) == 0
      5. Assert cap(doc.FiveMEvents) >= 3 (backing array preserved)
    Expected Result: Slice length 0, capacity preserved, no new allocation
    Evidence: .sisyphus/evidence/task-1-slice-reuse.txt
  ```

  **Commit**: YES (Wave 1 group)
  - Message: `feat(fivem): add event data structures and built-in definitions`
  - Files: `lsp/document.go`, `lsp/fivem.go`, `lsp/server.go`, `lsp/messages.go`, `lsp/fivem_events_test.go`

- [x] 2. Built-in event definitions (~15 core events)

  **What to do**:
  - Define `FiveMBuiltinEvent` struct in `lsp/fivem.go`:
    ```go
    type FiveMBuiltinEvent struct {
        Name        string
        Subset      string // "CLIENT", "SERVER", "SHARED"
        Description string
        Payload     string // human-readable payload description
    }
    ```
  - Define `EventsBuiltin` constant in `lsp/fivem.go` — a package-level `var` map:
    ```go
    var EventsBuiltin = map[string]FiveMBuiltinEvent{
        "playerConnecting":    {Name: "playerConnecting", Subset: "SERVER", Description: "Fired when a player is connecting to the server", Payload: "playerName: string, setKickReason: function, deferrals: Deferrals"},
        "playerJoining":       {Name: "playerJoining", Subset: "SERVER", Description: "Fired when a player connection is accepted", Payload: "source: number, oldId: number"},
        "playerDropped":       {Name: "playerDropped", Subset: "SERVER", Description: "Fired when a player disconnects", Payload: "reason: string"},
        "entityCreating":      {Name: "entityCreating", Subset: "SERVER", Description: "Fired before an entity is created", Payload: "entity: number"},
        "entityCreated":       {Name: "entityCreated", Subset: "SERVER", Description: "Fired after an entity is created", Payload: "entity: number"},
        "entityRemoved":       {Name: "entityRemoved", Subset: "SERVER", Description: "Fired when an entity is removed", Payload: "entity: number"},
        "weaponDamageEvent":   {Name: "weaponDamageEvent", Subset: "SHARED", Description: "Fired on weapon damage", Payload: "victim, attacker: number | isHeadshot, isMelee: boolean | weaponType: hash"},
        "onResourceStarting":  {Name: "onResourceStarting", Subset: "SHARED", Description: "Fired when a resource begins loading", Payload: "resourceName: string"},
        "onResourceStart":     {Name: "onResourceStart", Subset: "SHARED", Description: "Fired when a resource has started", Payload: "resourceName: string"},
        "onResourceStop":      {Name: "onResourceStop", Subset: "SHARED", Description: "Fired when a resource is stopping", Payload: "resourceName: string"},
        "playerSpawned":       {Name: "playerSpawned", Subset: "SHARED", Description: "Fired when a player spawns", Payload: "source: number (server), none (client)"},
        "characterUnloaded":   {Name: "characterUnloaded", Subset: "SHARED", Description: "Fired when a player character model is unloaded", Payload: "source: number"},
        "gameEventTriggered":  {Name: "gameEventTriggered", Subset: "CLIENT", Description: "Fired for all game events (data file events)", Payload: "eventName: string, args: table"},
        "entityDamaged":       {Name: "entityDamaged", Subset: "CLIENT", Description: "Fired when an entity receives damage", Payload: "victim, attacker: number | weapon: hash | damageFlags: number"},
        "sessionInitialized":  {Name: "sessionInitialized", Subset: "SHARED", Description: "Fired when the game session is initialized and ready", Payload: "none"},
    }
    ```
  - Add `EventsBuiltin` to `Server` struct as a pointer/reference — NOT a copy:
    - Actually: since it's a package-level map, just reference `EventsBuiltin` directly in code. No Server field needed.
  - Write test: `TestFiveMBuiltinEvents` — verify map has 15 entries, verify known event exists with correct subset

  **Must NOT do**:
  - Do NOT add more than ~15 events — keep it minimal for MVP
  - Do NOT copy the map per-Server instance — use the package-level var

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Data entry — populating a map with known values from specs
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 1, 3, 4)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 9 (hover uses built-in info)
  - **Blocked By**: None (can start immediately)

  **References**:
  - `fivem-specs/game-events-reference.md` — authoritative event definitions with subsets and payloads
  - `fivem-specs/event-system-reference.md` — event lifecycle context
  - `lsp/fivem.go:411-417` — pattern for package-level constructor/factory functions

  **Acceptance Criteria**:
  - [ ] `FiveMBuiltinEvent` struct defined with 4 fields (Name, Subset, Description, Payload)
  - [ ] `EventsBuiltin` map has 15 entries
  - [ ] Each entry has correct subset (CLIENT/SERVER/SHARED) per spec
  - [ ] `go vet ./lsp/` → no warnings
  - [ ] Built-in events test passes: `go test ./lsp/ -run TestFiveMBuiltinEvents -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Built-in events map has correct size and known entries
    Tool: Bash (go test)
    Preconditions: EventsBuiltin map defined in fivem.go
    Steps:
      1. Assert len(EventsBuiltin) == 15
      2. Look up "playerConnecting" → assert Subset == "SERVER"
      3. Look up "gameEventTriggered" → assert Subset == "CLIENT"
      4. Look up "onResourceStart" → assert Subset == "SHARED"
      5. Look up "nonexistent_event" → assert not found (ok=false)
    Expected Result: All 15 entries present, subsets correct, missing key returns false
    Evidence: .sisyphus/evidence/task-2-builtin-events.txt

  Scenario: Unknown event lookup returns zero value
    Tool: Bash (go test)
    Preconditions: EventsBuiltin map accessible
    Steps:
      1. ev, ok := EventsBuiltin["nonexistent_event_name"]
      2. Assert ok == false
      3. Assert ev.Name == "" (zero value)
    Expected Result: Missing keys handled safely
    Evidence: .sisyphus/evidence/task-2-missing-key.txt
  ```

  **Commit**: YES (Wave 1 group)
  - Message: `feat(fivem): add event data structures and built-in definitions`
  - Files: `lsp/fivem.go`

- [x] 3. Diagnostic config flags (server.go + messages.go)

  **What to do**:
  - Add three new diagnostic boolean fields to `Server` struct in `lsp/server.go` (after line 119):
    ```go
    DiagFiveMEventDirection       bool
    DiagFiveMUnregisteredNetEvent bool
    DiagFiveMUnknownEvent         bool
    ```
  - Add three corresponding JSON fields to `GlobalSettingsOptions` in `lsp/messages.go` (after line 153):
    ```go
    DiagFiveMEventDirection       bool `json:"diagFiveMEventDirection"`
    DiagFiveMUnregisteredNetEvent bool `json:"diagFiveMUnregisteredNetEvent"`
    DiagFiveMUnknownEvent         bool `json:"diagFiveMUnknownEvent"`
    ```
  - Add three `setCfg` calls in the settings refresh handler in `lsp/server.go` (after line 315):
    ```go
    setCfg(&s.DiagFiveMEventDirection, opts.DiagFiveMEventDirection, &needsRepublish)
    setCfg(&s.DiagFiveMUnregisteredNetEvent, opts.DiagFiveMUnregisteredNetEvent, &needsRepublish)
    setCfg(&s.DiagFiveMUnknownEvent, opts.DiagFiveMUnknownEvent, &needsRepublish)
    ```
  - Enable all three in `newFiveMFixtureHarness` function in `lsp/fivem_fixture_harness_test.go` (after line 192):
    ```go
    s.DiagFiveMEventDirection = true
    s.DiagFiveMUnregisteredNetEvent = true
    s.DiagFiveMUnknownEvent = true
    ```

  **Must NOT do**:
  - Do NOT wire these into actual diagnostics yet (Tasks 6-8 do that)
  - Do NOT add the flags to `messages.go` without the corresponding `json` struct tags

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Boilerplate config flag wiring — same pattern repeated 3 times

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 1, 2, 4)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 6, 7, 8
  - **Blocked By**: None (can start immediately)

  **References**:
  - `lsp/server.go:117-119` — existing `DiagFiveM*` fields pattern
  - `lsp/server.go:313-315` — existing `setCfg` call pattern
  - `lsp/messages.go:151-153` — existing diagnostic options JSON fields
  - `lsp/fivem_fixture_harness_test.go:190-192` — existing test harness flag enablement

  **Acceptance Criteria**:
  - [ ] Three new fields on `Server` struct compile
  - [ ] Three new JSON fields in `GlobalSettingsOptions` with correct tags
  - [ ] Three `setCfg` calls in settings handler
  - [ ] Test harness enables all three flags
  - [ ] `go build ./lsp/` → success (compile check)
  - [ ] `go vet ./lsp/` → no warnings

  **QA Scenarios**:

  ```
  Scenario: Config flags are properly wired and toggleable
    Tool: Bash (go test)
    Preconditions: Test server created with newFiveMFixtureHarness
    Steps:
      1. Create test harness: h := newFiveMFixtureHarness(t, "resource_events")
      2. Assert h.server.DiagFiveMEventDirection == true
      3. Assert h.server.DiagFiveMUnregisteredNetEvent == true
      4. Assert h.server.DiagFiveMUnknownEvent == true
    Expected Result: All three flags enabled by default in fixture harness
    Evidence: .sisyphus/evidence/task-3-config-flags.txt

  Scenario: Disabling flags suppresses related diagnostics
    Tool: Bash (go test)
    Preconditions: Server with event-introducing fixture file
    Steps:
      1. Create test harness, set s.DiagFiveMEventDirection = false
      2. Index a file with a known direction mismatch
      3. Assert no "fivem-event-direction" diagnostics published
    Expected Result: Diagnostic suppressed when flag is false
    Evidence: .sisyphus/evidence/task-3-flag-suppression.txt
  ```

  **Commit**: YES (Wave 1 group)
  - Message: `feat(fivem): add event data structures and built-in definitions`
  - Files: `lsp/server.go`, `lsp/messages.go`, `lsp/fivem_fixture_harness_test.go`

- [x] 4. Test fixture setup (resource_events testdata)

  **What to do**:
  - Create directory `lsp/testdata/fivem/resource_events/`
  - Create `fxmanifest.lua`:
    ```lua
    fx_version 'cerulean'
    game 'gta5'

    client_scripts {
        'client.lua',
    }

    server_script 'server.lua'
    shared_script 'shared.lua'
    ```
  - Create `client.lua`:
    ```lua
    --[[@client_registration]]
    AddEventHandler("client:playerLoaded", function(source)
        --[[@client_hover]]
        TriggerServerEvent("shared:requestSync")
    end)

    --[[@client_net_registration]]
    RegisterNetEvent("shared:syncData", function(data)
        --[[@client_handler_def]]
        print("synced", data)
    end)
    ```
  - Create `server.lua`:
    ```lua
    --[[@server_registration]]
    AddEventHandler("server:playerReady", function(source, name)
        --[[@server_hover]]
        TriggerClientEvent("shared:syncData", -1, {ready = true})
    end)

    --[[@server_net_registration]]
    RegisterNetEvent("shared:requestSync")

    --[[@server_direction_error]]
    TriggerServerEvent("shared:requestSync") -- should trigger direction warning
    ```
  - Create `shared.lua`:
    ```lua
    --[[@shared_registration]]
    AddEventHandler("shared:configLoaded", function(config)
        --[[@shared_hover]]
        TriggerEvent("shared:reloadUI")
    end)

    --[[@shared_wildcard]]
    AddEventHandler("*", function(eventName, ...)
        print("wildcard:", eventName)
    end)
    ```
  - Register `"resource_events"` in the fixture list at `TestFiveMFixtureHarness` in `lsp/fivem_fixture_harness_test.go` (line ~42)
  - Write test: `TestFiveMEventFixtureLoading` — loads these fixtures and verifies markers are found

  **Must NOT do**:
  - Do NOT create real FiveM resource files — these are test fixtures only
  - Do NOT use `RegisterNUICallback` or `__cfx_nui:` events (out of scope)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: File creation + marker annotations — no complex logic

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 1, 2, 3)
  - **Parallel Group**: Wave 1
  - **Blocks**: Task 5 (scanner needs test data)
  - **Blocked By**: None (can start immediately)

  **References**:
  - `lsp/testdata/fivem/resource_client_server_shared/` — existing fixture layout pattern
  - `lsp/testdata/fivem/resource_exports/` — fixture with markers for go-to-def/hover
  - `lsp/fivem_fixture_harness_test.go:41-52` — fixture list registration
  - `lsp/fivem_fixture_harness_test.go:17-25` — marker format (`--[[@marker_name]]`)

  **Acceptance Criteria**:
  - [ ] `lsp/testdata/fivem/resource_events/` directory exists with 4 files
  - [ ] `fxmanifest.lua` defines client, server, shared scripts
  - [ ] `client.lua` has 4 markers: `client_registration`, `client_hover`, `client_net_registration`, `client_handler_def`
  - [ ] `server.lua` has 4 markers: `server_registration`, `server_hover`, `server_net_registration`, `server_direction_error`
  - [ ] `shared.lua` has 3 markers: `shared_registration`, `shared_hover`, `shared_wildcard`
  - [ ] `"resource_events"` added to `TestFiveMFixtureHarness` fixture list
  - [ ] Test fixture loading passes: `go test ./lsp/ -run TestFiveMEventFixtureLoading -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: All fixture markers are registered and discoverable
    Tool: Bash (go test)
    Preconditions: Fixture harness loaded with resource_events
    Steps:
      1. h := newFiveMFixtureHarness(t, "resource_events")
      2. h.requireMarker("client_registration") → must not panic
      3. h.requireMarker("client_hover") → must not panic
      4. h.requireMarker("server_direction_error") → must not panic
      5. h.requireMarker("shared_wildcard") → must not panic
    Expected Result: All 11 markers found without errors
    Evidence: .sisyphus/evidence/task-4-fixture-markers.txt

  Scenario: Fixture documents have correct FiveM profiles
    Tool: Bash (go test)
    Preconditions: Fixture harness loaded and indexed
    Steps:
      1. clientDoc := h.docForMarker("client_registration")
      2. Assert h.server.getDocumentFiveMProfile(clientDoc).Kind == FiveMProfileClient
      3. serverDoc := h.docForMarker("server_registration")
      4. Assert h.server.getDocumentFiveMProfile(serverDoc).Kind == FiveMProfileServer
      5. sharedDoc := h.docForMarker("shared_registration")
      6. Assert h.server.getDocumentFiveMProfile(sharedDoc).Kind == FiveMProfileShared
    Expected Result: All three documents have correct profile classification
    Evidence: .sisyphus/evidence/task-4-fixture-profiles.txt
  ```

  **Commit**: YES (Wave 1 group)
  - Message: `feat(fivem): add event data structures and built-in definitions`
  - Files: `lsp/testdata/fivem/resource_events/*.lua`, `lsp/fivem_fixture_harness_test.go`

- [x] 5. Event call scanner in finalizeDocumentUpdate — TDD

  **What to do**:

  **RED (write failing test first)**:
  - Add to `fivem_events_test.go`: `TestFiveMEventScanner` — uses fixture harness with `resource_events` fixture
  - After indexing, retrieve `clientDoc`, `serverDoc`, `sharedDoc`
  - Assert `len(clientDoc.FiveMEvents) >= 4` (AddEventHandler + RegisterNetEvent + 2 TriggerEvent variants)
  - Assert `len(serverDoc.FiveMEvents) >= 4` (similar)
  - Assert `len(sharedDoc.FiveMEvents) >= 3` (AddEventHandler ×2 + TriggerEvent)
  - Assert specific event: `sharedDoc.FiveMEvents[0].Name == "shared:configLoaded"` and `.Kind == FiveMEventAddHandler`
  - Run: `go test ./lsp/ -run TestFiveMEventScanner -count=1` → FAIL (no scanner yet)

  **GREEN (implement scanner)**:
  - Add scanner block in `lsp/workspace.go` inside `finalizeDocumentUpdate()`, after line 769 (`}` that closes FiveM exports scan), and before line 771 (`doc.ExportedNode = ...`)
  - Pattern: follow the exports scan exactly
  - Gated behind `if s.FeatureFiveM { ... }`
  - Reset: `doc.FiveMEvents = doc.FiveMEvents[:0]`
  - Iterate `tree.Nodes[1:]` looking for `ast.KindCallExpr`
  - Match function names by byte comparison against `doc.Source()`:
    - `[]byte("AddEventHandler")` → `FiveMEventAddHandler`
    - `[]byte("RegisterNetEvent")` → `FiveMEventRegisterNet`
    - `[]byte("TriggerEvent")` → `FiveMEventTriggerLocal`
    - `[]byte("TriggerServerEvent")` → `FiveMEventTriggerServer`
    - `[]byte("TriggerClientEvent")` → `FiveMEventTriggerClient`
  - Extract event name from first argument (like exports scan: `tree.ExtraList[node.Extra]`)
  - Use `unquoteLuaString()` for string argument extraction
  - For AddEventHandler and RegisterNetEvent: extract second argument NodeID as HandlerID
  - Append `FiveMEventInfo` to `doc.FiveMEvents`
  - After scanner: populate `FiveMResourceGraphNode` event slices for this document's resource
  - After scanner: populate `s.FiveMEventIndex` with event refs (keyed by `ast.HashBytes([]byte(eventName))`)
  - Run test → PASS

  **REFACTOR**:
  - Verify no allocations in the scan loop (byte comparison, no `string()` conversion before `unquoteLuaString`)
  - Extract scanner into helper method `scanEventCalls(doc *Document, tree *ast.Tree)` if >30 lines

  **Must NOT do**:
  - Do NOT scan for `RegisterServerEvent` (deprecated alias)
  - Do NOT scan for `TriggerLatentServerEvent` or `TriggerLatentClientEvent`
  - Do NOT scan for `RegisterNUICallback` (out of scope)
  - Do NOT allocate strings for comparison — use byte comparison
  - Do NOT place scanner before semantic resolution

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Core scanning logic — must integrate correctly with AST traversal and existing pipeline
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 9, 10, 11, 12 (LSP features depend on scanner data)
  - **Blocked By**: Task 1 (FiveMEventInfo type), Task 4 (fixture data)

  **References**:
  - `lsp/workspace.go:743-769` — FiveMLuaExports scan pattern (copy this exactly, adapt for events)
  - `lsp/workspace.go:679-690` — `finalizeDocumentUpdate` function signature
  - `lsp/fivem.go:805-829` — `unquoteLuaString` implementation (handles "", '', [[]])
  - `lsp/document.go:124-127` — `FiveMLuaExport` struct for comparison
  - `ast/tree.go` — check `Tree.Nodes`, `Tree.ExtraList`, `Node.Kind`, `Node.Left`, `Node.Count`, `Node.Extra` fields
  - `ast/ast.go` — `KindCallExpr`, `KindIdent`, `KindString` constants

  **Acceptance Criteria**:
  - [ ] RED: `TestFiveMEventScanner` fails before implementation
  - [ ] GREEN: Scanner populates `doc.FiveMEvents` for all 5 event functions
  - [ ] Event name extracted correctly (quotes stripped)
  - [ ] HandlerID captured for `AddEventHandler` and `RegisterNetEvent` (valid NodeID)
  - [ ] HandlerID is `ast.InvalidNode` for trigger-only events
  - [ ] `FiveMResourceGraphNode` event slices populated
  - [ ] `s.FiveMEventIndex` populated with hash-keyed event refs
  - [ ] Scanner gated behind `s.FeatureFiveM`
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventScanner -count=1` → PASS
  - [ ] No regression: `go test ./lsp/ -run TestFiveMFixtureHarness -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Scanner discovers all 5 event function types
    Tool: Bash (go test)
    Preconditions: resource_events fixture loaded and indexed
    Steps:
      1. clientDoc := h.docForMarker("client_registration")
      2. Count clientDoc.FiveMEvents with Kind == FiveMEventAddHandler → >= 1
      3. Count with Kind == FiveMEventRegisterNet → >= 1
      4. Count with Kind == FiveMEventTriggerServer → >= 1
      5. serverDoc := h.docForMarker("server_registration")
      6. Count serverDoc.FiveMEvents with Kind == FiveMEventTriggerClient → >= 1
      7. sharedDoc := h.docForMarker("shared_registration")
      8. Count sharedDoc.FiveMEvents with Kind == FiveMEventTriggerLocal → >= 1
    Expected Result: All 5 event kinds discovered across fixtures
    Evidence: .sisyphus/evidence/task-5-scanner-kinds.txt

  Scenario: Event name extraction handles quoted strings correctly
    Tool: Bash (go test)
    Preconditions: scanner processes fixture files
    Steps:
      1. Find FiveMEventInfo with Name == "client:playerLoaded"
      2. Assert Kind == FiveMEventAddHandler
      3. Assert HandlerID != ast.InvalidNode (callback captured)
      4. Find FiveMEventInfo with Name == "shared:requestSync"
      5. Assert at least one has Kind == FiveMEventTriggerServer
    Expected Result: Event names extracted without quotes, correct kinds
    Evidence: .sisyphus/evidence/task-5-scanner-names.txt

  Scenario: Non-event function calls are NOT captured as events
    Tool: Bash (go test)
    Preconditions: Document with non-event function calls
    Steps:
      1. Check sharedDoc.FiveMEvents — no entry with Name "print" or "data"
      2. Verify only actual AddEventHandler/TriggerEvent/etc calls are captured
    Expected Result: No false positives from unrelated function calls
    Evidence: .sisyphus/evidence/task-5-scanner-precision.txt
  ```

  **Commit**: YES (Wave 2 group)
  - Message: `feat(fivem): implement event scanner and diagnostics`
  - Files: `lsp/workspace.go`, `lsp/fivem_events_test.go`

- [x] 6. Event direction diagnostic — TDD

  **What to do**:

  **RED (write failing test first)**:
  - Add `TestFiveMEventDirectionDiagnostic` to `fivem_events_test.go`
  - Use fixture harness with `resource_events` + known direction error at `server_direction_error` marker
  - Assert diagnostic with code `"fivem-event-direction"` is published for that position
  - Assert message contains direction hint ("server script should use TriggerEvent instead of TriggerServerEvent")
  - Run: `go test ./lsp/ -run TestFiveMEventDirectionDiagnostic -count=1` → FAIL

  **GREEN (implement diagnostic)**:
  - Add diagnostic publisher in `lsp/diagnostics.go` (within `if s.FeatureFiveM` block, after existing FiveM diagnostics)
  - Gated behind `s.DiagFiveMEventDirection`
  - For each document with FiveM profile:
    - If profile is `FiveMProfileServer`: warn on `TriggerServerEvent` calls (should use `TriggerEvent` for local)
    - If profile is `FiveMProfileClient`: warn on `TriggerClientEvent` calls (should use `TriggerEvent` for local)
    - Skip for `FiveMProfileShared` (ambiguous context)
    - Skip for `FiveMProfilePlainLua` and `FiveMProfileManifest`
  - Diagnostic code: `"fivem-event-direction"`
  - Severity: Warning (not Error)
  - Message: `"'{funcName}' called from {side} script — use 'TriggerEvent' for same-side events or verify this is intentional"`
  - Position: at the function call identifier node (left node of call expression)
  - Run test → PASS

  **REFACTOR**:
  - Extract profile-to-direction mapping into helper: `isCrossSideCall(kind FiveMEventKind, profile FiveMExecutionProfileKind) bool`

  **Must NOT do**:
  - Do NOT emit errors — only warnings
  - Do NOT warn on `TriggerEvent` (always local, always valid)
  - Do NOT warn on cross-side calls that are intentional (`TriggerServerEvent` from client, `TriggerClientEvent` from server)
  - Do NOT warn on shared files

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Diagnostic logic with profile awareness — must handle shared/ambiguous cases correctly

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 7, 8 — all diagnostics are independent)
  - **Parallel Group**: Wave 2
  - **Blocks**: Task 16 (manifest invalidation re-tests diagnostics)
  - **Blocked By**: Task 3 (config flags), Task 5 (scanner data)

  **References**:
  - `lsp/diagnostics.go:116-192` — existing FiveM diagnostic pattern (gating, code, message, position)
  - `lsp/fivem.go` — `FiveMExecutionProfile` and `classifyDocumentEnv()` for profile checking
  - `lsp/workspace.go:743-769` — exports scan for understanding call expression structure

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: diagnostic fires for `TriggerServerEvent` in server files
  - [ ] GREEN: diagnostic fires for `TriggerClientEvent` in client files
  - [ ] No diagnostic for `TriggerServerEvent` in client files (correct direction)
  - [ ] No diagnostic for `TriggerClientEvent` in server files (correct direction)
  - [ ] No diagnostic for `TriggerEvent` anywhere
  - [ ] No diagnostic for shared files
  - [ ] Diagnostic gated behind `s.DiagFiveMEventDirection`
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventDirectionDiagnostic -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: TriggerServerEvent in server file warns
    Tool: Bash (go test)
    Preconditions: resource_events fixture with server_direction_error marker
    Steps:
      1. h := newFiveMFixtureHarness(t, "resource_events")
      2. marker := h.requireMarker("server_direction_error")
      3. diags := h.diagnostics("resource_events/server.lua")
      4. Assert hasDiagnosticCode(diags, "fivem-event-direction") == true
      5. Assert diagnostic range covers the function call
    Expected Result: Warning diagnostic on TriggerServerEvent call in server file
    Evidence: .sisyphus/evidence/task-6-direction-warning.txt

  Scenario: Cross-side calls from correct side produce no warning
    Tool: Bash (go test)
    Preconditions: client.lua has TriggerServerEvent (correct: client→server)
    Steps:
      1. diags := h.diagnostics("resource_events/client.lua")
      2. Assert no diagnostic with code "fivem-event-direction" for client_hover marker
    Expected Result: No false positive for correct cross-side calls
    Evidence: .sisyphus/evidence/task-6-no-false-positive.txt

  Scenario: Shared files suppress direction warnings
    Tool: Bash (go test)
    Preconditions: shared.lua with events (ambiguous side context)
    Steps:
      1. diags := h.diagnostics("resource_events/shared.lua")
      2. Assert no diagnostic with code "fivem-event-direction"
    Expected Result: Shared files never trigger direction warnings
    Evidence: .sisyphus/evidence/task-6-shared-suppression.txt
  ```

  **Commit**: YES (Wave 2 group)
  - Message: `feat(fivem): implement event scanner and diagnostics`
  - Files: `lsp/diagnostics.go`, `lsp/fivem_events_test.go`

- [x] 7. Unregistered net event diagnostic — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMUnregisteredNetEventDiagnostic` to `fivem_events_test.go`
  - Use fixture harness with `resource_events`
  - Assert diagnostic `"fivem-unregistered-net-event"` fires when `TriggerServerEvent("someEvent")` exists but no `RegisterNetEvent("someEvent")` in same resource's opposite-side files
  - Run → FAIL

  **GREEN**:
  - Add diagnostic publisher in `lsp/diagnostics.go`
  - Gated behind `s.DiagFiveMUnregisteredNetEvent`
  - For each resource, collect:
    - All network triggers: `TriggerServerEvent` (from client scripts) and `TriggerClientEvent` (from server scripts)
    - All network registrations: `RegisterNetEvent` calls (any script in the resource)
  - For each network trigger event name, check if `RegisterNetEvent` exists for that name in the same resource
  - If not found: emit diagnostic at the trigger call site
  - Diagnostic code: `"fivem-unregistered-net-event"`
  - Severity: Warning
  - Message: `"Network event '{name}' triggered but no RegisterNetEvent found in this resource — receiving side will not receive it"`
  - **IMPORTANT**: Only validate within the same resource (not cross-resource) — this is the MVP scope

  **Must NOT do**:
  - Do NOT validate cross-resource RegisterNetEvent gating
  - Do NOT flag `RegisterNetEvent` without matching triggers (informational only)
  - Do NOT validate `TriggerEvent` (local events don't need RegisterNetEvent)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Cross-document validation within a resource — must correctly associate files by resource root

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 6, 8)
  - **Parallel Group**: Wave 2
  - **Blocks**: None directly
  - **Blocked By**: Task 3 (config flags), Task 5 (scanner data)

  **References**:
  - `lsp/diagnostics.go` — existing diagnostic publishing pattern
  - `lsp/fivem.go` — `FiveMResourceGraph` and `FiveMResourceGraphNode` for resource grouping
  - `lsp/workspace.go` — `getDocResourceRoot()` to group documents by resource
  - `lsp/symbols.go:975-1017` — `getFiveMResourceExportDefinitions()` for cross-document resolution pattern

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: diagnostic fires for unregistered network events
  - [ ] No diagnostic when RegisterNetEvent exists for the triggered event
  - [ ] Only validates within same resource (not cross-resource)
  - [ ] Only validates TriggerServerEvent and TriggerClientEvent (not TriggerEvent)
  - [ ] Diagnostic gated behind `s.DiagFiveMUnregisteredNetEvent`
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMUnregisteredNetEventDiagnostic -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Unregistered network event triggers warning
    Tool: Bash (go test)
    Preconditions: Resource where client triggers net event but server doesn't register it
    Steps:
      1. Create fixture with client TriggerServerEvent("unregisteredNetEvent") and no RegisterNetEvent
      2. diags := h.diagnostics("resource_events/client.lua")
      3. Assert diagnostic code "fivem-unregistered-net-event" found
    Expected Result: Diagnostic fires for network event without registration
    Evidence: .sisyphus/evidence/task-7-unregistered-warning.txt

  Scenario: Registered network event produces no warning
    Tool: Bash (go test)
    Preconditions: resource_events fixture — "shared:syncData" has RegisterNetEvent
    Steps:
      1. client has TriggerServerEvent("shared:syncData")
      2. server has RegisterNetEvent("shared:syncData")
      3. Assert no "fivem-unregistered-net-event" diagnostic for that event
    Expected Result: No false positive when registration exists
    Evidence: .sisyphus/evidence/task-7-registered-clean.txt
  ```

  **Commit**: YES (Wave 2 group)
  - Message: `feat(fivem): implement event scanner and diagnostics`
  - Files: `lsp/diagnostics.go`, `lsp/fivem_events_test.go`

- [x] 8. Unknown event diagnostic — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMUnknownEventDiagnostic` to `fivem_events_test.go`
  - Verify diagnostic `"fivem-unknown-event"` fires for `AddEventHandler("totallyUnknownEvent", ...)` where event name is not in workspace triggers nor built-in events
  - Verify NO diagnostic for known events (in triggers or built-in)
  - Run → FAIL

  **GREEN**:
  - Add diagnostic publisher in `lsp/diagnostics.go`
  - Gated behind `s.DiagFiveMUnknownEvent`
  - For each `AddEventHandler` or `RegisterNetEvent` call:
    - Check if event name exists in:
      1. Same resource's trigger events (from scanner)
      2. Other resources' trigger events (from `FiveMEventIndex`)
      3. Built-in events (`EventsBuiltin`)
    - If NOT found in any of the above: emit diagnostic
  - Diagnostic code: `"fivem-unknown-event"`
  - Severity: Information (lowest severity — event might come from external resource or be dynamically dispatched)
  - Message: `"Event '{name}' is not triggered anywhere in the workspace — verify this is intentional"`
  - Special case: wildcard handler `"*"` — skip (it's always valid)
  - Position: at the event name string argument

  **Must NOT do**:
  - Do NOT emit as error or warning — informational only
  - Do NOT flag events that exist as built-in FiveM events
  - Do NOT flag wildcard `"*"` handlers
  - Do NOT flag events triggered by external resources (outside workspace) — informational is sufficient

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Multi-source lookup (per-resource + cross-resource + built-in) — moderate complexity

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 6, 7)
  - **Parallel Group**: Wave 2
  - **Blocks**: None directly
  - **Blocked By**: Task 2 (built-in events), Task 3 (config flags), Task 5 (scanner + event index)

  **References**:
  - `lsp/diagnostics.go` — existing diagnostic pattern
  - `lsp/symbols.go:975-1017` — cross-document symbol resolution for lookup pattern
  - `lsp/fivem.go` — `EventsBuiltin` map for built-in event names
  - `lsp/server.go` — `FiveMEventIndex` for O(1) cross-resource lookup

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: diagnostic fires for truly unknown events
  - [ ] No diagnostic for events found in same-resource triggers
  - [ ] No diagnostic for events found in cross-resource triggers
  - [ ] No diagnostic for built-in events (e.g. "playerConnecting")
  - [ ] No diagnostic for wildcard `"*"` handlers
  - [ ] Diagnostic is Information severity (not Warning or Error)
  - [ ] Diagnostic gated behind `s.DiagFiveMUnknownEvent`
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMUnknownEventDiagnostic -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Unknown event handler flagged as informational
    Tool: Bash (go test)
    Preconditions: Resource with AddEventHandler("totallyUnknown")
    Steps:
      1. Create fixture with AddEventHandler("completelyUnknownEvent", handler)
      2. diags := h.diagnostics("...")
      3. Assert diagnostic code "fivem-unknown-event" found
      4. Assert severity == Information
    Expected Result: Low-severity informational diagnostic
    Evidence: .sisyphus/evidence/task-8-unknown-info.txt

  Scenario: Known event handler produces no diagnostic
    Tool: Bash (go test)
    Preconditions: resource_events fixture — "client:playerLoaded" has a trigger
    Steps:
      1. diags := h.diagnostics("resource_events/client.lua")
      2. Assert no "fivem-unknown-event" for "client:playerLoaded"
    Expected Result: Known events not flagged
    Evidence: .sisyphus/evidence/task-8-known-clean.txt

  Scenario: Built-in game events not flagged as unknown
    Tool: Bash (go test)
    Preconditions: AddEventHandler("playerConnecting", handler)
    Steps:
      1. diags := h.diagnostics(...)
      2. Assert no "fivem-unknown-event" for "playerConnecting"
    Expected Result: Built-in events recognized as valid
    Evidence: .sisyphus/evidence/task-8-builtin-recognized.txt
  ```

  **Commit**: YES (Wave 2 group)
  - Message: `feat(fivem): implement event scanner and diagnostics`
  - Files: `lsp/diagnostics.go`, `lsp/fivem_events_test.go`

- [x] 9. Hover on event name strings — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMEventHover` to `fivem_events_test.go`
  - Use `h.hover("client_hover")` — the `--[[@client_hover]]` marker is on a `TriggerServerEvent("shared:requestSync")` call
  - Assert hover is non-nil
  - Assert hover content contains "shared:requestSync"
  - Assert hover content contains trigger type info ("TriggerServerEvent" or "client → server")
  - Assert for `shared_hover` marker (on `TriggerEvent("shared:reloadUI")` in shared file) — hover content contains "shared:reloadUI"
  - Run → FAIL

  **GREEN**:
  - In `lsp/features.go`, find the hover handler (search for `handleHover` or `textDocument/hover`)
  - Add FiveM event hover logic within `if s.FeatureFiveM` block, AFTER AST resolution check
  - Detect when cursor is positioned on a string literal argument in one of the 5 event function calls
  - Strategy:
    1. Get AST token at position → check if it's a string
    2. Walk up to parent call expression → check if it's an event function
    3. Extract event name via `unquoteLuaString`
  - Look up event info from:
    - `doc.FiveMEvents` (per-document events)
    - `s.FiveMEventIndex[ast.HashBytes([]byte(name))]` (workspace-wide)
    - `EventsBuiltin[name]` (built-in)
  - Compose hover text using Markdown:
    ```markdown
    ## Event: `{name}`
    **Subset**: {CLIENT|SERVER|SHARED}
    **Type**: {Handler|Trigger (local)|Trigger (client→server)|Trigger (server→client)}

    Handlers in workspace: N
    - `{uri}:{line}`

    Triggers in workspace: N
    - `{uri}:{line}`

    {built-in description if applicable}
    ```
  - Run test → PASS

  **Must NOT do**:
  - Do NOT modify hover behavior for non-event strings
  - Do NOT show NUI callback hover info

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Feature integration into existing hover handler — must understand AST position resolution

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 10, 11, 12, 13, 14)
  - **Parallel Group**: Wave 3
  - **Blocks**: Task 15 (shared file testing uses hover)
  - **Blocked By**: Task 5 (scanner data)

  **References**:
  - `lsp/features.go` — existing `handleHover` implementation (find via symbol search `textDocument/hover`)
  - `lsp/symbols.go:975-1017` — cross-document lookup pattern
  - `lsp/fivem.go:805-829` — `unquoteLuaString` for event name extraction
  - `lsp/fivem_fixture_harness_test.go:66-69` — existing `h.hover()` test pattern

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: hover shows event name, subset, type
  - [ ] Hover shows handler and trigger counts
  - [ ] Hover shows built-in description for game events
  - [ ] Hover shows file locations of handlers/triggers
  - [ ] No hover for non-event string literals
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventHover -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Hover on TriggerServerEvent shows cross-side info
    Tool: Bash (go test)
    Preconditions: resource_events fixture indexed
    Steps:
      1. hover := h.hover("client_hover")
      2. Assert hover != nil
      3. Assert strings.Contains(hover.Contents.Value, "shared:requestSync")
      4. Assert strings.Contains(hover.Contents.Value, "TriggerServerEvent") or "client → server"
    Expected Result: Rich hover with event name, type, and direction
    Evidence: .sisyphus/evidence/task-9-hover-trigger.txt

  Scenario: Hover on AddEventHandler shows handler info
    Tool: Bash (go test)
    Preconditions: resource_events fixture
    Steps:
      1. hover := h.hover("shared_registration")
      2. Assert hover != nil
      3. Assert strings.Contains(hover.Contents.Value, "shared:configLoaded")
      4. Assert strings.Contains(hover.Contents.Value, "Handler")
    Expected Result: Hover shows handler type with handler count
    Evidence: .sisyphus/evidence/task-9-hover-handler.txt

  Scenario: Hover on non-event string produces no event hover
    Tool: Bash (go test)
    Preconditions: Document with print("hello") call
    Steps:
      1. Place marker on "hello" string
      2. hover := h.hover("non_event_marker")
      3. Assert hover is nil or does not contain FiveM event content
    Expected Result: Non-event strings unaffected
    Evidence: .sisyphus/evidence/task-9-hover-no-false.txt
  ```

  **Commit**: YES (Wave 3 group)
  - Message: `feat(fivem): wire event LSP features`
  - Files: `lsp/features.go`, `lsp/fivem_events_test.go`

- [x] 10. Completion for event names — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMEventCompletion` to `fivem_events_test.go`
  - Use fixture harness with `resource_events`
  - Create a new file with `AddEventHandler("--[[@completion_marker]]` and complete at the marker
  - Assert completion items include known event names from workspace:
    - `"client:playerLoaded"`, `"shared:configLoaded"`, `"shared:requestSync"`, `"server:playerReady"`
  - Assert completion items include built-in events:
    - `"playerConnecting"`, `"onResourceStart"`
  - Assert each completion item has `detail` showing subset and source
  - Run → FAIL

  **GREEN**:
  - In `lsp/features.go`, find the completion handler (`handleCompletion` or `textDocument/completion`)
  - Add FiveM event completion within `if s.FeatureFiveM` block
  - Detect completion context: cursor is inside a string argument to one of the 5 event functions
  - Strategy:
    1. Parse the line: check if preceding text matches `AddEventHandler("` / `TriggerEvent("` / etc.
    2. Or: check AST for parent call expression at cursor position
  - Collect event names from:
    - `s.FiveMEventIndex` → all known workspace event names
    - `EventsBuiltin` → built-in event names
  - Format completion items:
    - `label`: event name (e.g. `"playerConnecting"`)
    - `detail`: `"SERVER — Built-in"` or `"SHARED — 3 handlers"` etc.
    - `kind`: `CompletionItemKind.Event` (14)
    - `sortText`: prioritize exact matches, then prefix matches, then built-ins
  - Run test → PASS

  **Must NOT do**:
  - Do NOT include NUI callback names in completion
  - Do NOT include wildcard `"*"` as a completion suggestion
  - Do NOT trigger event completion outside of event function call context

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Completion integration — must correctly identify context and merge multiple sources

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 9, 11, 12, 13, 14)
  - **Parallel Group**: Wave 3
  - **Blocks**: None directly
  - **Blocked By**: Task 5 (scanner + event index)

  **References**:
  - `lsp/features.go` — existing `handleCompletion` implementation
  - `lsp/symbols.go:1019-1054` — `getFiveMResourceExportNames()` for name collection pattern
  - `lsp/fivem_fixture_harness_test.go:71-74` — existing `h.completion()` test pattern
  - `lsp/fivem.go` — `EventsBuiltin` map for built-in event names

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: completion lists workspace event names
  - [ ] GREEN: completion lists built-in event names
  - [ ] Completion items have informative detail (subset, source)
  - [ ] Completion uses `CompletionItemKind.Event`
  - [ ] Completion only triggers inside event function string arguments
  - [ ] No duplicate entries for same event name across multiple resources
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventCompletion -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Completion inside AddEventHandler shows event names
    Tool: Bash (go test)
    Preconditions: resource_events fixture indexed
    Steps:
      1. completion := h.completion("completion_marker")  -- inside AddEventHandler("|")
      2. Assert completionHasLabel(completion, "client:playerLoaded")
      3. Assert completionHasLabel(completion, "shared:requestSync")
    Expected Result: All known event names appear as completion items
    Evidence: .sisyphus/evidence/task-10-completion-addhandler.txt

  Scenario: Completion inside TriggerEvent shows same event names
    Tool: Bash (go test)
    Preconditions: resource_events fixture
    Steps:
      1. completion := h.completion("trigger_completion_marker")
      2. Assert same event names available (handlers and triggers use same namespace)
    Expected Result: Consistent completion across all event functions
    Evidence: .sisyphus/evidence/task-10-completion-trigger.txt

  Scenario: Built-in events appear in completion
    Tool: Bash (go test)
    Preconditions: resource_events fixture
    Steps:
      1. completion := h.completion("completion_marker")
      2. Assert completionHasLabel(completion, "playerConnecting")
      3. Assert completionHasLabel(completion, "onResourceStart")
    Expected Result: Built-in events available alongside workspace events
    Evidence: .sisyphus/evidence/task-10-completion-builtin.txt
  ```

  **Commit**: YES (Wave 3 group)
  - Message: `feat(fivem): wire event LSP features`
  - Files: `lsp/features.go`, `lsp/fivem_events_test.go`

- [x] 11. Go-to-definition on event names — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMEventGoToDef` to `fivem_events_test.go`
  - Use fixture harness with `resource_events`
  - Place marker on `TriggerServerEvent("shared:syncData")` in client.lua (`client_hover`)
  - Assert definition resolves to `RegisterNetEvent("shared:syncData")` in server.lua (`server_net_registration`)
  - Place marker on `AddEventHandler("shared:reloadUI")` in shared.lua
  - Assert definition resolves to `TriggerEvent("shared:reloadUI")` in shared.lua
  - Run → FAIL

  **GREEN**:
  - In `lsp/features.go`, find the definition handler (`handleDefinition` or `textDocument/definition`)
  - Add FiveM event definition logic within `if s.FeatureFiveM` block
  - Detect cursor on an event name string inside one of the 5 event functions
  - Lookup logic:
    - For `AddEventHandler("X")` → find `RegisterNetEvent("X")` first, then `TriggerEvent("X")`/`TriggerServerEvent("X")`/`TriggerClientEvent("X")`
    - For `RegisterNetEvent("X")` → find `AddEventHandler("X")` handlers
    - For `TriggerEvent("X")` → find `AddEventHandler("X")` or `RegisterNetEvent("X")`
  - Use `s.FiveMEventIndex[ast.HashBytes([]byte(name))]` to find all matching event refs
  - Return the first matching definition with a different kind (handler → trigger, trigger → handler)
  - Run test → PASS

  **Must NOT do**:
  - Do NOT resolve event name strings to the stdlib function definition
  - Do NOT break existing go-to-def behavior for non-event symbols

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Definition resolution — must correctly map between event kinds

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 9, 10, 12, 13, 14)
  - **Parallel Group**: Wave 3
  - **Blocks**: None directly
  - **Blocked By**: Task 5 (scanner + event index)

  **References**:
  - `lsp/features.go` — existing definition handler
  - `lsp/symbols.go:975-1017` — `getFiveMResourceExportDefinitions()` for cross-resource resolution
  - `lsp/fivem_fixture_harness_test.go:64-65` — existing `h.requireSingleDefinitionAt()` test pattern

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: go-to-def from trigger resolves to handler definition
  - [ ] GREEN: go-to-def from handler resolves to trigger or RegisterNetEvent
  - [ ] Go-to-def works across resources (client → server, etc.)
  - [ ] Go-to-def returns single location when only one match (e.g., RegisterNetEvent)
  - [ ] Go-to-def on non-event string does nothing (existing behavior preserved)
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventGoToDef -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: TriggerEvent jumps to AddEventHandler definition
    Tool: Bash (go test)
    Preconditions: resource_events fixture
    Steps:
      1. h.requireSingleDefinitionAt("shared_hover", "shared_registration")
      2. Assert definition position matches the handler
    Expected Result: Jump from trigger to handler
    Evidence: .sisyphus/evidence/task-11-def-trigger-to-handler.txt

  Scenario: TriggerServerEvent jumps to RegisterNetEvent
    Tool: Bash (go test)
    Preconditions: client has TriggerServerEvent("shared:syncData"), server has RegisterNetEvent("shared:syncData")
    Steps:
      1. h.requireSingleDefinitionAt("client_hover", "server_net_registration")
      2. Assert definition is the RegisterNetEvent call in server file
    Expected Result: Cross-resource jump from client trigger to server registration
    Evidence: .sisyphus/evidence/task-11-def-cross-resource.txt
  ```

  **Commit**: YES (Wave 3 group)
  - Message: `feat(fivem): wire event LSP features`
  - Files: `lsp/features.go`, `lsp/fivem_events_test.go`

- [x] 12. Find-references on event names — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMEventFindRefs` to `fivem_events_test.go`
  - Use fixture harness with `resource_events`
  - Place marker on `RegisterNetEvent("shared:syncData")` in server.lua
  - Assert `h.references()` returns at least 2 locations: the server `RegisterNetEvent` and the client `TriggerServerEvent`
  - Run → FAIL

  **GREEN**:
  - In `lsp/features.go`, find the references handler (`handleReferences` or `textDocument/references`)
  - Add FiveM event references logic within `if s.FeatureFiveM` block
  - Detect cursor on event name string inside event function
  - Look up `s.FiveMEventIndex[ast.HashBytes([]byte(name))]` for all matching event refs
  - Return all matching locations as `[]Location`:
    - Include declaration if `includeDeclaration` context flag is true
    - Each location has URI from `FiveMEventRef.URI` and range from node position
  - Run test → PASS

  **Must NOT do**:
  - Do NOT include non-event references (regular Lua references should still work)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: References resolution — workspace-wide lookup from event index

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 9, 10, 11, 13, 14)
  - **Parallel Group**: Wave 3
  - **Blocks**: None
  - **Blocked By**: Task 5 (event index)

  **References**:
  - `lsp/features.go` — existing references handler
  - `lsp/symbols.go` — existing symbol lookups using GlobalIndex
  - `lsp/fivem_fixture_harness_test.go` — marker-based test pattern

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: find-refs returns all handlers and triggers for an event
  - [ ] Works across resources and files
  - [ ] Works for all 5 event kinds
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventFindRefs -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Find-references returns all sites for shared event
    Tool: Bash (go test)
    Preconditions: resource_events fixture — "shared:syncData" referenced in client and server
    Steps:
      1. refs := h.references("server_net_registration")
      2. Assert len(refs) >= 2
      3. Assert at least one ref is in client.lua (TriggerServerEvent)
      4. Assert at least one ref is in server.lua (RegisterNetEvent)
    Expected Result: Cross-file, cross-resource reference collection
    Evidence: .sisyphus/evidence/task-12-refs-cross-file.txt
  ```

  **Commit**: YES (Wave 3 group)
  - Message: `feat(fivem): wire event LSP features`
  - Files: `lsp/features.go`, `lsp/fivem_events_test.go`

- [x] 13. Workspace symbols for events — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMEventWorkspaceSymbols` to `fivem_events_test.go`
  - Use fixture harness with `resource_events`
  - Query workspace symbols for event names
  - Assert results include `"client:playerLoaded"`, `"shared:syncData"`, `"server:playerReady"`, `"shared:configLoaded"`
  - Assert results include built-in events like `"playerConnecting"`
  - Run → FAIL

  **GREEN**:
  - In `lsp/symbols.go`, find the workspace symbols handler (`handleWorkspaceSymbol` or `workspace/symbol`)
  - Add FiveM event symbols within `if s.FeatureFiveM` and appropriate query matching
  - When query matches event names (by prefix or contains):
    - Iterate `s.FiveMEventIndex` to find matching event names
    - Also check `EventsBuiltin` for matching built-in events
    - Return each matching event as `SymbolInformation`:
      - `Name`: event name (e.g. `"playerConnecting"`)
      - `Kind`: `SymbolKind.Event` (24)
      - `Location`: URI of one representative handler or trigger, or a synthetic location for built-ins
  - Run test → PASS

  **Must NOT do**:
  - Do NOT return duplicate entries for same event across multiple resources (deduplicate by name)
  - Do NOT break existing workspace symbol queries

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Workspace symbol integration — must merge two sources and deduplicate

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 9-12, 14)
  - **Parallel Group**: Wave 3
  - **Blocks**: None
  - **Blocked By**: Task 5 (event index), Task 1 (FiveMEventKind)

  **References**:
  - `lsp/symbols.go` — existing workspace symbol handler
  - `lsp/symbols.go:1019-1054` — `getFiveMResourceExportNames()` name collection pattern
  - `lsp/fivem.go` — `EventsBuiltin` for built-in event names

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: workspace symbols include event names
  - [ ] Uses `SymbolKind.Event` (24)
  - [ ] Built-in events appear alongside workspace events
  - [ ] No duplicate entries for same event name
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventWorkspaceSymbols -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Ctrl+T search finds event by name
    Tool: Bash (go test)
    Preconditions: resource_events fixture indexed
    Steps:
      1. symbols := h.workspaceSymbols("client:player")
      2. Assert any match has Name == "client:playerLoaded"
      3. Assert match.Kind == SymbolKind.Event
    Expected Result: Event name appears in workspace symbol search
    Evidence: .sisyphus/evidence/task-13-symbols-search.txt
  ```

  **Commit**: YES (Wave 3 group)
  - Message: `feat(fivem): wire event LSP features`
  - Files: `lsp/symbols.go`, `lsp/fivem_events_test.go`

- [x] 14. Code Lens for events — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMEventCodeLens` to `fivem_events_test.go`
  - Use fixture harness with `resource_events`
  - Request code lens for server.lua
  - Assert code lens appears above `RegisterNetEvent("shared:syncData")` showing reference count
  - Assert code lens appears above `AddEventHandler` calls showing handler count
  - Run → FAIL

  **GREEN**:
  - In `lsp/features.go`, find the code lens handler (`handleCodeLens` or `textDocument/codeLens`)
  - Add FiveM event code lens within existing code lens provider
  - Gated behind `s.FeatureCodeLens` (existing flag — no new flag needed)
  - For each `AddEventHandler` or `RegisterNetEvent` call in the document:
    - Look up event name in `s.FiveMEventIndex[ast.HashBytes([]byte(name))]`
    - Count total refs (triggers + other handlers) for this event
    - Create code lens: `"N references"` above the handler
    - Command: if clicked, execute `textDocument/references` for this event
  - Run test → PASS

  **Must NOT do**:
  - Do NOT show "0 references" code lens for handlers with no known triggers

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Code lens integration — must compute and display reference counts

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 9-13)
  - **Parallel Group**: Wave 3
  - **Blocks**: None
  - **Blocked By**: Task 5 (event index)

  **References**:
  - `lsp/features.go` — existing code lens handler (search for `codeLens` or `CodeLens`)
  - `lsp/server.go:110` — `FeatureCodeLens` flag
  - Existing code lens patterns in the codebase for other reference counts

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: code lens shows reference count above handlers
  - [ ] Code lens shows "N references" where N > 0
  - [ ] No code lens for handlers with 0 known references
  - [ ] Code lens uses existing `FeatureCodeLens` flag
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventCodeLens -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Handler with known triggers shows reference count
    Tool: Bash (go test)
    Preconditions: resource_events fixture — "shared:syncData" has at least 1 trigger
    Steps:
      1. lenses := h.codeLens("resource_events/server.lua")
      2. Assert any lens matches "shared:syncData" with "1 references" title
    Expected Result: Code lens shows accurate reference count
    Evidence: .sisyphus/evidence/task-14-codelens-count.txt

  Scenario: Handler with no triggers shows no code lens
    Tool: Bash (go test)
    Preconditions: Event handler that no one triggers
    Steps:
      1. lenses := h.codeLens("resource_events/client.lua")
      2. Assert no code lens for "client:playerLoaded" (no triggers found) — or if found, verify it shows "0 references" and decide if that's acceptable
      3. If "0 references" is shown: verify code lens exists
      4. If not: verify absence is intentional
    Expected Result: Behavior consistent — either shows 0 or is absent
    Evidence: .sisyphus/evidence/task-14-codelens-zero.txt
  ```

  **Commit**: YES (Wave 3 group)
  - Message: `feat(fivem): wire event LSP features`
  - Files: `lsp/features.go`, `lsp/fivem_events_test.go`

- [x] 15. Shared file ambiguity handling — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMSharedFileEvents` to `fivem_events_test.go`
  - Use `resource_events` fixture — shared.lua has `AddEventHandler("shared:configLoaded")` and `TriggerEvent("shared:reloadUI")`
  - Assert events from shared files appear in both client AND server event index lookups
  - Assert direction warnings are NOT fired for any events in shared files (even cross-side calls)
  - Assert unknown-event diagnostic is NOT fired for shared-file events (they're inherently "known" to both sides)
  - Run → FAIL

  **GREEN**:
  - In scanner (`workspace.go`): when profile is `FiveMProfileShared`, add event refs to BOTH `SharedHandlers` and `SharedTriggers` on the resource graph node
  - In event index: ensure shared events are indexed with a marker indicating they're visible from both sides
  - In diagnostics: skip direction warnings for documents with `FiveMProfileShared`
  - In unknown-event diagnostic: treat shared-file event registrations as valid for both client and server contexts
  - Run test → PASS

  **Must NOT do**:
  - Do NOT suppress ALL diagnostics for shared files — only direction and unknown-event
  - Do NOT duplicate event entries in the index (one entry should serve both lookups)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Edge case handling — shared files have ambiguous side context that must be handled gracefully

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 16, 17)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: Task 5 (scanner), Task 6 (direction diagnostic), Task 8 (unknown event diagnostic)

  **References**:
  - `lsp/workspace.go:743-769` — scanner with profile awareness
  - `lsp/diagnostics.go` — existing diagnostic gating by profile
  - `lsp/fivem.go` — `FiveMProfileShared` constant and `classifyDocumentEnv()`

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation
  - [ ] GREEN: shared file events visible from client context
  - [ ] GREEN: shared file events visible from server context
  - [ ] No direction warnings for shared files
  - [ ] No unknown-event diagnostics for shared file events
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMSharedFileEvents -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Shared file event is visible from both sides
    Tool: Bash (go test)
    Preconditions: resource_events fixture indexed
    Steps:
      1. sharedDoc := h.docForMarker("shared_registration")
      2. Profile should be FiveMProfileShared
      3. Look up "shared:configLoaded" — must be found from both client-perspective and server-perspective lookups
    Expected Result: Shared events accessible from both context queries
    Evidence: .sisyphus/evidence/task-15-shared-visibility.txt

  Scenario: TriggerServerEvent in shared file gets no direction warning
    Tool: Bash (go test)
    Preconditions: shared.lua has TriggerServerEvent call
    Steps:
      1. diags := h.diagnostics("resource_events/shared.lua")
      2. Assert no diagnostic with code "fivem-event-direction"
    Expected Result: Direction warnings suppressed for shared files
    Evidence: .sisyphus/evidence/task-15-shared-no-warning.txt
  ```

  **Commit**: YES (Wave 4 group)
  - Message: `feat(fivem): handle event edge cases and perf budget`
  - Files: `lsp/workspace.go`, `lsp/diagnostics.go`, `lsp/fivem_events_test.go`

- [x] 16. Manifest invalidation re-scans events — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMEventManifestInvalidation` to `fivem_events_test.go`
  - Start with resource where a file is classified as client
  - Assert event data is populated with client-side events
  - Change the manifest to reclassify the file as server (`server_script 'client.lua'` instead of `client_script 'client.lua'`)
  - Re-index via `h.reindex()`
  - Assert event data is re-scanned (previous event data cleared, new data matches new profile)
  - Assert diagnostics update (direction warnings may change)
  - Run → FAIL

  **GREEN**:
  - No additional scanner code needed — `finalizeDocumentUpdate` already re-runs on reindex
  - Verify that `doc.FiveMEvents = doc.FiveMEvents[:0]` correctly resets event data before re-scan
  - Verify that `FiveMResourceGraphNode` event slices are cleared/repopulated when the resource is re-processed
  - Verify diagnostic publishers re-run on re-publish after manifest change
  - Run test → PASS

  **Must NOT do**:
  - Do NOT add explicit invalidation logic — the existing pipeline already handles this via `finalizeDocumentUpdate` re-running
  - Verify the existing pattern works, don't add new complexity

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Pipeline integration testing — verify the existing invalidation mechanism handles events correctly

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 15, 17)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: Task 5 (scanner), Task 6-8 (diagnostics)

  **References**:
  - `lsp/fivem_fixture_harness_test.go:81-109` — `TestFiveMFixtureWarmReindex` pattern (warm reindex with manifest change)
  - `lsp/fivem_fixture_harness_test.go:88-100` — writing updated manifest and calling `h.reindex()`
  - `lsp/workspace.go:743-744` — event data reset pattern
  - `lsp/fivem.go` — `FiveMProfileCached` invalidation logic

  **Acceptance Criteria**:
  - [ ] RED: test fails before implementation (or verifies existing behavior)
  - [ ] GREEN: manifest change triggers event re-scan
  - [ ] GREEN: old event data cleared on re-scan
  - [ ] GREEN: new profile reflected in event data
  - [ ] GREEN: diagnostics update after reindex
  - [ ] Test passes: `go test ./lsp/ -run TestFiveMEventManifestInvalidation -count=1` → PASS

  **QA Scenarios**:

  ```
  Scenario: Changing manifest from client to server updates events
    Tool: Bash (go test)
    Preconditions: Resource where client.lua initially classified as client
    Steps:
      1. Build initial state: client profile, client events captured
      2. Write new manifest: make same file a server_script
      3. h.reindex()
      4. Assert doc profile is now FiveMProfileServer
      5. Assert event data reflects new profile (e.g., TriggerServerEvent now flagged as direction mismatch)
    Expected Result: All event data reflects new profile classification
    Evidence: .sisyphus/evidence/task-16-manifest-change.txt

  Scenario: Diagnostics update after reindex with new manifest
    Tool: Bash (go test)
    Preconditions: Same as above
    Steps:
      1. Before reindex: assert client.lua has no direction warning for TriggerServerEvent
      2. Write manifest: make it a server_script
      3. h.reindex()
      4. After reindex: assert client.lua NOW has direction warning for TriggerServerEvent (since it's now classified as server)
    Expected Result: Diagnostics update to reflect new file classification
    Evidence: .sisyphus/evidence/task-16-diag-update.txt
  ```

  **Commit**: YES (Wave 4 group)
  - Message: `feat(fivem): handle event edge cases and perf budget`
  - Files: `lsp/fivem_events_test.go`

- [x] 17. Perf budget for event scanning — TDD

  **What to do**:

  **RED**:
  - Add `TestFiveMEventScanPerf` to `lsp/fivem_perf_test.go` (near existing perf tests)
  - Generate a large Lua file with 10,000 lines and 500 `AddEventHandler`/`TriggerEvent` calls intermixed
  - Measure time to run `finalizeDocumentUpdate` for this file WITH event scanning enabled
  - Assert that event scanning adds ≤ 1ms overhead compared to baseline (exports-only scan)
  - Run → FAIL (if overhead exceeds budget, or test verifies budget after optimization)

  **GREEN**:
  - Benchmark the scanner: use Go's `testing.B` for `BenchmarkEventScan`
  - Optimize if needed:
    - Use byte comparison (already planned) — no string allocation in hot loop
    - Check that scanner gating (`s.FeatureFiveM`) prevents work on non-FiveM files
    - Verify no `string()` conversion before `unquoteLuaString` — pass `doc.Source()[start:end]` directly
  - Set perf budget: event scanning overhead < 1ms on 10k-line file
  - Run test → PASS (within budget)

  **Must NOT do**:
  - Do NOT introduce allocations in the scan loop
  - Do NOT degrade overall `finalizeDocumentUpdate` time beyond acceptable threshold

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Performance testing — requires benchmarking and potential optimization

  **Parallelization**:
  - **Can Run In Parallel**: YES (with Task 15, 16)
  - **Parallel Group**: Wave 4
  - **Blocks**: None
  - **Blocked By**: Task 5 (scanner implementation)

  **References**:
  - `lsp/fivem_perf_test.go` — existing perf budget tests (line ~350 has FiveM-related perf test)
  - `lsp/fivem_runtime_abi_test.go:235` — existing large test pattern with multiple globals
  - `lsp/workspace.go:743-769` — the scanner loop we're measuring

  **Acceptance Criteria**:
  - [ ] RED: overhead measured (may exceed budget initially)
  - [ ] GREEN: overhead within 1ms on 10k-line file
  - [ ] No allocations in the event scan loop (verify with `testing.B.ReportAllocs()`)
  - [ ] Scanner gating prevents work on non-FiveM files (verify 0 events found in plain Lua)
  - [ ] `go test ./lsp/ -run TestFiveMEventScanPerf -count=1` → PASS
  - [ ] `go test ./lsp/ -bench BenchmarkEventScan -count=1` → shows ns/op within budget

  **QA Scenarios**:

  ```
  Scenario: Large file with 500 events scans under 1ms
    Tool: Bash (go test)
    Preconditions: Generated 10k-line Lua file with 500 event calls
    Steps:
      1. Run benchmark: go test ./lsp/ -bench BenchmarkEventScan -benchtime=10x
      2. Assert ns/op < 1,000,000 (1ms)
      3. Assert B/op == 0 (no allocations)
    Expected Result: Sub-millisecond scan with zero allocations
    Evidence: .sisyphus/evidence/task-17-perf-budget.txt

  Scenario: Plain Lua files incur zero event scan overhead
    Tool: Bash (go test)
    Preconditions: Non-FiveM Lua file (no manifest, plain lua)
    Steps:
      1. Index plain Lua file
      2. Assert len(doc.FiveMEvents) == 0
      3. Measure time — should be near-zero (scanner exits early)
    Expected Result: Scanner early-exits for non-FiveM files
    Evidence: .sisyphus/evidence/task-17-plain-skip.txt
  ```

  **Commit**: YES (Wave 4 group)
  - Message: `feat(fivem): handle event edge cases and perf budget`
  - Files: `lsp/fivem_perf_test.go`, `lsp/workspace.go` (if optimization needed)

---

## Final Verification Wave

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, `go test`). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in `.sisyphus/evidence/`. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `go vet ./lsp/`. Review all changed files for: `any` usage, empty `if err != nil { continue }` without logging, commented-out code, unused imports. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Vet [PASS/FAIL] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration (events working together, not isolation). Test edge cases: empty workspace, shared files, manifest change. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1 — everything in spec was built (no missing), nothing beyond spec was built (no creep). Check "Must NOT do" compliance. Detect cross-task contamination. Flag unaccounted changes.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | Unaccounted [CLEAN/N files] | VERDICT`

---

## Commit Strategy

- **Wave 1**: `feat(fivem): add event data structures and built-in definitions` — document.go, fivem.go, server.go, messages.go, testdata/
- **Wave 2**: `feat(fivem): implement event scanner and diagnostics` — workspace.go, diagnostics.go, fivem_events_test.go
- **Wave 3**: `feat(fivem): wire event LSP features (hover, completion, goto-def, refs, symbols, codelens)` — features.go, symbols.go, fivem_events_test.go
- **Wave 4**: `feat(fivem): handle event edge cases and perf budget` — workspace.go, fivem_events_test.go, fivem_perf_test.go

---

## Success Criteria

### Verification Commands
```bash
go test ./lsp/ -run "TestFiveMEvent|TestFiveMFixtureHarness" -count=1  # Expected: ALL PASS
go vet ./lsp/                                                          # Expected: no new warnings
```

### Final Checklist
- [x] All "Must Have" present
- [x] All "Must NOT Have" absent
- [x] All tests pass (`go test ./lsp/`)
- [x] No regressions in existing FiveM fixtures
- [x] Perf budget not exceeded
