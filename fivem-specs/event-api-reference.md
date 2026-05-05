# FiveM event scripting API reference

## Scope

This document describes the event-related APIs available in FiveM's scripting runtimes:

- **Lua**: `data/shared/citizen/scripting/lua/scheduler.lua`
- **JavaScript**: `ext/typings/citizen/client/main.js` and `ext/typings/citizen/server/main.js`
- **C#**: `ext/clrcore-v2/` and `ext/natives/schemas/netlib_*/schema.json`

It is a Reference document. It describes each API surface, its parameters, return values, and invocation semantics. It does not provide tutorials or how-to guides.

## Lua event API

All Lua event functions are defined in `scheduler.lua` and made available as global functions in every resource runtime.

### AddEventHandler

```lua
AddEventHandler(eventName, eventRoutine)
```

Registers a handler function for the given event name.

- `eventName` (string) — the event to listen for
- `eventRoutine` (function) — the callback function. Receives the unpacked event arguments.

Returns a table `{ key = index, name = eventName }` that can be passed to `RemoveEventHandler`.

**Dispatch mechanism**: The handler is stored in the `eventHandlers` local table, keyed by `eventName`. When the host delivers an event to the runtime via `IScriptEventRuntime::TriggerEvent`, the scheduler unpacks the msgpack payload and invokes all registered handlers for that event name sequentially.

**Wildcard handlers**: The `*` event name is reserved for global handlers. Handlers registered for `*` receive ALL events. The event name is prepended as the first argument to the callback before the unpacked payload. The global handler mechanism is implemented in `ResourceEventManagerComponent`, not in the Lua scheduler.

**Net-safe handling**: Handlers registered via `RegisterNetEvent` are stored with a `netSafe` flag. The scheduler checks this flag when receiving network-sourced events. If an event arrives from the network and no handler has the `netSafe` flag for that event name, the event is dropped. This prevents malicious or accidental cross-resource event listening.

### RemoveEventHandler

```lua
RemoveEventHandler(eventData)
```

Removes a previously registered event handler.

- `eventData` (table) — the table returned by `AddEventHandler`.

**Internal**: Looks up `eventData.key` in the `eventHandlers` table and removes the entry. If the event name no longer has any registered handlers, the event name entry is also removed from the `eventHandlers` table.

### RegisterNetEvent

```lua
RegisterNetEvent(eventName)
RegisterNetEvent(eventName, callback)
```

Marks an event as safe to receive from the network, and optionally registers a handler.

- `eventName` (string) — the event name to mark as network-safe
- `callback` (function, optional) — if provided, registers this function as a handler for the event

**Two-argument form**: Equivalent to calling `RegisterNetEvent(eventName)` followed by `AddEventHandler(eventName, callback)`.

**Restricted events**: The following event names cannot be registered as network-safe:
- `__cfx_internal:commandFallback`

If `eventName` matches one of these restricted patterns, `RegisterNetEvent` silently ignores the registration.

**Network event gate**: When an event arrives from the network (via `TriggerClientEvent` or `TriggerServerEvent`), the scheduler checks whether any handler for that event name has the `netSafe` flag before delivering the event. This gate is the mechanism that prevents resources from receiving network events they did not explicitly register for.

### RegisterServerEvent

```lua
RegisterServerEvent(eventName, callback)
```

Alias for `RegisterNetEvent`. Both names refer to the same underlying function. The alias exists for semantic clarity in server-side scripts.

### TriggerEvent

```lua
TriggerEvent(eventName, ...)
```

Triggers an event locally (same-side, within the current resource and any other resource on the same side).

- `eventName` (string) — the event name
- `...` (variadic) — arguments to pass to handlers

**Internal sequence**:

1. Arguments are serialized with `msgpack_pack_args(...)`.
2. `TriggerEventInternal` native is called with the event name and serialized payload.
3. This invokes `ResourceEventManagerComponent::TriggerEvent`, which dispatches to all registered handlers for `eventName`.

**Function arguments**: If any argument is a Lua function, it is packed as `EXT_FUNCREF` (msgpack extension type 10). The receiving handler receives a callable proxy table, not a native Lua closure. See `specs/lua-function-reference-reference.md` for details.

