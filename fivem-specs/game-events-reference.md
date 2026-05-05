# FiveM game events reference

## Scope

This document catalogs all default events triggered by the FiveM host. It covers:

- **NETEV-documented events** — events with TypeScript type annotations extracted from C++ source by `ext/event-doc-gen/index.js`
- **Internal system events** — events triggered by the host that are not documented via NETEV markers
- **Chat system events** — events defined by the bundled `chat` system resource
- **Server game state events** — game-synchronization events triggered by `ServerGameState`

Source files:

- `code/components/citizen-server-impl/src/GameServer.cpp`
- `code/components/citizen-server-impl/src/ClientRegistry.cpp`
- `code/components/citizen-server-impl/src/ServerResources.cpp`
- `code/components/citizen-server-impl/src/ServerGameState.cpp`
- `code/components/citizen-server-impl/src/ServerEventComponent*.cpp`
- `code/components/citizen-resources-core/src/ResourceEventComponent.cpp`
- `code/components/gta-net-five/src/` (various NETEV markers)
- `ext/event-doc-gen/index.js`
- `ext/system-resources/chat/`

It is a Reference document. It describes each event's name, subset, payload type, and trigger conditions.

## NETEV documentation convention

Events documented with `/*NETEV` markers in C++ source are the canonical reference for event type signatures. The marker format is:

```
/*NETEV eventName SUBSET
 *
 * // TypeScript type definition of payload
 * { field: type, ... }
 *
 */
TriggerEvent2("eventName", ...)
```

Where:
- `eventName` — the event name string
- `SUBSET` — `CLIENT`, `SERVER`, or `SHARED`

The `ext/event-doc-gen/index.js` tool extracts these markers and generates Typedoc-formatted documentation. Events without NETEV markers are not included in the generated documentation, even if they are functional at runtime.

### Subset semantics

The `SUBSET` field in a NETEV marker is **not just a documentation convention**. It reflects where the C++ trigger code physically runs:

| Subset | C++ trigger location | Fires on | Can be received by |
|---|---|---|---|
| `CLIENT` | Client-side C++ code (game components, `gta-net-five`, client-impl) | Client process only | Client resource scripts only |
| `SERVER` | Server-side C++ code (GameServer, ServerGameState, ServerResources) | Server process only | Server resource scripts only |
| `SHARED` | Both (via `ResourceEventComponent` which runs on both sides) | Both client and server processes | Both client and server resource scripts |

**The subset is enforced by process architecture, not by runtime validation.** The runtime's `TriggerEvent` dispatches to handlers in the same process only. A `SERVER`-subset event has its C++ trigger code in the server process, so it physically cannot fire on the client (and vice versa). The runtime does not need to check the subset — the process boundary guarantees it.

What is **not** enforced: if you manually call `TriggerEvent("playerConnecting")` from a client resource script, the runtime will dispatch it to client-side handlers. The subset marking does not prevent this. But the host never triggers `playerConnecting` on the client side, so a client handler registered for it would never receive the genuine host-triggered event.

### Events that fire on both sides (SHARED subset)

`SHARED` events have trigger code in `ResourceEventComponent`, which runs in both the server and client processes. When a resource starts on the server, the server's `ResourceEventComponent` fires `onResourceStart` to server-side handlers. When a resource starts on the client, the client's `ResourceEventComponent` fires `onResourceStart` to client-side handlers. These are two separate, independent dispatches — one does not cross the network boundary.

## Server events

### playerConnecting

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | Yes |
| **Source** | `GameServer.cpp` (NETEV) |

Triggered when a player begins connecting to the server. Canceling this event prevents the player from joining.

**Payload**:

```typescript
{
  playerName: string;
  setKickReason: (reason: string) => void;
  deferrals: {
    defer(): void;
    update(message: string): void;
    presentCard(data: string | object, callback?: (data: any, rawData: string) => void): void;
    done(reason?: string): void;
    handover(data: object): void;
  };
  source: string;
}
```

**Lifecycle**: This is the first event fired for a connecting player. The `deferrals` object allows resources to delay connection acceptance for tasks such as identity verification, character selection, or queue management. Calling `deferrals.done()` without a reason allows the connection to proceed. Calling `deferrals.done("reason")` rejects the connection.

**Cancelation**: Calling `CancelEvent()` has the same effect as calling `deferrals.done()` with a rejection.

### playerJoining

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | `GameServer.cpp` (NETEV) |

