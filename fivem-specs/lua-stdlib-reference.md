# FiveM Lua standard-library reference

## Scope

This document catalogues every global function, table, and value exposed by FiveM to Lua script execution environments.

It covers the curated Lua 5.4 standard libraries opened by the C++ runtime, the C serialisation extensions, the `Citizen` host-binding table, the scheduler-layer globals installed by `scheduler.lua`, the promise library from `deferred.lua`, the server-only custom `io` and `os` library replacements, and the lazy-loaded native-function injection mechanism.

It is a Reference document. It describes what exists. It does not describe how to use these facilities in workflows.

Primary implementation files:

- `code/components/citizen-scripting-lua/src/LuaScriptRuntime.cpp`
- `code/components/citizen-scripting-lua/src/LuaDebug.cpp`
- `code/components/citizen-scripting-lua/src/LuaOS.cpp`
- `code/components/citizen-scripting-lua/src/LuaIO.cpp`
- `code/components/citizen-scripting-lua/src/LuaScriptNatives.cpp`
- `code/components/citizen-scripting-lua/src/LuaNativesLoader.cpp`
- `code/components/citizen-scripting-core/src/ResourceScriptFunctions.cpp`
- `code/components/citizen-scripting-core/src/EventScriptFunctions.cpp`
- `code/components/citizen-scripting-core/src/RefScriptFunctions.cpp`
- `data/shared/citizen/scripting/lua/scheduler.lua`
- `data/shared/citizen/scripting/lua/deferred.lua`
- `data/shared/citizen/scripting/lua/graph.lua`
- `data/shared/citizen/scripting/lua/json.lua`
- `data/shared/citizen/scripting/lua/MessagePack.lua`
- `data/shared/citizen/scripting/lua/natives_loader.lua`
- `code/tests/server/TestLua.cpp`

## Standard Lua 5.4 libraries

The runtime opens a curated subset of stock Lua 5.4 libraries. The selection is defined by the `lualibs[]` array in `LuaScriptRuntime.cpp`.

`dofile` and `loadfile` are removed from `_G` immediately after the base library is opened. See "Removed and replaced globals" below for the full list.

### `_G` (base library)

Available members:

| Member | Notes |
|--------|-------|
| `ipairs(t)` | |
| `pairs(t)` | |
| `tostring(v)` | |
| `type(v)` | |
| `pcall(f, ...)` | |
| `xpcall(f, msgh, ...)` | |
| `error(msg [, level])` | |
| `assert(v [, msg])` | |
| `select(which, ...)` | |
| `next(table [, index])` | |
| `rawget(table, key)` | |
| `rawset(table, key, value)` | |
| `rawlen(v)` | |
| `setmetatable(table, metatable)` | |
| `getmetatable(object)` | |
| `load(chunk [, chunkname [, mode [, env]]])` | Accepts string or function chunks |
| `tonumber(v [, base])` | |
| `tostring(v)` | |
| `print(...)` | Replaced by custom implementation (see "Removed and replaced globals") |
| `require(modname)` | Replaced by custom implementation (see "Removed and replaced globals") |
| `warn(msg1, ...)` | Replaces stock `warn` — routed through `lua_setwarnf` to `Lua_Warn` |
| `_VERSION` | `"Lua 5.4"` |
| `_G` | Self-reference to global table |

**Removed from `_G`:**

| Removed global | Reason |
|----------------|--------|
| `dofile` | Security — explicit removal after base library open |
| `loadfile` | Security — explicit removal after base library open |

Stock `_G` members `collectgarbage`, `rawequal`, and `newproxy` are not documented here; these may also be absent or inaccessible depending on the Lua build configuration. The documented list above is the confirmed usable surface.

### `table`

Full stock implementation. Available functions:

`table.concat`, `table.insert`, `table.pack`, `table.unpack`, `table.remove`, `table.sort`, `table.move`, `table.clear`

### `string`

Full stock implementation. Available functions:

`string.byte`, `string.char`, `string.find`, `string.format`, `string.gmatch`, `string.gsub`, `string.len`, `string.lower`, `string.match`, `string.rep`, `string.reverse`, `string.sub`, `string.upper`, `string.dump`

### `math`