### TriggerServerEvent

```lua
TriggerServerEvent(eventName, ...)
```

Client-side only. Sends an event from the client to the server.

- `eventName` (string) — the event name
- `...` (variadic) — arguments to pass to server-side handlers

**Internal**: Arguments are serialized with `msgpack_pack_args(...)`, then `TriggerServerEventInternal` native is called. This sends the event over the game network to the server, where it is processed by `ServerEventComponent`.

**Fails silently if called on the server**: The function exists in the server scheduler but has no effect.

### TriggerClientEvent

```lua
TriggerClientEvent(eventName, playerId, ...)
```

Server-side only. Sends an event from the server to one or all clients.

- `eventName` (string) — the event name
- `playerId` (number) — the target player's server ID. Use `-1` to broadcast to all clients.
- `...` (variadic) — arguments to pass to client-side handlers

**Internal**: Arguments are serialized with `msgpack_pack_args(...)`, then `TriggerClientEventInternal` native is called with the target player ID.

**Fails silently if called on the client**: The function exists in the client scheduler but has no effect.

### TriggerLatentServerEvent

```lua
TriggerLatentServerEvent(eventName, bytesPerSecond, ...)
```

Client-side only. Sends a potentially large event from the client to the server using the latent event system.

- `eventName` (string) — the event name
- `bytesPerSecond` (number) — transmission rate limit in bytes per second
- `...` (variadic) — arguments to pass to server-side handlers

**Internal**: Uses `EventReassemblyComponent` to fragment the payload if it exceeds the fragment threshold. Transmits fragments at a rate not exceeding `bytesPerSecond`. Reassembled by the server before delivery.

### TriggerLatentClientEvent

```lua
TriggerLatentClientEvent(eventName, playerId, bytesPerSecond, ...)
```

Server-side only. Sends a potentially large event from the server to one or all clients using the latent event system.

- `eventName` (string) — the event name
- `playerId` (number) — the target player's server ID. Use `-1` to broadcast to all clients.
- `bytesPerSecond` (number) — transmission rate limit in bytes per second
- `...` (variadic) — arguments to pass to client-side handlers

**Internal**: Same fragmentation and rate-limiting mechanism as `TriggerLatentServerEvent`, but in the server-to-client direction.

### CancelEvent

```lua
CancelEvent()
```

Cancels the current event being dispatched. Must be called from within an event handler during dispatch.

**Internal**: Calls the `CANCEL_EVENT` native, which sets the thread-local cancellation flag in `ResourceEventManagerComponent`. The event manager checks this flag after each handler invocation and stops dispatching if it is set.

### WasEventCanceled

```lua
WasEventCanceled()
```

Returns whether the most recent event dispatch on the calling thread was canceled.

**Returns**: `boolean` — `true` if the last event dispatch was canceled by any handler.

**Internal**: Calls the `WAS_EVENT_CANCELED` native.

## JavaScript event API

All JS event functions are defined in `main.js` (with separate client and server implementations) and are available as global functions or as members of the `on` / `emit` objects.

### on / addEventListener

```javascript
on(eventName, callback)
addEventListener(eventName, callback)
```

Registers a handler function for the given event name. `on` and `addEventListener` are aliases.

- `eventName` (string) — the event to listen for
- `callback` (function) — the handler function

**Dispatch**: Same mechanism as Lua — handlers are stored in a local map and invoked when the host delivers the event via `IScriptEventRuntime::TriggerEvent`. The msgpack payload is deserialized to JS values before calling the callback.

### onNet / addNetEventListener

```javascript
onNet(eventName, callback)
addNetEventListener(eventName, callback)
```

Marks an event as network-safe and registers a handler. Equivalent to calling `RegisterNetEvent(eventName)` followed by `addEventListener(eventName, callback)`.

### RegisterNetEvent / RegisterServerEvent

```javascript
RegisterNetEvent(eventName)
RegisterServerEvent(eventName)
```

Marks an event name as safe to receive from the network.

- `eventName` (string) — the event name to mark as network-safe