Triggered after a player has passed through `playerConnecting` and is about to fully join the server.

**Payload**:

```typescript
{
  source: string;
  oldID: string;
}
```

`oldID` is typically empty for new connections. It may contain a value when a player reconnects and the old player object is being replaced.

### playerDropped

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | `GameServer.cpp:1142` (NOT a NETEV, triggered via `TriggerEvent2`) |

Triggered when a player disconnects from the server.

**Payload** (via `TriggerEvent2`):

- `reason` (string) — human-readable disconnect reason, e.g. `"Disconnected."`, `"Exiting."`, `"Timed out."`, `"Reloading."`
- `resourceName` (string) — deprecated, typically empty
- `dropReason` (uint32) — numeric drop reason code (netEvent-based)

**Important**: This event is NOT documented via a NETEV marker. Its payload is passed through `TriggerEvent2` with three separate arguments rather than a single data object. Language servers must not expect this event to match the NETEV-type pattern (single object payload).

### playerEnteredScope

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when a player enters another player's network scope.

**Payload**:

```typescript
{
  player: string;
  for: string;
}
```

- `player` — the player ID (source) that entered scope
- `for` — the player ID whose scope was entered

### playerLeftScope

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when a player leaves another player's network scope.

**Payload**:

```typescript
{
  player: string;
  for: string;
}
```

### entityCreating

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | Yes |
| **Source** | NETEV |

Triggered when an entity is about to be created on the server.

**Payload**:

```typescript
{
  handle: number;
}
```

Canceling this event prevents the entity from being created.

### entityCreated

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered after an entity has been created on the server.

**Payload**:

```typescript
{
  handle: number;
}
```

### entityRemoved

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when an entity is removed.

**Payload**:

```typescript
{
  entity: number;
}
```

### serverEntityCreated

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when a server-managed (non-network-synced) entity is created.

**Payload**:

```typescript
{
  handle: number;
}
```

### onPlayerBucketChange

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when a player's routing bucket changes.

**Payload**:

```typescript
{
  player: string;
  bucket: number;
  oldBucket: number;
}
```

### onEntityBucketChange

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when an entity's routing bucket changes.

**Payload**:

```typescript
{
  entity: string;
  bucket: number;
  oldBucket: number;
}
```

### weaponDamageEvent

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when weapon damage occurs. This event is fired for every instance of weapon-based damage in the game world.

**Payload**:

```typescript
{
  sender: number;
  data: {
    damageType: number;
    weaponType: number;          // hash
    overrideDefaultDamage: boolean;
    hitEntityWeapon: boolean;
    hitWeaponAmmoAttachment: boolean;
    silenced: boolean;
    damageFlags: number;
    hasActionResult: boolean;
    actionResultName: number;    // hash
    actionResultId: number;
    f104: number;
    weaponDamage: number;
    isNetTargetPos: boolean;
    localPosX: number;
    localPosY: number;
    localPosZ: number;
    f112: boolean;
    damageTime: number;
    willKill: boolean;
    f120: boolean;
    hasVehicleData: boolean;
    // ... additional fields vary by game build
  };
}
```

### removeAllWeaponsEvent

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when all weapons are removed from a ped.

**Payload**:

```typescript
{
  sender: number;
  data: {
    pedId: number;
  };
}
```

### startProjectileEvent

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when a projectile is fired.

**Payload**:

```typescript
{
  sender: number;
  data: {
    ownerId: number;
    projectileHash: number;
    weaponHash: number;
    initialPositionX: number;
    initialPositionY: number;
    initialPositionZ: number;
    targetEntity: number;
    firePositionX: number;
    firePositionY: number;
    firePositionZ: number;
    effectGroup: number;
    // ... additional fields
  };
}
```

### ptFxEvent

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when a particle effects (PTFX) event occurs.

**Payload**:

```typescript
{
  sender: number;
  data: {
    effectHash: number;
    assetHash: number;
    posX: number;
    posY: number;
    posZ: number;
    offsetX: number;
    offsetY: number;
    offsetZ: number;
    rotX: number;
    rotY: number;
    rotZ: number;
    scale: number;
    axisBitset: number;
    isOnEntity: boolean;
    entityNetId: number;
    // ... additional fields
  };
}
```

### givePedScriptedTaskEvent

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when a scripted task is given to a ped.

**Payload**:

```typescript
{
  sender: number;
  data: {
    entityNetId: number;
    taskId: number;
  };
}
```

### onResourceListRefresh

