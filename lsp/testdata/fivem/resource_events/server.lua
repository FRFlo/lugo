--[[@server_registration]]AddEventHandler("server:playerReady", function(source, name)
	print("player ready", source, name)
end)

--[[@server_hover]]TriggerClientEvent("shared:syncData", -1, {ready = true})

--[[@server_net_registration]]RegisterNetEvent("shared:requestSync")

--[[@server_direction_error]]TriggerServerEvent("shared:requestSync")