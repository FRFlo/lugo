# FiveM specification set

This directory contains reference specifications for FiveM's implementation.

These files are intended to describe the implementation precisely enough to support tooling such as language servers.

The specifications cover the custom Lua runtime, the event system, NUI callbacks, and related infrastructure.

## Files

### Lua runtime

- `lua-runtime-reference.md`
  - runtime bootstrap
  - opened libraries
  - injected globals
  - scheduler model
  - custom `io` and `os` behaviour on server
- `lua-stdlib-reference.md`
  - standard Lua 5.4 library surface
  - C serialisation extensions (`msgpack`, `json`)
  - `Citizen` host-binding table
  - scheduler-installed globals (event system, exports, NUI, state bags)
  - promise library
  - server-only custom `io` and `os` libraries
  - lazy-loaded native function globals
  - removed and replaced globals
- `lua-native-binding-reference.md`
  - native-build selection
  - lazy native global materialisation
  - argument marshaling
  - result coercion
- `lua-manifest-reference.md`
  - `fxmanifest.lua` and `__resource.lua`
  - manifest loader state
  - directive interception model
  - metadata emission and source locations
- `lua-resource-management-reference.md`
  - resource discovery
  - metadata expansion
  - runtime attachment
  - lifecycle events
  - dependencies, provides, exports, client download flow
- `lua-function-reference-reference.md`
  - callable proxy tables
  - function-reference serialisation
  - async return adaptation
  - events, exports, NUI, and state-bag callback transport
- `lua-environment-isolation-reference.md`
  - resource-level runtime boundaries
  - client/server separation
  - cross-resource bridges
  - cleanup guarantees and non-guarantees

### Event system

- `event-system-reference.md`
  - event dispatch architecture (TriggerEvent, QueueEvent, CancelEvent)
  - ResourceEventManagerComponent and ResourceEventComponent
  - ServerEventComponent network broadcast
  - EventReassemblyComponent fragmentation
  - IScriptEventRuntime interface
  - NETEV documentation pipeline
- `event-api-reference.md`
  - Lua, JavaScript, and C# event APIs
  - handler registration, triggering, cancellation
  - network event gating
  - cross-language equivalence table
- `game-events-reference.md`
  - catalog of all NETEV-documented game events with type signatures
  - server events (playerConnecting, entityCreating, weaponDamageEvent, etc.)
  - client events (populationPedCreating, gameEventTriggered, entityDamaged, etc.)
  - shared resource lifecycle events
  - internal system events and chat events
- `nui-callback-reference.md`
  - NUI callback registration (legacy vs ref-based)
  - `__cfx_nui:` event routing and strict mode
  - result callback transport
  - SendNUIMessage push events
  - SetNuiFocus input control

## Source basis

The reference set is derived from the implementation in:

- `code/components/citizen-scripting-lua/`
- `code/components/citizen-scripting-core/`
- `code/components/citizen-resources-core/`
- `code/components/citizen-resources-metadata-lua/`
- `code/components/citizen-server-impl/`
- `code/components/citizen-legacy-net-resources/`
- `code/components/nui-resources/`
- `code/components/gta-net-five/`
- `code/components/net/`
- `code/vendor/lua.lua`
- `data/shared/citizen/scripting/`
- `ext/natives/`
- `ext/event-doc-gen/`
- `ext/typings/`
- `ext/system-resources/chat/`

## Documentation type

All files in this directory are Reference documentation.

They describe behaviour and structure. They do not provide tutorials or how-to workflows.