Full stock implementation. Available constants and functions:

Constants: `math.pi`, `math.huge`, `math.maxinteger`, `math.mininteger`

Functions: `math.abs`, `math.acos`, `math.asin`, `math.atan`, `math.ceil`, `math.cos`, `math.deg`, `math.exp`, `math.floor`, `math.fmod`, `math.log`, `math.max`, `math.min`, `math.modf`, `math.pow`, `math.rad`, `math.random`, `math.randomseed`, `math.sin`, `math.sqrt`, `math.tan`, `math.tointeger`, `math.type`, `math.ult`

### `coroutine`

Full stock implementation. Available functions:

`coroutine.create`, `coroutine.resume`, `coroutine.running`, `coroutine.status`, `coroutine.wrap`, `coroutine.yield`, `coroutine.isyieldable`

### `utf8`

Full stock implementation. Available functions:

`utf8.char`, `utf8.charpattern`, `utf8.codepoint`, `utf8.codes`, `utf8.len`, `utf8.offset`

### `debug` (custom implementation)

The runtime does not open the stock Lua debug library. Instead it opens a custom limited implementation from `LuaDebug.cpp` (`lua_fx_opendebug`).

Available functions:

| Function | Behaviour |
|----------|-----------|
| `debug.getinfo([thread,] func [, what])` | Returns function information table |
| `debug.getmetatable(value)` | Returns the metatable of `value` |
| `debug.getupvalue(func, n)` | Returns the `n`-th upvalue name and value of `func` |
| `debug.setmetatable(value, table)` | Sets metatable on `value` |
| `debug.traceback([thread,] [msg [, level]])` | Returns stack trace string |

**Not available** (present in stock Lua 5.4 debug library but absent in the FiveM implementation):

`debug.debug`, `debug.gethook`, `debug.getlocal`, `debug.getregistry`, `debug.getupvalue`, `debug.setlocal`, `debug.setupvalue`, `debug.sethook`, `debug.upvalueid`, `debug.upvaluejoin`, `debug.setcstacklimit`

## Serialisation C libraries

### `msgpack`

C library (`cmsgpack`) bound as a global `msgpack` table.

| Member | Signature | Behaviour |
|--------|-----------|-----------|
| `msgpack.pack(data)` | `any → string` | Serialise a Lua value to a MessagePack binary string. Function values are serialised as extension references through the function-reference transport layer. |
| `msgpack.unpack(str)` | `string → any` | Deserialise a MessagePack binary string. Function-reference extensions are materialised as callable proxy tables. |
| `msgpack.pack_args(...)` | `... → string` | Serialise multiple arguments as a packed array. |
| `msgpack.set_string(mode)` | `string → nil` | Set string encoding mode (`"unsigned"`, `"binary"`). |
| `msgpack.set_integer(mode)` | `string → nil` | Set integer encoding mode (`"signed"`, `"unsigned"`). |
| `msgpack.set_array(mode)` | `string → nil` | Set array encoding mode. |
| `msgpack.set_number(mode)` | `string → nil` | Set number encoding mode. |
| `msgpack.settype(typeName, extId)` | `string, integer → nil` | Register a Lua type name to a MessagePack extension type ID for custom serialisation. Built-in: `"function"` mapped to `EXT_FUNCREF`. |
| `msgpack.extend(tbl)` | `table → nil` | Register extension handler table with `pack`/`unpack` functions. |
| `msgpack.extend_clear(from, to)` | `integer, integer → nil` | Clear extension handlers in a range of type IDs. |
| `msgpack.build_ext(tag, data)` | `integer, string → string` | Build a raw MessagePack extension binary payload. |
| `msgpack.sentinel` | sentinel value | Value used as nil-marker in packed tables. |
| `msgpack._VERSION` | `string` | Version string. |

See `specs/lua-function-reference-reference.md` for the complete description of function-reference serialisation and materialisation behaviour.

### `json`

C library (`rapidjson` through a Lua binding) registered as a global `json` table, with a pure-Lua fallback (`dkjson 2.5`).

