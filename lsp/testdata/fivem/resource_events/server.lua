--[[@server_registration]]AddEventHandler("server:playerReady", function(source, name)
	print("player ready", source, name)
end)

TriggerClientEvent("--[[@server_hover]]shared:syncData", -1, {ready = true})

--[[@server_net_registration]]RegisterNetEvent("--[[@event_register_def]]shared:requestSync")

--[[@server_direction_error]]TriggerServerEvent("shared:requestSync")

TriggerClientEvent("--[[@server_shared_hover]]shared:bidirectionalNet", -1)
