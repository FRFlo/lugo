--- @class Account
--- @field id number
--- @field balance number

--- @type Account
Core.Account = setmetatable({}, { __index = Core.BaseModel })
Core.Account.__index = Core.Account

function Core.Account:--[[@account_save_definition]]save()
end

function Core.Account:deposit(amount)
end

function Core.Account:withdraw(amount)
end

function Core.Account.create(id, balance)
end

--- @type Account
local a = {}

a:--[[@account_save_call]]save()

a:--[[@completion_class_method]]