| Property | Value |
|---|---|
| **Subset** | `SERVER` |
| **Cancelable** | No |
| **Source** | `ServerResources.cpp:245-254` (NETEV) |

Triggered when the resource list is refreshed (resources started or stopped).

**Payload**: `void` — no payload.

## Client events

### populationPedCreating

| Property | Value |
|---|---|
| **Subset** | `CLIENT` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when a population (ambient) ped is being created.

**Payload**:

```typescript
{
  x: number;
  y: number;
  z: number;
  model: number;
  overrideCalls: {
    setPosition(x: number, y: number, z: number): void;
    setModel(model: number): void;
  };
}
```

### gameEventTriggered

| Property | Value |
|---|---|
| **Subset** | `CLIENT` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered for every game event (CEvent network events). The event name is dynamic — it is prefixed with `CEvent` followed by the game event class name.

**Payload**:

```typescript
{
  name: string;
  data: number[];
}
```

The full set of `CEvent*` names is determined by the game build. Common examples include:
- `CEventNetworkPlayerCollectedAmbientPickup`
- `CEventNetworkPlayerCollectedPickup`
- `CEventNetworkPlayerCollectedPortablePickup`
- `CEventNetworkEntityDamage`
- `CEventNetworkVehicleUndrivable`
- `CEventNetworkPlayerEnteredVehicle`
- `CEventNetworkPlayerLeftVehicle`
- ... (hundreds of game event types)

### entityDamaged

| Property | Value |
|---|---|
| **Subset** | `CLIENT` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when an entity takes damage on the client.

**Payload**:

```typescript
{
  victim: number;
  culprit: number;
  weapon: number;       // weapon hash
  baseDamage: number;
}
```

### mumbleConnected

| Property | Value |
|---|---|
| **Subset** | `CLIENT` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when the Mumble VoIP client connects to a server.

**Payload**:

```typescript
{
  address: string;
  reconnecting: boolean;
}
```

### mumbleDisconnected

| Property | Value |
|---|---|
| **Subset** | `CLIENT` |
| **Cancelable** | No |
| **Source** | NETEV |

Triggered when the Mumble VoIP client disconnects.

**Payload**:

```typescript
{
  address: string;
}
```

## Shared events (both client and server)

### onResourceStarting

| Property | Value |
|---|---|
| **Subset** | `SHARED` |
| **Cancelable** | Yes |
| **Source** | `ResourceEventComponent.cpp` (NETEV) |

Triggered before a resource starts. Canceling this event prevents the resource from starting.

**Payload**:

```typescript
{
  resource: string;
}
```

### onResourceStart

| Property | Value |
|---|---|
| **Subset** | `SHARED` |
| **Cancelable** | No |
| **Source** | `ResourceEventComponent.cpp` (NETEV) |

Triggered immediately after a resource starts. This event is triggered synchronously during resource initialization.

**Payload**:

```typescript
{
  resource: string;
}
```

### onClientResourceStart

| Property | Value |
|---|---|
| **Subset** | `SHARED` |
| **Cancelable** | No |
| **Source** | `ResourceEventComponent.cpp` (NETEV) |

Triggered when a resource starts, delivered via the queued event system. This event fires on both client and server, regardless of the name.

**Payload**:

```typescript
{
  resource: string;
}
```

### onServerResourceStart

| Property | Value |
|---|---|
| **Subset** | `SHARED` |
| **Cancelable** | No |
| **Source** | `ResourceEventComponent.cpp` (NETEV) |

Triggered when a resource starts, delivered via the queued event system. Fires on both client and server.

**Payload**:

```typescript
{
  resource: string;
}
```

**Note**: Both `onClientResourceStart` and `onServerResourceStart` fire for every resource on both the client and the server. The distinction is in the event name, not in whether the event fires on a particular side.

### onResourceStop

| Property | Value |
|---|---|
| **Subset** | `SHARED` |
| **Cancelable** | No |
| **Source** | `ResourceEventComponent.cpp` (NETEV) |

Triggered immediately when a resource stops.

**Payload**:

```typescript
{
  resource: string;
}
```

### onClientResourceStop

| Property | Value |
|---|---|
| **Subset** | `SHARED` |
| **Cancelable** | No |
| **Source** | `ResourceEventComponent.cpp` (NETEV) |

Triggered when a resource stops, delivered via the queued event system.

**Payload**:

```typescript
{
  resource: string;
}
```

### onServerResourceStop