| Member | Signature | Behaviour |
|--------|-----------|-----------|
| `json.encode(value [, state])` | `any [, table] → string` | Encode a Lua value to a JSON string. Accepts optional state table for pretty-printing configuration. |
| `json.decode(str [, pos [, nullval [, objectmeta [, arraymeta]]]])` | `string [, integer [, any [, table [, table]]]] → any` | Decode a JSON string to a Lua value. |
| `json.null` | sentinel value | Value used to represent JSON `null`. |
| `json.quotestring(str)` | `string → string` | Quote a Lua string as a JSON string literal. |
| `json.addnewline(state)` | `table → nil` | Append a newline to the pretty-printing state buffer. |
| `json.use_lpeg()` | `→ boolean` | Attempt to switch the decoder to LPeg-based parsing. Returns `true` if LPeg is available. |
| `json.version` | `string` | Library version string. |
| `json.using_lpeg` | `boolean` | `true` when the LPeg-based parser is active. |
| `json.setoption(optname, value)` | `string, any → nil` | Set JSON library options (C binding). |

## `Citizen` host-binding table

The runtime creates a global table named `Citizen` from the `g_citizenLib` registry in `LuaScriptRuntime.cpp`.

### Scheduler and lifecycle members

| Member | Signature | Behaviour |
|--------|-----------|-----------|
| `Citizen.SetBoundaryRoutine(fn)` | `function → nil` | Install the host boundary-routine callback used for stack-trace delimiting. |
| `Citizen.SetTickRoutine(fn)` | `function → 0` | Install the tick routine. Currently a no-op that always returns `0`. |
| `Citizen.SetEventRoutine(fn)` | `function → nil` | Install the host event-delivery callback. |
| `Citizen.CreateThread(fn [, name])` | `function [, string] → coroutine` | Create a new Lua coroutine wrapped in a scheduler bookmark. |
| `Citizen.CreateThreadNow(fn [, name])` | `function [, string] → nil` | Create and immediately resume a coroutine. |
| `Citizen.Wait(ms)` | `number → nil` | Yield the current coroutine for at least `ms` milliseconds. Must be called inside a scheduler coroutine. |
| `Citizen.SetTimeout(fn, ms)` | `function, number → integer` | Schedule `fn` to run after `ms` milliseconds. Returns a timeout ID that can be passed to `ClearTimeout`. |
| `Citizen.ClearTimeout(ref)` | `integer → nil` | Cancel a pending timeout by its timeout ID. No-op if the ID does not exist. |
| `Citizen.Trace(msg)` | `string → nil` | Output a message to the script tracing/logging system. |

### Native-invocation members

| Member | Signature | Behaviour |
|--------|-----------|-----------|
| `Citizen.InvokeNative(hash, ...)` | `integer, ... → any` | Invoke a game native function by its hash. |
| `Citizen.LoadNative(name)` | `string → function or string or nil` | Resolve a native function by name. Returns a callable function, a Lua source string (compiled by `natives_loader.lua`), or `nil` if unknown. |

Client builds additionally expose:

| Member | Signature | Behaviour |
|--------|-----------|-----------|
| `Citizen.GetNative(name)` | `string → function or nil` | Resolve a native function directly (client only). |
| `Citizen.InvokeNative2(...)` | `... → any` | Alternative native-invocation path (client only). |

### Function-reference members

| Member | Signature | Behaviour |
|--------|-----------|-----------|
| `Citizen.SetCallRefRoutine(fn)` | `function → nil` | Install the host call-ref routine. |
| `Citizen.SetDeleteRefRoutine(fn)` | `function → nil` | Install the host delete-ref routine. |
| `Citizen.SetDuplicateRefRoutine(fn)` | `function → nil` | Install the host duplicate-ref routine. |
| `Citizen.CanonicalizeRef(idx)` | `integer → string` | Canonicalise a function-reference integer index to its canonical string form `resource:instanceId:refId`. |
| `Citizen.InvokeFunctionReference(ref, args)` | `string, string → any` | Invoke a canonical function reference with a msgpack-serialised argument payload. |
| `Citizen.GetFunctionReference(func)` | `function → string` | Create a canonical function reference for a Lua closure, returning its string form. |

### Boundary and stack-trace members

