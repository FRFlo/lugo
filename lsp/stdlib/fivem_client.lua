---@meta

---Triggers an event on the server.
---@param eventName string
---@param ... any
function TriggerServerEvent(eventName, ...) end

---Triggers a latent event on the server.
---@param eventName string
---@param bytesPerSecond integer
---@param ... any
function TriggerLatentServerEvent(eventName, bytesPerSecond, ...) end

---Registers a NUI callback handler.
---@param callbackType string
---@param handler fun(data: any, cb: fun(response: any))
function RegisterNUICallback(callbackType, handler) end

---Sends a JSON-serializable message to NUI.
---@param message table
---@return boolean
function SendNUIMessage(message) end

---@class LocalPlayerHandle
---@field state StateBag
LocalPlayer = {}

---Gets a client native function by hash.
---@param hash integer|string
---@return function
function Citizen.GetNative(hash) end
