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

---Returns the name and the value of the local variable with index local
---of the function at the given level of the call stack.
---@param thread? thread
---@param level integer
---@param local integer
---@return string|nil, any
function debug.getlocal(thread, level, local) end

---Assigns value to the local variable with index local of the function
---at the given level of the call stack. Returns the variable name or nil.
---@param thread? thread
---@param level integer
---@param local integer
---@param value any
---@return string|nil
function debug.setlocal(thread, level, local, value) end

---Returns the Lua registry table.
---@return table
function debug.getregistry() end

---Returns the Lua value associated to the given userdata.
---@param udata userdata
---@return any
function debug.getuservalue(udata) end

---Sets the given value as the Lua value associated to the given userdata.
---@param udata userdata
---@param value any
---@return userdata
function debug.setuservalue(udata, value) end

---Sets the debugging hook function.
---@param hook function
---@param mask string
---@param count? integer
function debug.sethook(hook, mask, count) end

---Returns the current hook function, hook mask, and hook count.
---@return function|nil, string, integer|nil
function debug.gethook() end

---Returns a unique identifier for the upvalue numbered n of the closure
---at the given level of the call stack.
---@param f function
---@param n integer
---@return lightuserdata
function debug.upvalueid(f, n) end

---Makes the n1-th upvalue of the Lua closure f1 refer to the n2-th
---upvalue of the Lua closure f2.
---@param f1 function
---@param n1 integer
---@param f2 function
---@param n2 integer
function debug.upvaluejoin(f1, n1, f2, n2) end
