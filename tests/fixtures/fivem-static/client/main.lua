local function greet(player)
  return Config.prefix .. player
end

RegisterNetEvent('lugo:greet', function(player)
  print(greet(player))
end)

return greet