| Member | Signature | Behaviour |
|--------|-----------|-----------|
| `Citizen.SubmitBoundaryStart(id)` | `string → nil` | Submit a boundary start marker for stack-trace delimiting. |
| `Citizen.SubmitBoundaryEnd(id)` | `string → nil` | Submit a boundary end marker. |
| `Citizen.SetStackTraceRoutine(fn)` | `function → nil` | Install the stack-trace capture routine. |

### Pointer and result-helper members

These are used as native-invocation argument and result annotations. They are not general-purpose functions; they return special lightuserdata or sentinel values consumed by the native-marshalling layer.

| Member | Behaviour |
|--------|-----------|
| `Citizen.PointerValueInt()` | Output int32 pointer annotation |
| `Citizen.PointerValueFloat()` | Output float32 pointer annotation |
| `Citizen.PointerValueVector()` | Output vector pointer annotation |
| `Citizen.PointerValueIntInitialized(val)` | Output int32 pointer with initial value |
| `Citizen.PointerValueFloatInitialized(val)` | Output float32 pointer with initial value |
| `Citizen.ReturnResultAnyway()` | Native result: return result regardless of success |
| `Citizen.ResultAsInteger()` | Hint: expect int32 result |
| `Citizen.ResultAsLong()` | Hint: expect int64 result |
| `Citizen.ResultAsFloat()` | Hint: expect float32 result |
| `Citizen.ResultAsString()` | Hint: expect string result |
| `Citizen.ResultAsVector()` | Hint: expect vector result |
| `Citizen.ResultAsObject()` | Return object result via callback (no-op if no callback installed) |
| `Citizen.ResultAsObject2(unpackFn)` | Return object result via custom unpack function |
| `Citizen.AwaitSentinel()` | Special sentinel for `Citizen.Await` |

### Profiling member

| Member | Behaviour |
|--------|-----------|
| `Citizen.Graph(header, records)` | Construct a profiling graph from profiler data (from `graph.lua`). Returns an `LMGraph` object with methods `:Flat()`, `:Pepperfish()`, `:Callgrind()`, `:Verbose()`. |

## Scheduler globals

The runtime loads `citizen:/scripting/lua/scheduler.lua` during bootstrap. This script installs the top-level globals listed below. These globals are available in all Lua resources on both client and server, except where noted.

### Top-level convenience aliases

| Global | Maps to |
|--------|---------|
| `Wait(ms)` | `Citizen.Wait` |
| `CreateThread(fn)` | `Citizen.CreateThread` |
| `SetTimeout(ms, fn)` | `Citizen.SetTimeout` |
| `ClearTimeout(ref)` | `Citizen.ClearTimeout` |

### Event-system globals

| Global | Signature | Side |
|--------|-----------|------|
| `AddEventHandler(eventName, handler)` | `string, function → table` | Both |
| `RemoveEventHandler(token)` | `table → nil` | Both |
| `RegisterNetEvent(eventName [, callback])` | `string [, function] → nil` | Both |
| `TriggerEvent(eventName, ...)` | `string, ... → nil` | Both |
| `CancelEvent()` | `→ nil` | Both |
| `WasEventCanceled()` | `→ boolean` | Both |

`AddEventHandler` returns a token table shaped as `{ key = <integer>, name = <eventName> }`. This token must be passed to `RemoveEventHandler`.

`RegisterNetEvent` marks the event as `safeForNet`, which permits network-delivered events to invoke handlers registered for that name. The internal event `__cfx_internal:commandFallback` is explicitly excluded from safe-for-net marking.

The scheduler uses `msgpack.pack_args(...)` to serialise event arguments before delivery.

#### Client-only event globals

| Global | Signature | Behaviour |
|--------|-----------|-----------|
| `TriggerServerEvent(eventName, ...)` | `string, ... → nil` | Fire event on server. Serialises arguments with msgpack. |
| `TriggerLatentServerEvent(eventName, bps, ...)` | `string, number, ... → nil` | Fire event on server with rate-limited delivery in bits per second. |

#### Server-only event globals

| Global | Signature | Behaviour |
|--------|-----------|-----------|
| `TriggerClientEvent(eventName, playerId, ...)` | `string, integer, ... → nil` | Fire event on a specific client. |
| `TriggerLatentClientEvent(eventName, playerId, bps, ...)` | `string, integer, number, ... → nil` | Fire event on a specific client with rate-limited delivery. |
| `RegisterServerEvent(eventName [, callback])` | `string [, function] → nil` | Alias of `RegisterNetEvent`. |

