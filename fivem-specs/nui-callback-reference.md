# FiveM NUI callback system reference

## Scope

This document describes the NUI (Native UI) callback system — a client-only infrastructure that enables communication between game resources (Lua/JS/C#) and CEF-based HTML/JavaScript user interfaces. It covers:

- `code/components/nui-resources/src/ResourceUICallbacks.cpp` — callback registration and dispatch
- `code/components/nui-resources/src/NUICallbacks_PushEvent.cpp` — NUI push events (CEF → resource)
- `code/components/nui-resources/src/ResourceUI.cpp` — resource-level NUI state management
- `code/components/citizen-scripting-core/src/ResourceEventManagerComponent.cpp` — `__cfx_nui:` event routing
- `code/components/citizen-scripting-core/src/ResourceCallbackComponent.cpp` — result callback transport
- `data/shared/citizen/scripting/lua/scheduler.lua` — Lua `RegisterNuiCallback` / `RegisterNUICallback`

It is a Reference document. It describes registration, dispatch, result handling, and the routing bypass behavior.

## Architecture overview

The NUI callback system bridges two domains:

1. **Game resource domain** — Lua/JS/C# scripts running inside the FiveM runtime
2. **Browser domain** — HTML/JavaScript running inside a CEF (Chromium Embedded Framework) browser instance rendered as a game overlay

**Client-side only.** The NUI callback system exists exclusively on the client process. CEF browser instances only run on the client (as a game overlay). There is no NUI infrastructure on the server. All NUI-related APIs (`RegisterNUICallback`, `RegisterNuiCallback`, `SendNUIMessage`, `SetNuiFocus`) are only meaningful in client scripts. Calling them from server scripts is a silent no-op.

Communication flows in two directions:

- **Browser → Game** (NUI callback): Browser JavaScript sends a POST request to `https://cfx-nui-{resourceName}/{callbackType}`, which the CEF handler routes to the C++ NUI callback system. The system dispatches it to registered resource handlers and returns a response.
- **Game → Browser** (NUI push): Game resources call `SendNUIMessage(data)` to push JSON data to the browser's JavaScript context.

## Callback registration (C++ layer)

### REGISTER_NUI_CALLBACK_TYPE

```cpp
REGISTER_NUI_CALLBACK_TYPE(callbackType)
```

Registers a **legacy** callback type. This creates a handler that uses the `__cfx_nui:` event mechanism.

**Behavior**:

1. A handler is registered under the key `callbackType` in the resource's NUI callback map.
2. When a NUI request arrives:
   - The JSON body is parsed.
   - The body is converted to msgpack.
   - A result callback reference is created via `ResourceCallbackComponent::CreateCallback(...)`.
   - The event `__cfx_nui:{callbackType}` is queued via `QueueEvent2` with payload `(postObject, resultCallback)`.

`resultCallback` is a function reference string in the form `_cfx_internal:0:{refId}`. When called from Lua with a return value, the value is msgpack-serialized, converted to JSON, and sent back as the HTTP response to the browser.

### REGISTER_NUI_CALLBACK

```cpp
REGISTER_NUI_CALLBACK(callbackType, callback)
```

Registers a **ref-based** callback type. This creates a handler that uses direct function reference invocation (not the `__cfx_nui:` event path).

**Behavior**:

1. A handler is registered under the key `callbackType` in the resource's NUI callback map.
2. When a NUI request arrives:
   - The JSON body is parsed and converted to msgpack.
   - A result callback reference is created via `ResourceCallbackComponent::CreateCallback(...)`.
   - The registered `callback` function reference is invoked via `CallReference<void>(ref, postObject, resultReference)`.

This path bypasses the event system entirely. The callback goes directly to the C++ function reference, without creating a `__cfx_nui:` event.

### MakeUICallback

Both registration macros use `MakeUICallback`, which creates a C++ closure that:

1. Receives the `body` (raw string), `headers` (HTTP headers), and `response` callback from the CEF request handler.
2. Parses the JSON body.
3. Converts the parsed JSON to msgpack format.
4. Creates a result callback reference via `ResourceCallbackComponent::CreateCallback(...)`.
5. Either queues a `__cfx_nui:` event (legacy path) or directly invokes the stored function reference (ref-based path).
6. The result callback reference, when invoked by the resource handler, serializes the return value and sends it as the HTTP response.

## NUI event routing (Strict Mode)

In `ResourceEventManagerComponent::TriggerEvent`, NUI events receive special routing treatment:

```
if (eventName.find("__cfx_nui:") != 0)
{
    // dispatch to wildcard handlers
}
else
{
    // skip wildcard handlers
}
```

Events prefixed with `__cfx_nui:` **bypass the global wildcard (`*`) handler routing**. They are only delivered to resources that explicitly registered for the specific `__cfx_nui:{type}` event name.

This means:

- A resource that listens to `*` (all events) will NOT receive NUI callbacks directed to other resources.
- A resource must explicitly call `RegisterNUICallback(type, cb)` or `AddEventHandler("__cfx_nui:type", cb)` to receive NUI callbacks of that type.
- NUI callbacks are isolated to the NUI's owning resource by default, because the `__cfx_nui:` event is only triggered towards that resource's event handlers.

## Resource-level NUI state

`ResourceUI` (defined in `ResourceUI.cpp`) manages per-resource NUI state:

- **Callback map**: Stores registered NUI callback types and their associated function references or event paths.
- **Resource frame**: Associates a CEF browser frame with the owning resource.
- **Push callback list**: Maintains registered push event callbacks for forwarding browser events to the resource.

## Lua scripting API

### RegisterNUICallback (legacy path)

```lua
RegisterNUICallback(type, callback)
```

Registers a legacy-style NUI callback handler.

- `type` (string) — the callback type name (maps to `__cfx_nui:{type}`)
- `callback` (function) — handler function with signature `function(data, cb)`

**Internal sequence**:

1. Calls `RegisterNuiCallbackType(type)` to register the C++-side callback type.
2. Calls `AddEventHandler("__cfx_nui:" .. type, function(data, cb) ... end)`.

The handler receives two arguments:
- `data` — the parsed JSON body from the NUI request, converted to a Lua table
- `cb` — a callable proxy table backed by the result callback reference (`_cfx_internal:0:{refId}`)

Calling `cb(result)` sends the result back to the browser as JSON.

### RegisterNuiCallback (ref-based path)

```lua
RegisterNuiCallback(type, callback)
```

Registers a ref-based NUI callback handler (newer API).

- `type` (string) — the callback type name
- `callback` (function) — handler function with signature `function(data, cb)`

**Internal sequence**:

1. Calls `RegisterNuiCallbackType(type)` to register the C++-side callback type.
2. The handler is wrapped and registered through a different path than the legacy event.

The behavior from the handler's perspective is identical to `RegisterNUICallback`. The difference is internal: ref-based callbacks use direct function reference invocation, while legacy callbacks queue a `__cfx_nui:` event.

### RegisterNuiCallbackType

```lua
RegisterNuiCallbackType(type)
```

Registers a callback type at the C++ level without attaching a Lua handler. This is called internally by both `RegisterNUICallback` and `RegisterNuiCallback`, but can also be called directly to register types that will be handled via explicit `AddEventHandler` calls.

### NUI message passing (game → browser)

```lua
SendNUIMessage(data)
```

Pushes a JSON-serializable Lua table to the NUI browser frame. Available as a native.

**Browser-side receiving**:

```javascript
window.addEventListener('message', (event) => {
    const data = event.data;
    // data is the JSON object sent from Lua
});
```

The message is delivered via the CEF `postMessage` mechanism to the NUI frame's JavaScript context.

### NUI focus control

```lua
SetNuiFocus(hasFocus, hasCursor)
```

Controls whether keyboard/mouse focus is given to the NUI overlay.

- `hasFocus` (boolean) — enables or disables NUI keyboard focus
- `hasCursor` (boolean) — enables or disables mouse cursor for NUI

When focus is active, game input is suppressed and redirected to the NUI browser.

## NUI push events (browser → game via CEF)

Defined in `NUICallbacks_PushEvent.cpp`, push events provide a separate mechanism for the browser to communicate with game resources outside the callback request/response pattern.

### Push event registration

Browser-side JavaScript can register push event handlers via:

```javascript
// Internal CEF binding
registerPushFunction(eventName, callback)
```

The `registerPushFunction` V8 binding stores the callback in the resource's push callback list.

### Push event dispatch

When the browser triggers a push event (via CEF process message `pushEvent`):

1. The event name and data are received as a CEF process message.
2. All registered push callbacks are invoked with the event data.
3. If the frame has a resource name set (via `setName`), the event is scoped to that resource.

### setName

Sets the resource name associated with a NUI frame. Called during NUI initialization to associate the CEF browser frame with the owning resource.

## HTTP request lifecycle

A NUI callback from browser to game follows this lifecycle:

1. **Browser sends request**: JavaScript in the NUI page makes a fetch/XHR POST to `https://cfx-nui-{resourceName}/{callbackType}` with a JSON body.
2. **CEF intercepts**: The custom CEF scheme handler intercepts the request. The URL is parsed to extract `resourceName` and `callbackType`.
3. **Callback lookup**: The resource's NUI callback map is queried for `callbackType`.
4. **JSON → msgpack**: The request body is parsed as JSON and converted to msgpack.
5. **Result callback creation**: `ResourceCallbackComponent::CreateCallback(...)` creates a callback reference that will serialize the handler's return value and send it as the HTTP response.
6. **Handler invocation**: Either:
   - (ref-based path) `CallReference<void>(callbackRef, postObject, resultReference)` is called directly.
   - (legacy path) `QueueEvent2("__cfx_nui:{type}", ..., postObject, resultReference)` queues the event.
7. **Handler executes**: The Lua/JS/C# handler processes the data and calls `cb(result)`.
8. **Result serialization**: The result callback reference converts the return value: msgpack → JSON → HTTP response body.
9. **Browser receives response**: The browser's fetch/XHR promise resolves with the JSON response.

## Response types

When the handler calls `cb(result)`, the result is serialized back to the browser:

- **Tables/objects**: Serialized as JSON objects
- **Arrays**: Serialized as JSON arrays
- **Strings, numbers, booleans**: Passed through directly as JSON values
- **nil/null**: Serialized as JSON `null`
- **Function references**: Not valid as NUI callback return values. Attempting to return a function reference through the callback channel produces undefined behavior.

## Guarantees

The implementation guarantees the following observable properties:

- The NUI callback system is client-only infrastructure. No NUI APIs exist on the server.
- `__cfx_nui:` events bypass the wildcard (`*`) global handler routing in `ResourceEventManagerComponent`.
- Legacy NUI callbacks are delivered via `QueueEvent2` (deferred to next Tick).
- Ref-based NUI callbacks are invoked synchronously via `CallReference`.
- Result callback references use the `_cfx_internal:0:{refId}` canonical form.
- `SendNUIMessage` delivers JSON data to the NUI browser frame via CEF postMessage.
- NUI callbacks are scoped to the resource that owns the NUI frame.
- `SetNuiFocus` blocks game input when `hasFocus` is true.

## Non-guarantees

The implementation does **not** guarantee:

- That NUI callback results arrive at the browser within any specific time bound (especially for legacy path, which is queued).
- That multiple concurrent callbacks of the same type are processed in order.
- That the browser receives a response if the handler never calls `cb(result)`.
- That function reference return values through NUI callbacks produce meaningful results.
- That `RegisterNUICallback` and `RegisterNuiCallback` behave identically at the timing level (legacy path uses queued events; ref-based path uses direct invocation).
- That push events are delivered if no push function is registered for the event name.

## Relationship to other specifications

- `specs/event-system-reference.md` describes the event dispatch system that processes `__cfx_nui:` events.
- `specs/event-api-reference.md` describes the scripting APIs used for event handling.
- `specs/lua-function-reference-reference.md` describes the callable proxy tables used for NUI result callbacks.
- `specs/game-events-reference.md` catalogs all default game events, including `__cfx_nui:` events.
