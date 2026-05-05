# FiveM event system reference

## Scope

This document describes the event dispatch architecture implemented by FiveM in the following components:

- `code/components/citizen-scripting-core/src/ResourceEventComponent.cpp`
- `code/components/citizen-scripting-core/src/ResourceEventManagerComponent.cpp`
- `code/components/citizen-server-impl/src/ServerEventComponent.cpp`
- `code/components/citizen-scripting-core/src/EventScriptFunctions.cpp`
- `code/components/citizen-legacy-net-resources/src/ResourceNetBindings.cpp`
- `code/components/net/src/EventReassemblyComponent.cpp`
- `ext/event-doc-gen/index.js`
- `code/components/citizen-scripting-core/include/fxScripting.idl`

It is a Reference document. It describes the event dispatch lifecycle, cancellation model, network transport, fragmentation, handler registration, and the type-documentation pipeline.

## Architecture overview

The event system has three tiers:

1. **ResourceEventComponent** — per-resource handler table, manages which events a resource listens to, and wires resource lifecycle hooks (start/stop) into the event system.
2. **ResourceEventManagerComponent** — global event router, dispatches triggered events to all registered handlers across all resources, supports queued delivery and cancellation.
3. **ServerEventComponent** — server-only network transport, sends events from server to connected clients over the game network protocol.