### Export system

| Global | Behaviour |
|--------|-----------|
| `exports` | Root export table. Two calling conventions: `exports.resourceName.exportName(...)` for calling another resource's export, `exports("exportName", func)` for registering an export in the current resource. |

Export registration via `exports(name, func)` installs an event handler on the synthetic event name `__cfx_export_<resource>_<name>`.

Export resolution via `exports[resource][name]` triggers the synthetic export event lazily and caches the result in `exportsCallbackCache`.

See `specs/lua-function-reference-reference.md` for the complete export-callable lifecycle.

### NUI callback system (client only)

| Global | Signature | Behaviour |
|--------|-----------|-----------|
| `RegisterNuiCallback(type, callback)` | `string, function → nil` | Register a NUI callback directly through the NUI callback host binding. |
| `RegisterNUICallback(type, callback)` | `string, function → nil` | Register a legacy NUI callback: calls `RegisterNuiCallbackType(type)` and registers event handler for `__cfx_nui:<type>`. |
| `SendNUIMessage(message)` | `table → nil` | Encode message as JSON and push to the NUI layer. |

### State-bag globals

| Global | Type | Behaviour |
|--------|------|-----------|
| `GlobalState` | proxy table | Global replicated state bag. Reads call `GetStateBagValue('global', key)`. Assignment calls `SetStateBagValue('global', key, msgpack_value)`. |
| `LocalPlayer` | proxy value | Client only. Table with `.state` bag (equivalent to `Player(-1).state`). |

`GlobalState` is created eagerly as `NewStateBag('global')`.

State-bag proxy tables support:
- direct field access: `GlobalState.key` → `GetStateBagValue('global', 'key')`
- direct field assignment: `GlobalState.key = value` → `SetStateBagValue('global', 'key', msgpack_encoded, false)`
- `.set(key, value, replicated)`: explicit replication flag

### Entity and Player wrappers

| Global | Signature | Behaviour |
|--------|-----------|-----------|
| `Entity(handle)` | `integer → table` | Wraps an entity handle in a table with a `.state` state-bag property. |
| `Player(handle)` | `integer → table` | Wraps a player handle in a table with a `.state` state-bag property. |

Both return value types that are serialisable through the msgpack extension mechanism.

### Server-specific utility globals

Available only when `IsDuplicityVersion()` returns `true`.

| Global | Signature | Behaviour |
|--------|-----------|-----------|
| `GetPlayers()` | `→ table` | Returns an array table of all connected player IDs as integers. |
| `GetPlayerIdentifiers(playerId)` | `integer → table` | Returns an array of identifier strings for the given player (e.g. `"steam:12345"`, `"license:abc"`). |
| `GetPlayerTokens(playerId)` | `integer → table` | Returns an array of token strings for the given player. |
| `PerformHttpRequest(url, cb [, method [, data [, headers [, options]]]])` | `string, function [, string [, string [, table [, table]]]] → nil` | Perform an asynchronous HTTP request. Calls `PerformHttpRequestInternalEx`. Completion dispatched through internal event `__cfx_internal:httpResponse`. |
| `PerformHttpRequestAwait(url [, method [, data [, headers [, options]]]])` | `string [, string [, string [, table [, table]]]] → promise` | Perform an HTTP request that returns a promise. Uses `Citizen.Await` on completion. |
| `RconPrint(...)` | `... → nil` | Alias of `Citizen.Trace`. |
| `RconLog(...)` | `... → nil` | No-op. |
| `GetPlayerEP(playerId)` | `integer → string` | Returns the endpoint address of a player. |

## Promise library (`deferred.lua`)

The runtime loads `citizen:/scripting/lua/deferred.lua` during bootstrap. It registers a global `promise` table.

### `promise` table

| Member | Behaviour |
|--------|-----------|
| `promise.new([fn])` | Create a new deferred (promise). Returns a deferred table. If `fn` is supplied, it is called immediately with the deferred as argument. |
| `promise.all({...})` | Create a promise that resolves when all promises in the array resolve. Non-promise values are treated as already-resolved. |