| Property | Value |
|---|---|
| **Subset** | `SHARED` |
| **Cancelable** | No |
| **Source** | `ResourceEventComponent.cpp` (NETEV) |

Triggered when a resource stops, delivered via the queued event system.

**Payload**:

```typescript
{
  resource: string;
}
```

## Internal system events

These events are triggered by the host but are NOT documented via NETEV markers. They use the same dispatch mechanism as NETEV events.

### onPlayerJoining (internal, client-only)

| Property | Value |
|---|---|
| **Triggered by** | `ClientRegistry.cpp:219,226` |
| **Direction** | Sent to connected game clients, NOT to the resource event system |

This is NOT delivered through the normal event system. It is sent directly to game clients via the network protocol. It should not appear in resource event handlers.

### onPlayerDropped (internal, client-side)

| Property | Value |
|---|---|
| **Triggered by** | `ServerGameState.cpp:1546` |
| **Direction** | Client-side event for scope removal |

Triggered on the client when a remote player is removed from the local player's network scope.

### __cfx_internal:commandFallback

| Property | Value |
|---|---|
| **Triggered by** | Command system |
| **Direction** | Local |

Internal event used by the command system for unregistered command fallback. This event name cannot be made network-safe via `RegisterNetEvent`.

### __cfx_internal:httpResponse

| Property | Value |
|---|---|
| **Triggered by** | `PerformHttpRequest` internal handler |
| **Direction** | Local |

Delivers HTTP response data to resources that made HTTP requests.

### __cfx_internal:serverPrint

| Property | Value |
|---|---|
| **Triggered by** | Server print relay |
| **Direction** | Server → Client |

Relays server console print output to connected clients for display in the client console.

### rconCommand

| Property | Value |
|---|---|
| **Triggered by** | RCON system |
| **Direction** | Server |

Triggered when an RCON command is received.

### disconnecting

| Property | Value |
|---|---|
| **Triggered by** | Client disconnect handler |
| **Direction** | Client |

Triggered on the client when it is about to disconnect from the server.

### __cfx_nui:* events

See `specs/nui-callback-reference.md` for the full NUI callback system. Events prefixed with `__cfx_nui:` are used for NUI callback delivery and bypass wildcard routing in `ResourceEventManagerComponent`.

## Chat system events

These events are defined by the bundled `chat` system resource (`ext/system-resources/chat/`). Direction indicates which side triggers the event and where handlers receive it.

### chatMessage (legacy)

| Property | Value |
|---|---|
| **Source** | `sv_chat.lua` |
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Legacy chat event. The recommended replacement is `chat:addMessage`.

### chat:addMessage

| Property | Value |
|---|---|
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Triggered on the client to add a message to the chat display.

**Payload**: Message object with `color`, `multiline`, `args` fields.

### chat:addSuggestion

| Property | Value |
|---|---|
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Triggered on the client to add a command suggestion.

**Payload**: Suggestion object with `name`, `help`, `params` fields.

### chat:addSuggestions

| Property | Value |
|---|---|
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Triggered on the client to add multiple command suggestions at once.

**Payload**: Array of suggestion objects.

### chat:removeSuggestion

| Property | Value |
|---|---|
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Triggered on the client to remove a command suggestion.

**Payload**: `{ name: string }` — the suggestion name to remove.

### chat:addMode

| Property | Value |
|---|---|
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Triggered on the client to add a chat mode template.

**Payload**: A template string with `{}` placeholders for player name and message.

### chat:removeMode

| Property | Value |
|---|---|
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Triggered on the client to remove a chat mode template.

**Payload**: The template string that was previously added.

### chat:addTemplate

| Property | Value |
|---|---|
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Triggered on the client to add a chat message template.

**Payload**: Template configuration object.

### chat:clear

| Property | Value |
|---|---|
| **Direction** | Server → Client (via `TriggerClientEvent`) |

Triggered on the client to clear the chat display.

### chat:init

| Property | Value |
|---|---|
| **Direction** | Client → Server (via `TriggerServerEvent`) |

Triggered by the client to request chat initialization from the server. The server responds by sending chat configuration (suggestions, templates, modes) back to the client.

### _chat:messageEntered

| Property | Value |
|---|---|
| **Direction** | Client → Server (via `TriggerServerEvent`) |

Triggered on the server when a player enters a chat message.

**Payload**:

```typescript
{
  author: string;
  text: string;
}
```

## Server game state events

