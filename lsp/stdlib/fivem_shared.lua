---@meta

---@class CitizenRuntime
Citizen = {}

---Schedules a Lua coroutine to run on the next runtime tick.
---@param handler fun(...: any)
---@return thread
function Citizen.CreateThread(handler) end

---Schedules a Lua coroutine immediately.
---@param handler fun(...: any)
---@return thread
function Citizen.CreateThreadNow(handler) end

---Yields the current coroutine for the given number of milliseconds.
---@param msec integer
function Citizen.Wait(msec) end

---Runs a callback after the given number of milliseconds.
---@param msec integer
---@param callback fun(...: any)
---@return integer
function Citizen.SetTimeout(msec, callback) end

---Cancels a timeout handle returned by SetTimeout.
---@param handle integer
function Citizen.ClearTimeout(handle) end

---Waits for a promise-like object and returns its resolved values.
---@param awaitable any
---@return any ...
function Citizen.Await(awaitable) end

---Invokes a native by hash.
---@param hash integer|string
---@param ... any
---@return any ...
function Citizen.InvokeNative(hash, ...) end

---Loads a native binding by hash.
---@param hash integer|string
---@return function
function Citizen.LoadNative(hash) end

---Emits a runtime trace line.
---@param message string
function Citizen.Trace(message) end

---Yields the current coroutine for the given number of milliseconds.
---@param msec integer
function Wait(msec) end

---Schedules a Lua coroutine to run on the next runtime tick.
---@param handler fun(...: any)
---@return thread
function CreateThread(handler) end

---Runs a callback after the given number of milliseconds.
---@param msec integer
---@param callback fun(...: any)
---@return integer
function SetTimeout(msec, callback) end

---Cancels a timeout handle returned by SetTimeout.
---@param handle integer
function ClearTimeout(handle) end

---Registers a local event handler.
---@param eventName string
---@param handler fun(...: any)
---@return integer
function AddEventHandler(eventName, handler) end

---Removes a local event handler.
---@param handler integer
function RemoveEventHandler(handler) end

---Marks an event as safe for network use and optionally registers a handler.
---@param eventName string
---@param handler? fun(...: any)
function RegisterNetEvent(eventName, handler) end

---Triggers a local event.
---@param eventName string
---@param ... any
function TriggerEvent(eventName, ...) end

---Cancels the currently executing event.
function CancelEvent() end

---Returns whether the current event has been canceled.
---@return boolean
function WasEventCanceled() end

---@class promise
promise = {}

---Creates a new deferred promise.
---@return promise
function promise.new() end

---Resolves when all promises resolve.
---@param promises promise[]
---@return promise
function promise.all(promises) end

---@param value any
function promise:resolve(value) end

---@param reason any
function promise:reject(reason) end

---@param onResolve? fun(value: any)
---@param onReject? fun(reason: any)
---@return promise
function promise:next(onResolve, onReject) end

---@param onReject fun(reason: any)
---@return promise
function promise:catch(onReject) end

---@class StateBag
local state_bag = {}

---Gets a state bag value.
---@param key string
---@return any
function state_bag:get(key) end

---Sets a state bag value.
---@param key string
---@param value any
---@param replicated? boolean
function state_bag:set(key, value, replicated) end

---@type StateBag
GlobalState = {}

---@class EntityHandle
---@field state StateBag
local entity_handle = {}

---Returns an entity wrapper exposing its state bag.
---@param handle integer
---@return EntityHandle
function Entity(handle) end

---@class PlayerHandle
---@field state StateBag
local player_handle = {}

---Returns a player wrapper exposing its state bag.
---@param handle integer|string
---@return PlayerHandle
function Player(handle) end