### Deferred object

Created by `promise.new()`. Each deferred has the following methods:

| Method | Behaviour |
|--------|-----------|
| `deferred:resolve(value)` | Resolve the promise with `value`. Subsequent calls are ignored. |
| `deferred:reject(value)` | Reject the promise with `value`. Subsequent calls are ignored. |
| `deferred:next(successFn [, failureFn])` | Register success and failure callbacks. Returns a new promise that resolves with the return value of the callback. |

### `Citizen.Await`

`Citizen.Await(promise)` suspends the current scheduler coroutine until the promise resolves or rejects. Must be called from inside a scheduler coroutine (i.e. not from the top level of a script file). When called outside a scheduler coroutine it asserts with an error instructing the caller to use `CreateThread`, `SetTimeout`, or an event handler.

## Server-only custom `io` library

On FXServer builds, the runtime opens a custom VFS-backed `io` library from `LuaIO.cpp`. This replaces the stock Lua `io` library entirely.

### Top-level `io` functions

| Function | Behaviour |
|----------|-----------|
| `io.open(path, mode)` | Open a file through FiveM VFS. Rejects path segments containing `..` with `ENOENT`. Supports `@`-prefixed device paths. Write modes are gated by `ScriptingFilesystemAllowWrite`. Returns file handle or `(nil, strerror, ENOENT)`. |
| `io.popen(cmd, mode)` | Does not spawn a process. Only recognises `dir "..."` and `ls "..."` commands with a single quoted path operand. Returns directory userdata or fails. |
| `io.readdir(path)` | Enumerate directory entries through VFS. Skips `.` and `..`. Returns directory userdata. |
| `io.type(obj)` | Returns `"file"` for file handles, `"directory"` for directory handles, or `nil`. |
| `io.write(...)` | Writes arguments to script trace output (same as `Citizen.Trace`/`Lua_Print`). Not a file write. |
| `io.flush()` | No-op. |
| `io.lines()` | Empty iterator (returns nothing). Not a file-backed line iterator. |
| `io.tmpfile()` | Always fails. |
| `io.close(file)` | Close an open file handle. |
| `io.stdin` | Emulated empty file handle. |
| `io.stdout` | Emulated closed file handle. |
| `io.stderr` | Emulated closed file handle. |

### File handle methods

Returned by `io.open`. Each handle supports:

| Method | Behaviour |
|--------|-----------|
| `handle:read(...)` | Read modes: default (line without newline), numeric byte count, `"*n"` (number), `"*l"` (line without newline), `"*L"` (line with newline), `"*a"` (all). |
| `handle:write(...)` | Write numeric (converted via string format) or string data. Truncates file to current position after write if shorter than original stream. |
| `handle:seek([whence [, offset]])` | Seek in file. `whence` is `"set"`, `"cur"`, or `"end"`. |
| `handle:flush()` | Flush buffered writes. |
| `handle:close()` | Close the file handle. |
| `handle:lines(...)` | Line iterator with 64 KiB intermediate buffer. |
| `handle:setvbuf(...)` | No-op (succeeds but does nothing). |

### Directory userdata methods

Returned by `io.popen` and `io.readdir`. Each directory userdata supports:

| Method | Behaviour |
|--------|-----------|
| `dir:close()` | Close the directory handle. |
| `dir:lines()` | Iterator over filenames in the directory. |
| `__gc` | Closes on garbage collection. |
| `__tostring` | Returns newline-joined filenames or `"directory (closed)"`. |

## Server-only custom `os` library

On FXServer builds, the runtime opens a custom `os` library from `LuaOS.cpp`. This replaces the stock Lua `os` library entirely.

### `os` functions

