---@meta
debug = {}

---Returns a table with information about a function.
---@param thread? thread
---@param f function|integer
---@param what? string
---@return table|nil
function debug.getinfo(thread, f, what) end

---Returns the metatable of the given value or nil if it does not have one.
---@param value any
---@return table|nil
function debug.getmetatable(value) end

---Returns the name and the value of the upvalue with index up of the function.
---@param f function
---@param up integer
---@return string|nil, any
function debug.getupvalue(f, up) end

---Sets the metatable for the given value to the given table (which can be nil). Returns value.
---@param value any
---@param table table|nil
---@return any
function debug.setmetatable(value, table) end

---Returns a string with a traceback of the call stack.
---@param thread? thread
---@param message? string
---@param level? integer
---@return string
function debug.traceback(thread, message, level) end
