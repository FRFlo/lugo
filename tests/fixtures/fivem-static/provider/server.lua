local function GetGreeting(name)
  return 'hello ' .. name
end
exports('GetGreeting', GetGreeting)
AddEventHandler('provider:ping', function() end)