**Internal**: Adds `eventName` to the `netSafeEventNames` Set. When an event arrives from the network, the runtime checks this set before delivering. Events not in the set are dropped.

### emit / TriggerEvent

```javascript
emit(eventName, ...args)
TriggerEvent(eventName, ...args)
```

Triggers an event locally.

- `eventName` (string) — the event name
- `...args` — arguments to pass to handlers

**Internal**: Arguments are serialized with `msgpack_pack_args(...)` and delivered via `TriggerEventInternal`.

### emitNet

```javascript
emitNet(eventName, source, ...args)
```

Sends an event across the network. Behavior depends on context:

- **Client-side**: Sends `TriggerServerEventInternal` with serialized arguments to the server.
- **Server-side**: Sends `TriggerClientEventInternal` with serialized arguments to the specified `source` (target player ID).

- `eventName` (string) — the event name
- `source` (number) — client-side: ignored; server-side: target player ID (`-1` for broadcast)
- `...args` — arguments to pass to handlers

### CancelEvent / WasEventCanceled

Same semantics as Lua. Available as `CancelEvent()` and `WasEventCanceled()`.

## C# event API

All C# event functions are defined in the `clrcore-v2` extension (specifically through `Events` class and `EventHandlerAttribute`).

### EventHandlerAttribute

```csharp
[EventHandler(string eventName)]
```

Attribute used to mark a method as an event handler. Applied to static methods in a resource's entry-point class.

- `eventName` (string) — the event name to handle

**Behavior**: At resource initialization, methods decorated with `[EventHandler]` are automatically registered as event handlers for the specified event name. The method must accept parameters matching the event payload.

### Events.RegisterEventHandler

```csharp
Events.RegisterEventHandler(string eventName, Action<...> handler)
```

Registers a handler for the given event name at runtime.

- `eventName` (string) — the event name
- `handler` (delegate) — the handler function

### Events.TriggerEvent

```csharp
Events.TriggerEvent(string eventName, params object[] args)
```

Triggers an event locally.

### Events.TriggerServerEvent

```csharp
Events.TriggerServerEvent(string eventName, params object[] args)
```

Client-side only. Sends an event to the server.

### Events.TriggerClientEvent

```csharp
Events.TriggerClientEvent(string eventName, params object[] args)
```

Server-side only. Sends an event to clients. Overloads exist for targeting specific players or all players.

## Cross-language equivalences

| Operation | Lua | JavaScript | C# |
|---|---|---|---|
| Register local handler | `AddEventHandler(name, cb)` | `on(name, cb)` | `Events.RegisterEventHandler(name, handler)` |
| Mark event as net-safe | `RegisterNetEvent(name)` | `RegisterNetEvent(name)` | (automatic via `[EventHandler]`) |
| Trigger local event | `TriggerEvent(name, ...)` | `emit(name, ...)` | `Events.TriggerEvent(name, args)` |
| Send to server | `TriggerServerEvent(name, ...)` | `emitNet(name, null, ...)` | `Events.TriggerServerEvent(name, args)` |
| Send to client(s) | `TriggerClientEvent(name, id, ...)` | `emitNet(name, id, ...)` | `Events.TriggerClientEvent(name, args)` |
| Send large to server | `TriggerLatentServerEvent(name, bps, ...)` | — | — |
| Send large to client(s) | `TriggerLatentClientEvent(name, id, bps, ...)` | — | — |
| Cancel current event | `CancelEvent()` | `CancelEvent()` | `CancelEvent()` |
| Check if canceled | `WasEventCanceled()` | `WasEventCanceled()` | `WasEventCanceled()` |

## Client/Server boundary

FiveM runs as two completely separate processes — a server (`FXServer`) and a client (`FiveM.exe`). Each process has its own independent scripting runtime and its own event manager. Event APIs are available on both sides, but their behavior depends on which side they are called from.

### Same-side APIs (work identically on both client and server)

These functions operate within the local process's event manager only. They never cross the network boundary:

