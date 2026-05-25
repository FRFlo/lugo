---@meta
msgpack = {}

---@type any
msgpack.sentinel = nil

---@type string
msgpack._VERSION = ""

---Serializes a Lua value to a MessagePack binary string.
---@param data any
---@return string
function msgpack.pack(data) end

---Deserializes a MessagePack binary string to a Lua value.
---@param str string
---@return any
function msgpack.unpack(str) end

---Serializes multiple arguments as a packed array.
---@param ... any
---@return string
function msgpack.pack_args(...) end

---Sets string encoding mode.
---@param mode "unsigned"|"binary"|string
function msgpack.set_string(mode) end

---Sets integer encoding mode.
---@param mode "signed"|"unsigned"|string
function msgpack.set_integer(mode) end

---Sets array encoding mode.
---@param mode string
function msgpack.set_array(mode) end

---Sets number encoding mode.
---@param mode string
function msgpack.set_number(mode) end

---Registers a Lua type name to a MessagePack extension type ID.
---@param typeName string
---@param extId integer
function msgpack.settype(typeName, extId) end

---Registers extension pack/unpack handlers.
---@param tbl table
function msgpack.extend(tbl) end

---Clears extension handlers in an inclusive range of type IDs.
---@param from integer
---@param to integer
function msgpack.extend_clear(from, to) end

---Builds a raw MessagePack extension binary payload.
---@param tag integer
---@param data string
---@return string
function msgpack.build_ext(tag, data) end
