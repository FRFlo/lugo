---@meta
json = {}

---@type any
json.null = nil

---@type string
json.version = ""

---@type boolean
json.using_lpeg = false

---Encodes a Lua value as JSON.
---@param value any
---@param state? table
---@return string
function json.encode(value, state) end

---Decodes a JSON string to a Lua value.
---@param str string
---@param pos? integer
---@param nullval? any
---@param objectmeta? table
---@param arraymeta? table
---@return any
function json.decode(str, pos, nullval, objectmeta, arraymeta) end

---Quotes a Lua string as a JSON string literal.
---@param str string
---@return string
function json.quotestring(str) end

---Appends a newline to an encoder state buffer.
---@param state table
function json.addnewline(state) end

---Attempts to switch to LPeg-backed decoding.
---@return boolean
function json.use_lpeg() end

---Sets an option on the FiveM JSON binding.
---@param optname string
---@param value any
function json.setoption(optname, value) end