| Function | Behaviour |
|----------|-----------|
| `os.clock()` | Returns approximate CPU time in seconds. |
| `os.date([format [, time]])` | Format or parse date/time. Behaves like stock `os.date`. |
| `os.difftime(t2, t1)` | Returns difference `t2 - t1` in seconds. |
| `os.time([table])` | Returns the current or specified time as a Unix timestamp. |
| `os.execute(cmd)` | `os.execute(nil)` returns `true`. Any non-`nil` command returns `(nil, "Permission denied", EACCES)`. |
| `os.getenv(key)` | Lowercases the key. Only recognises the key `"os"`, returning `"Windows"` or `"Linux"`. All other keys return `nil`. |
| `os.setlocale(locale [, category])` | Validates the category argument. Returns the supplied locale string. Does not mutate the process locale. |
| `os.tmpname()` | Returns deterministic names in the form `tmp_<n>`. Does not expose an OS temporary path. |
| `os.createdir(path)` | Creates a directory through VFS. Returns `true` on success, or `(nil, message, code)` on failure. |
| `os.remove(path)` | Removes a file through VFS. Requires VFS write permission. Returns `(nil, "Permission denied", EACCES)` if denied. |
| `os.rename(from, to)` | Renames a file through VFS. Requires VFS write permission for both paths. |
| `os.exit([code [, closeAll]])` | Provided by stock Lua; behaviour depends on build configuration. |
| `os.deltatime(end, start)` | Returns the difference as an unsigned integer (high-resolution time). |
| `os.microtime()` | Returns the current time in microseconds since an unspecified epoch. |
| `os.nanotime()` | Returns the current time in nanoseconds since an unspecified epoch. |
| `os.rdtsc()` | Returns the CPU cycle counter value (x86 RDTSC). |
| `os.rdtscp()` | Returns the CPU cycle counter and processor ID (x86 RDTSCP). |

## Lazy-loaded native function globals

The runtime loads `citizen:/scripting/lua/natives_loader.lua` during bootstrap when lazy archive-backed native loading is active. This script installs a metatable on `_G` that materialises native functions on first access.

When an unknown global name is accessed:

1. The metatable's `__index` checks an internal `nilCache`.
2. If not cached as missing, it calls `Citizen.LoadNative(name)`.
3. If the result is a Lua function, it is cached directly in `_G`.
4. If the result is a Lua source string, it is compiled and executed in a sandboxed environment (`nativeEnv`), and the resulting global is cached.
5. If the name is unknown, it is memoised in `nilCache` to prevent repeated lookups.

The set of available native names depends on the platform (game build, client vs server).

### Core ScriptEngine natives

These natives are registered via `fx::ScriptEngine` and are always available regardless of game build. They become accessible through the lazy-loading mechanism.

| Global name | Signature | Side | Behaviour |
|-------------|-----------|------|-----------|
| `GetCurrentResourceName()` | `→ string` | Both | Returns the name of the current resource. |
| `GetInvokingResource()` | `→ string or nil` | Both | Returns the name of the resource that triggered the current call, or `nil`. |
| `IsDuplicityVersion()` | `→ boolean` | Both | `true` on server, `false` on client. |
| `ExecuteCommand(command)` | `string → nil` | Both | Execute a console command. |
| `RegisterCommand(name, fn [, restricted])` | `string, function [, boolean] → nil` | Both | Register a chat or console command handler. |
| `GetRegisteredCommands()` | `→ table` | Both | Returns an array of `{ name, restricted }` tables. |
| `GetResourceCommands(resource)` | `string → table` | Both | Returns commands registered by the specified resource. |
| `GetNumResources()` | `→ integer` | Both | Returns the total number of resources. |
| `GetResourceByFindIndex(i)` | `integer → string` | Both | Returns the name of the `i`-th resource (1-indexed). |
| `GetResourceState(name)` | `string → string` | Both | Returns the resource state: `"started"`, `"stopped"`, `"starting"`, `"stopping"`. |
| `GetInstanceId()` | `→ integer` | Both | Returns the script runtime instance ID. |
| `IsAceAllowed(object)` | `string → boolean` | Both | Check the Active Community Environment access control for the specified object. |
| `IsPrincipalAceAllowed(principal, object)` | `string, string → boolean` | Both | Check ACE for a specific principal identifier. |
| `GetStateBagValue(bag, key)` | `string, string → any` | Both | Read a value from a state bag. |
| `SetStateBagValue(bag, key, value, size, replicated)` | `string, string, string, integer, boolean → nil` | Both | Write a value to a state bag. The `value` is already-serialised msgpack data. |
| `AddStateBagChangeHandler(keyFilter, bagFilter, cb)` | `string, string, function → integer` | Both | Register a handler for state-bag changes. Returns a cookie for removal. |
| `RemoveStateBagChangeHandler(cookie)` | `integer → nil` | Both | Remove a previously registered state-bag change handler. |
| `STATE_BAG_HAS_KEY(bag, key)` | `string, string → boolean` | Both | Check whether a state bag contains a specific key. |
| `TriggerEventInternal(name, payload, len)` | `string, string, integer → nil` | Both | Low-level event trigger with raw msgpack payload. |
| `RegisterResourceAsEventHandler(name)` | `string → nil` | Both | Register the current resource as a handler for an event name. |
| `CancelEvent()` | `→ nil` | Both | Cancel the current event dispatch. |
| `WasEventCanceled()` | `→ boolean` | Both | Check whether the current event was cancelled. |