The scripting runtimes (Lua, JS, C#) access the event system through native invocations that call `ResourceEventManagerComponent` methods.

## Client / Server event isolation

FiveM runs as two completely separate processes:

- **Server process** (`FXServer`) — hosts the `citizen-server-impl` components, including the server-side `ResourceEventManagerComponent`. Server resources run here.
- **Client process** (`FiveM.exe`) — hosts the game engine, `citizen-scripting` components, and the client-side `ResourceEventManagerComponent` and `ServerEventComponent`. Client resources run here.

Each process has its **own independent event manager instance**. Events do not cross between client and server automatically.

### What stays on the same side

- `TriggerEvent` / `TriggerEventInternal` — dispatches within the same process only. A `TriggerEvent("myEvent", ...)` called on the server reaches only server-side handlers. Called on the client, it reaches only client-side handlers.
- `CancelEvent()` / `WasEventCanceled()` — operate on the local event manager's thread-local state. Canceling an event on one side has no effect on the other.
- `AddEventHandler` / `RegisterNetEvent` — register handlers only in the local process. A server handler cannot receive a client-only event and vice versa.

### What crosses the boundary

- `TriggerServerEvent` — client → server. The client serializes arguments via msgpack and sends them over the game network protocol. The server receives the packet, deserializes, and triggers the event in the server's event manager.
- `TriggerClientEvent` — server → client. The server serializes arguments and sends them to the target client(s) via `ServerEventComponent`. The client deserializes and triggers the event in its event manager.
- `TriggerLatentServerEvent` / `TriggerLatentClientEvent` — same directions as above, but large payloads are fragmented by `EventReassemblyComponent` before transmission.

### Network event gating

Even when an event crosses the boundary, the receiving side applies a network safety gate: handlers must be explicitly registered as network-safe via `RegisterNetEvent`. Otherwise the event is silently dropped. This prevents a server from triggering arbitrary events on clients that did not opt in, and vice versa.

### Side-specific infrastructure

| Component | Exists on |
|---|---|
| `ResourceEventManagerComponent` | Both server and client (separate instances) |
| `ResourceEventComponent` | Both server and client (per-resource) |
| `ServerEventComponent` | Server only (sends events from server to connected clients via network) |
| `EventReassemblyComponent` | Both server and client (for latent events in both directions) |
| NUI callback system | Client only (CEF browser only exists on client) |

### Consequences for language servers

- A server script CANNOT listen for a `CLIENT`-subset event, and a client script CANNOT listen for a `SERVER`-subset event. The C++ trigger code literally runs on only one side.
- `TriggerEvent` never triggers handlers on the other side.
- Calling `TriggerServerEvent` from a server script is a silent no-op. Same for `TriggerClientEvent` from a client script.
- The `*` wildcard handler on one side does not receive events triggered on the other side.

## ResourceEventComponent — per-resource registration

### Handler storage

Each `ResourceEventComponent` maintains a set of event names that its owning resource handles. This set is populated by:

- `AddResourceHandledEvent(resourceName, eventName)` called by the global event manager
- Calls from `REGISTER_RESOURCE_AS_EVENT_HANDLER` in `EventScriptFunctions.cpp`, which calls both `AddResourceHandledEvent` and `ResourceScriptingComponent::AddHandledEvent`

### Resource lifecycle wiring

The component translates resource lifecycle callbacks into global events:

| Lifecycle callback | Event(s) triggered | Mode |
|---|---|---|
| `OnBeforeStart(resourceName)` | `onResourceStarting` with resource name | Cancelable |
| `OnStart(resourceName)` | `onClientResourceStart` AND `onResourceStart` with resource name | Queued |
| `OnStop(resourceName)` | `onClientResourceStop` AND `onResourceStop` with resource name | Queued |

The dual-trigger for `OnStart` means that both `onClientResourceStart` and `onResourceStart` fire for every resource, regardless of context. The distinction between client and server is in the event name, not in whether the event fires.

### Event queuing

The component uses `tbb::concurrent_queue<EventData>` for queued events. Each `EventData` entry contains:

- `eventName` — the string event name
- `eventPayload` — msgpack-serialized payload buffer
- `eventPayloadLen` — payload length in bytes
- `source` — source identifier string

Queued events are drained each tick. Handlers for queued events receive the event via `TriggerEvent`, making them subject to the same cancellation and routing logic as immediately triggered events.

## ResourceEventManagerComponent — global dispatch

### TriggerEvent — synchronous dispatch

`TriggerEvent(eventName, payload, payloadLen, source)` is the primary dispatch entry point. It returns `bool`: `true` if the event was delivered, `false` if it was canceled.

Dispatch sequence:

1. If `source` is not provided, the current resource name is used as the implicit source.
2. **Global handlers** (`m_globalEventHandlers`) are invoked first. These are handlers that listen to all events via the `*` wildcard.
3. **Per-resource handlers** are invoked for each resource that registered for the specific `eventName`.
4. After each handler invocation, the cancellation flag is checked. If set, dispatch stops immediately and the method returns `false`.

### QueueEvent — deferred dispatch

`QueueEvent(eventName, payload, payloadLen, source)` pushes the event onto the concurrent queue. It is processed during the next Tick cycle by the queued event handler, which invokes `TriggerEvent` per-handler.

This is the mechanism used by the resource lifecycle events (`onClientResourceStart`, `onServerResourceStart`, `onClientResourceStop`, `onServerResourceStop`) to avoid re-entrant resource lifecycle callbacks during resource loading.

### NUI event routing (Strict Mode)

The dispatch logic checks whether the event name begins with `__cfx_nui:`:

```
eventName.find("__cfx_nui:") != 0
```

Events prefixed with `__cfx_nui:` **bypass the wildcard (`*`) global handler routing**. They are only delivered to resources that explicitly registered for the specific `__cfx_nui:` event name. This ensures NUI callbacks do not leak to resources that listen for all events.

### Event cancellation

The event manager maintains a thread-local stack of cancellation states. This is critical because event dispatch can be re-entrant (a handler can trigger another event).

- `CancelEvent()` — sets the cancellation flag on the current thread-local stack frame. Must be called from within an event handler currently being dispatched.
- `WasEventLastCanceled()` — returns the cancellation state of the most recent `TriggerEvent` or `QueueEvent` dispatch on the calling thread.

Cancellation is checked after each handler invocation. The first handler to call `CancelEvent()` causes all subsequent handlers for that event to be skipped.

### Template convenience methods

`TriggerEvent2` and `QueueEvent2` are variadic template wrappers that use msgpack to pack arbitrary argument types into a payload buffer before calling the corresponding base method.

## ServerEventComponent — network broadcast

Server-side only. Provides methods to send events from the server to connected game clients:

- `TriggerClientEvent(eventName, payload, payloadLen, targetSrc)` — sends the event to the specified client. If `targetSrc` is `-1`, the event is broadcast to all connected clients.

The component uses the game network protocol (NetGameEvent-based transmission) to deliver events. The event payload is msgpack-serialized before network transport.

### Client-to-server path

Client-side scripts use `TriggerServerEvent` variants, which route through different mechanisms:

- **Regular server events**: Sent as NetGameEvent packets from client to server, received by server-side event dispatch.
- **Latent server events**: Routed through `EventReassemblyComponent` for large payloads, using msgpack framing and fragment reassembly.

## EventReassemblyComponent — large event fragmentation

Handles events with payloads exceeding approximately 1 KB. The system splits large events into fragments and reassembles them on the receiving side.

### Fragment parameters

| Parameter | Value | Description |
|---|---|---|
| `kFragmentSize` (v1) | 1182 bytes | Maximum fragment payload size in version 1 framing |
| Fragment max (v2) | 1536 bytes | Maximum fragment size in version 2 framing |
| Max fragment overall | ~1536 bytes | Hard limit for any version |

### Fragmentation (sender side)

1. Event payload is serialized via msgpack.
2. If payload size exceeds the fragment threshold, it is split into fragments.
3. Fragments are tracked using `eastl::bitvector` for acknowledgment.
4. Sending is rate-limited by the configured `bytesPerSecond` parameter.
5. Separate send lists are maintained for v1 and v2 protocol versions.
6. v2 protocol uses `net::ByteWriter` instead of `rl::MessageBuffer` for framing.

### Reassembly (receiver side)

1. `HandlePacket` and `HandlePacketV2` process incoming fragments.
2. Fragments are buffered per-event until all fragments arrive.
3. Reassembly completes, and the full event is delivered.
4. Incomplete reassemblies are cleaned up after a 2-minute timeout.

### Pending event limits

The `maxPendingEvents` field controls how many pending reassemblies can be in-flight simultaneously:

| Value | Behavior |
|---|---|
| `0` | Blocked — no new latent events accepted |
| `0xFF` | Unlimited — no cap on pending events |
| Other | Maximum number of concurrent pending reassemblies |

### Interface

`TriggerLatentServerEventInternal` and `TriggerLatentClientEventInternal` in `ResourceNetBindings.cpp` use this component to send large events. The `EnableEventReassemblyChanged` callback responds to the `sv_enableNetEventReassembly` convar to enable or disable the latent event system.

## Event handler registration (native level)

### REGISTER_RESOURCE_AS_EVENT_HANDLER

Defined in `EventScriptFunctions.cpp`. This native:

1. Calls `eventManager->AddResourceHandledEvent(resourceName, eventName)` to register in the global event manager.
2. Calls `ResourceScriptingComponent::AddHandledEvent(eventName)` to register in the resource's scripting component.

Handler registration is idempotent at the global manager level — registering the same event name for the same resource multiple times has no additional effect.

### TriggerEvent native path

`TRIGGER_EVENT_INTERNAL` calls `eventManager->TriggerEvent(eventName, payload, payloadLen)`. The payload is already msgpack-serialized by the scripting runtime before the native invocation.

### CancelEvent native path

`CANCEL_EVENT` calls `eventManager->CancelEvent()`. This must be called during handler dispatch. Calling it outside a dispatch has no meaningful effect (the cancellation flag is thread-local and scoped to the current dispatch).

### WasEventCanceled native path

`WAS_EVENT_CANCELED` calls `eventManager->WasEventLastCanceled()`. This returns whether the most recent event dispatch on the calling thread was canceled by any handler.

## IScriptEventRuntime interface

Defined in `fxScripting.idl`, this is the low-level boundary between the host (C++) and script runtimes (Lua, JS, C#):

```
IScriptEventRuntime : IScriptRuntime
{
    TriggerEvent(
        charPtr eventName,
        charPtr argsSerialized,
        uint32_t serializedSize,
        charPtr sourceId
    );
};
```

Parameters:

- `eventName` — the event name string
- `argsSerialized` — msgpack-serialized argument buffer
- `serializedSize` — size of the serialized buffer in bytes
- `sourceId` — the source identifier (typically `"net:<netId>"` for network-sourced events, or resource name)

Each runtime (Lua `LuaScriptRuntime`, JS `V8ScriptRuntime`, C# `ClrScriptRuntime`) implements this interface to receive events from the host and dispatch them to the appropriate script-level handlers.

## NETEV documentation pipeline

The `ext/event-doc-gen/index.js` tool extracts type documentation from C++ source files.

### Marker format

NETEV markers are placed in C++ comments above the `TriggerEvent` or `TriggerEvent2` calls:

```
/*NETEV eventName SUBSET
 *
 * embedded TypeScript type definition
 *
 */
```

Where:

- `eventName` is the event name string
- `SUBSET` is one of `CLIENT`, `SERVER`, or `SHARED`
- The TypeScript definition describes the event payload type

### Extraction process

1. Read all `.cpp` files in the configured source directories.
2. Parse `/*NETEV` comment blocks.
3. Extract the embedded TypeScript type definitions.
4. Generate Typedoc-compatible output that documents each event with its type signature.
5. Events not marked with `NETEV` are excluded from generated documentation.

### Subset semantics

| Subset | Meaning |
|---|---|
| `CLIENT` | Event is available in client scripts only |
| `SERVER` | Event is available in server scripts only |
| `SHARED` | Event is available in both client and server scripts |

The subset is a documentation convention — it does not enforce availability at runtime. The runtime will attempt to dispatch any event regardless of subset marking.

## Guarantees

The implementation guarantees the following observable properties:

- `TriggerEvent` dispatches synchronously and returns cancellation state.
- `QueueEvent` defers dispatch to the next Tick cycle.
- `CancelEvent()` only affects the current dispatch on the calling thread.
- `WasEventLastCanceled()` reflects the most recent dispatch on the calling thread.
- `__cfx_nui:` events bypass wildcard (`*`) global handler routing.
- Resource lifecycle events (`onResourceStart`, `onResourceStop`, and their `Client`/`Server` variants) are queued, not immediately triggered.
- Latent events exceeding the fragment threshold are split into fragments and reassembled by `EventReassemblyComponent`.
- Incomplete latent event reassemblies are cleaned up after 2 minutes.
- Event payloads cross the scripting/host boundary as msgpack-serialized buffers.

## Non-guarantees

The implementation does **not** guarantee:

- That `NETEV` subset markings are enforced at runtime (they are documentation-only).
- That event delivery order is preserved across different resources.
- That canceled events leave no side effects in handlers that ran before cancellation.
- That network event delivery is reliable (packets may be dropped by the game network layer).
- That latent events complete delivery (fragments may time out).

## Relationship to other specifications

- `specs/event-api-reference.md` describes the Lua, JS, and C# scripting APIs that invoke these event system methods.
- `specs/game-events-reference.md` catalogs all default game events triggered by the host, including their type signatures.
- `specs/nui-callback-reference.md` describes the NUI callback system that uses `__cfx_nui:` events.
- `specs/lua-function-reference-reference.md` describes how callable values are transported through event payloads.
- `specs/lua-resource-management-reference.md` describes the resource lifecycle that triggers `onResourceStarting` and related events.