| Function | Behavior |
|---|---|
| `AddEventHandler` / `on` | Registers a handler in the local event manager |
| `RemoveEventHandler` | Removes a handler from the local event manager |
| `RegisterNetEvent` | Marks an event as network-safe on the local side |
| `TriggerEvent` / `emit` | Triggers an event within the local process only |
| `CancelEvent` | Cancels the current dispatch on the local event manager |
| `WasEventCanceled` | Checks cancellation state on the local event manager |

### Cross-side APIs (direction-specific)

These functions send events across the network boundary. They are only meaningful when called from the correct side:

| Function | Valid on | Sends to | Called from other side |
|---|---|---|---|
| `TriggerServerEvent` | **Client** only | Server | Silent no-op |
| `TriggerClientEvent` | **Server** only | Client(s) | Silent no-op |
| `TriggerLatentServerEvent` | **Client** only | Server | Silent no-op |
| `TriggerLatentClientEvent` | **Server** only | Client(s) | Silent no-op |
| `emitNet` | Both | Context-dependent: client→server, server→client | Effective on both sides |

### Calling a cross-side API from the wrong side

When `TriggerServerEvent` is called from a **server** script, or `TriggerClientEvent` is called from a **client** script, the call is a **silent no-op** — no error is raised, and nothing happens. This is because the underlying native (`TriggerServerEventInternal` / `TriggerClientEventInternal`) resolves to a stub that does nothing on the side where it is not applicable.

Language servers should flag these as warnings or errors: calling a client-only API from server code (and vice versa) produces no runtime effect.

### Network event gating on the receiving side

Even when a cross-side event successfully arrives via the network, the receiving side applies another safety check: the handler must be explicitly marked as network-safe via `RegisterNetEvent`. See the "Network event gating" section below for details.

### Event handler scope

- A handler registered with `AddEventHandler` on the **server** only receives events triggered on the server (same-side `TriggerEvent`, or `TriggerServerEvent` from clients).
- A handler registered with `AddEventHandler` on the **client** only receives events triggered on the client (same-side `TriggerEvent`, or `TriggerClientEvent` from the server).
- There is no mechanism for a server handler to directly listen to a client-only event, and vice versa. Cross-side communication requires explicit use of `TriggerServerEvent` / `TriggerClientEvent`.

## Network event gating

All three runtimes implement the same network event gating mechanism:

1. When an event arrives from the network (via `TriggerClientEvent` / `TriggerServerEvent`), the host delivers it to the scripting runtime.
2. The runtime checks whether any handler for that event name was registered as network-safe.
3. If no handler is marked as network-safe, the event is silently dropped.
4. If at least one handler is marked, the event is delivered to all registered handlers.

This gating applies only to network-sourced events. Locally triggered events (`TriggerEvent`) bypass the gate and are delivered regardless of `RegisterNetEvent` status.

## Guarantees

The implementation guarantees the following observable properties:

- `RegisterNetEvent` marks an event name as network-safe and optionally registers a handler.
- `RegisterServerEvent` is a direct alias for `RegisterNetEvent` in all runtimes.
- Network-sourced events are dropped if no handler is marked as net-safe.
- Locally triggered events bypass the network gate.
- `CancelEvent()` is thread-local and only affects the current dispatch.
- All event arguments are serialized via msgpack when crossing the scripting/host boundary.
- Lua functions in event payloads become callable proxy tables at the receiving end.
- `TriggerLatent*` functions use `EventReassemblyComponent` for large payloads.
- The `__cfx_internal:commandFallback` event name cannot be registered as network-safe.

## Non-guarantees

The implementation does **not** guarantee:

- That `TriggerServerEvent` or `TriggerClientEvent` has any effect when called from the wrong side (server/client). They fail silently.
- That latent events complete delivery within any specific time bound.
- That event delivery order is preserved for network events across different players.
- That `WasEventCanceled()` returns a meaningful value if called outside any event dispatch.

## Relationship to other specifications

- `specs/event-system-reference.md` describes the host-level event dispatch architecture.
- `specs/game-events-reference.md` catalogs all default game events.
- `specs/nui-callback-reference.md` describes the NUI callback system.
- `specs/lua-function-reference-reference.md` describes how function arguments are transported through events.
- `specs/lua-runtime-reference.md` describes the scheduler that hosts event handler execution.