These events are triggered by `ServerGameState.cpp` for game-world synchronization. They are sent as network events, not through the resource event system.

### ExplosionEvent

Triggered when an explosion occurs. Synchronized to all relevant clients.

### giveWeaponEvent

Triggered when a weapon is given to a ped. Synchronized to relevant clients.

### removeWeaponEvent

Triggered when a weapon is removed from a ped. Synchronized to relevant clients.

### vehicleComponentControlEvent

Triggered for vehicle component interactions (doors, windows, etc.).

### clearPedTasksEvent

Triggered when a ped's tasks are cleared.

### respawnPlayerPedEvent

Triggered when a player ped is respawned.

### lightningEvent

Triggered during lightning strikes (weather effect).

### fireEvent

Triggered when fire is started or spreads.

### Network-synced scene events

- `requestNetworkSyncedSceneEvent` — requests a synced scene
- `startNetworkSyncedSceneEvent` — starts a synced scene
- `updateNetworkSyncedSceneEvent` — updates a synced scene
- `stopNetworkSyncedSceneEvent` — stops a synced scene

### Carriable/carry events

- `endLootEvent`
- `sendCarriableUpdateCarryStateEvent`
- `carriableVehicleStowStartEvent`
- `carriableVehicleStowCompleteEvent`
- `pickupCarriableEvent`
- `placeCarriableOntoParentEvent`

**Note**: These game state events are transmitted over the game network protocol, not through `TriggerClientEvent`. Resources do not receive them as script event handlers. They exist for reference when understanding server-to-client game state synchronization.

## Cancelable events

The following events support cancellation via `CancelEvent()`:

| Event | Subset |
|---|---|
| `playerConnecting` | SERVER |
| `entityCreating` | SERVER |
| `onResourceStarting` | SHARED |

When a cancelable event is canceled, subsequent handlers for that event are not invoked. The host may take action based on the cancellation (e.g., rejecting a player connection, preventing entity creation, aborting resource start).

## Event dispatch order for resource lifecycle

When a resource starts, the following events fire in this order:

1. `onResourceStarting` (cancelable, synchronous) — can prevent the resource from starting
2. `onClientResourceStart` (queued, fires on next Tick) — fires on both client and server
3. `onServerResourceStart` (queued, fires on next Tick) — fires on both client and server
4. `onResourceStart` (synchronous, after `OnStart` callback)

When a resource stops:

1. `onClientResourceStop` (queued)
2. `onServerResourceStop` (queued)
3. `onResourceStop` (synchronous)

The queued events (2 and 3 in start; 1 and 2 in stop) are delivered during the next Tick cycle, not synchronously during resource initialization or shutdown.

## Guarantees

The implementation guarantees the following observable properties:

- NETEV-documented events have stable TypeScript type signatures as documented in C++ source.
- All NETEV events are triggered via `TriggerEvent2` or `TriggerEvent` from C++ host code.
- Event payloads are msgpack-serialized for delivery to scripting runtimes.
- Cancelable events can be canceled by any handler, stopping further dispatch.
- Resource lifecycle events follow the documented order (Starting → ClientStart → ServerStart → Start; ClientStop → ServerStop → Stop).
- `__cfx_nui:` events bypass wildcard routing.
- `__cfx_internal:commandFallback` cannot be registered as network-safe.

## Non-guarantees

The implementation does **not** guarantee:

- That the runtime prevents manually triggering an event on the side where the host would not trigger it (e.g., calling `TriggerEvent("playerConnecting")` on the client dispatches normally — but the real host-triggered event only fires on the server).
- That all events are documented via NETEV markers. Internal events such as `playerDropped` use the dispatch system but lack NETEV markers.
- That `CEvent*` event names are stable across game builds. The set of game events varies by GTA V version.
- That `weaponDamageEvent`, `startProjectileEvent`, `ptFxEvent`, and similar game-sync events have complete type definitions. Game updates may add or remove fields.
- That game state events (ExplosionEvent, giveWeaponEvent, etc.) are delivered as resource script events. They are network-level game synchronization events.

## Relationship to other specifications

- `specs/event-system-reference.md` describes the dispatch architecture that delivers these events.
- `specs/event-api-reference.md` describes the scripting APIs used to listen for and trigger events.
- `specs/nui-callback-reference.md` describes `__cfx_nui:` events and the NUI callback system.
- `specs/lua-resource-management-reference.md` describes the resource lifecycle that triggers `onResourceStarting`, `onResourceStart`, and their variants.