### Game and network natives

Thousands of game-specific and network-specific native functions are available through the same lazy-loading mechanism. The exact set depends on:

- the game build (GTA Five, RDR3, GTA NY)
- the platform (client vs server)
- the native archive mounted for that build

These include functions such as `GetPlayerName`, `SetVehicleColours`, `NetworkGetEntityFromNetworkId`, and all other natives documented by Cfx.re.

### Native wrapper environment helpers

The `nativeEnv` sandbox used during native wrapper compilation exposes these helpers to the compiled wrapper code:

| Helper | Description |
|--------|-------------|
| `Global` | Mirrors assignments into the actual global table `_G`. |
| `_mfr` | `Citizen.GetFunctionReference` for creating function references. |
| `_obj` | `msgpack` library reference. |
| `_ch` | Hash conversion helper. |
| `Citizen.InvokeNative` | For native calls from wrapper code. |
| `Citizen.InvokeNative2` | Alternative native-call path (client only). |
| `Citizen.GetNative` | Native lookup (client only). |
| Pointer/result helpers | `_i`, `_f`, `_v`, `_r`, `_ri`, `_rf`, `_rl`, `_s`, `_rv`, `_ro`, `_in`, `_ii`, `_fi` — shorthand bindings for `Citizen.PointerValueInt`, `Citizen.PointerValueFloat`, `Citizen.PointerValueVector`, `Citizen.ReturnResultAnyway`, `Citizen.ResultAsInteger`, `Citizen.ResultAsFloat`, `Citizen.ResultAsLong`, `Citizen.ResultAsString`, `Citizen.ResultAsVector`, `Citizen.ResultAsObject`, etc. |

## Removed and replaced globals

### Removed from `_G`

| Global | Replaced by |
|--------|-------------|
| `dofile` | **Removed.** Not available. |
| `loadfile` | **Removed.** Not available. |

### Replaced in `_G`

| Global | Replacement |
|--------|-------------|
| `print(...)` | Custom implementation that routes output through `Citizen.Trace`/`ScriptTrace` instead of `stdout`. |
| `require(modname)` | Custom implementation (`Lua_Require`) that only recognises `"lmprof"` (Lua profiler) and `"glm"` (GLM math library). Any other module name fails with `module '%s' not found`. |
| `warn(msg1, ...)` | Custom warning handler installed via `lua_setwarnf` that routes warnings through `Lua_Warn` with channel prefix. |

## Interaction with other spec files

- `specs/lua-runtime-reference.md` — runtime bootstrap sequence, scheduler coroutine model, native-invocation semantics, and detailed scheduler-layer behaviour.
- `specs/lua-function-reference-reference.md` — function-reference representation, serialisation, callable-proxy tables, and async-return adaptation across bridges (events, exports, NUI, state bags).
- `specs/lua-environment-isolation-reference.md` — per-resource runtime boundaries, cross-resource bridge semantics, client/server separation, and cleanup guarantees.
- `specs/lua-native-binding-reference.md` — native-build selection, lazy materialisation, argument marshalling, and result coercion for game natives.
- `specs/lua-manifest-reference.md` — manifest loader environment and directive interception.
- `specs/event-system-reference.md` — host event-dispatch architecture and routing.
- `specs/nui-callback-reference.md` — NUI callback lifecycle and strict-mode behaviour.
