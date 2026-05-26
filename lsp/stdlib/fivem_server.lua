---@meta

---Registers a server event and optionally installs a handler.
---@param eventName string
---@param handler? fun(...: any)
function RegisterServerEvent(eventName, handler) end

---Triggers a client event for one or more players.
---@param eventName string
---@param target integer|string|integer[]|string[]
---@param ... any
function TriggerClientEvent(eventName, target, ...) end

---Triggers a latent client event for one or more players.
---@param eventName string
---@param target integer|string|integer[]|string[]
---@param bytesPerSecond integer
---@param ... any
function TriggerLatentClientEvent(eventName, target, bytesPerSecond, ...) end

---Returns all connected player handles.
---@return string[]
function GetPlayers() end

---Returns player identifiers.
---@param playerSrc integer|string
---@return string[]
function GetPlayerIdentifiers(playerSrc) end

---Returns player tokens.
---@param playerSrc integer|string
---@return string[]
function GetPlayerTokens(playerSrc) end

---Returns a player's endpoint.
---@param playerSrc integer|string
---@return string
function GetPlayerEndpoint(playerSrc) end

---Performs an HTTP request.
---@param url string
---@param callback fun(statusCode: integer, body: string, headers: table, errorData?: string)
---@param method? string
---@param data? string
---@param headers? table
---@param options? table
function PerformHttpRequest(url, callback, method, data, headers, options) end

---Performs an HTTP request and waits for the response.
---@param url string
---@param method? string
---@param data? string
---@param headers? table
---@param options? table
---@return integer statusCode, string body, table headers, string? errorData
function PerformHttpRequestAwait(url, method, data, headers, options) end

---Prints to the server console/RCON stream.
---@param message string
function RconPrint(message) end

---Writes an RCON log line.
---@param message string
function RconLog(message) end
