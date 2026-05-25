---@meta
io = {}

---@type file*
io.stderr = nil

---@type file*
io.stdin = nil

---@type file*
io.stdout = nil

---Equivalent to file:close(). Without a file, closes the default output file.
---@param file? file*
---@return boolean|nil, string?, integer?
function io.close(file) end

---Flushes the default output file.
function io.flush() end

---Opens the given file name in read mode and returns an iterator function that works like file:lines(...).
---@param filename? string
---@param ... string|integer
---@return function
function io.lines(filename, ...) end

---This function opens a file, in the mode specified in the string mode.
---@param filename string
---@param mode? "r"|"w"|"a"|"r+"|"w+"|"a+"|"rb"|"wb"|"ab"|"r+b"|"w+b"|"a+b"
---@return file*|nil, string?, integer?
function io.open(filename, mode) end

---Starts program prog in a separated process and returns a file handle that you can use to read data from this program (if mode is "r") or to write data to this program (if mode is "w").
---@param prog string
---@param mode? "r"|"w"
---@return file*|nil, string?, integer?
function io.popen(prog, mode) end

---@class file*
local file_handle = {}

---@class directory*
local directory_handle = {}

---Opens a directory and returns a directory handle.
---@param dirname string
---@return directory*|nil, string?, integer?
function io.readdir(dirname) end

---In case of success, returns a handle for a temporary file.
---@return file*
function io.tmpfile() end

---Checks whether obj is a valid file or directory handle.
---@param obj any
---@return "file"|"closed file"|"directory"|"closed directory"|nil
function io.type(obj) end

---Equivalent to io.output():write(...).
---@param ... string|number
---@return file*|nil, string?
function io.write(...) end

---Closes the file handle.
---@return boolean|nil, string?, integer?
function file_handle:close() end

---Flushes pending writes for the file handle.
---@return boolean|nil, string?
function file_handle:flush() end

---Returns an iterator over lines from the file handle.
---@param ... string|integer
---@return function
function file_handle:lines(...) end

---Reads from the file handle.
---@param ... string|integer
---@return any
function file_handle:read(...) end

---Seeks the file handle.
---@param whence? "set"|"cur"|"end"
---@param offset? integer
---@return integer|nil, string?
function file_handle:seek(whence, offset) end

---Sets the buffering mode for the file handle.
---@param mode "no"|"full"|"line"
---@param size? integer
---@return boolean|nil, string?
function file_handle:setvbuf(mode, size) end

---Writes to the file handle.
---@param ... string|number
---@return file*|nil, string?
function file_handle:write(...) end

---Closes the directory handle.
---@return boolean|nil, string?, integer?
function directory_handle:close() end

---Returns an iterator over directory entries.
---@return function
function directory_handle:lines() end
