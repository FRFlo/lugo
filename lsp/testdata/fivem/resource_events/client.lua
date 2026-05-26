--[[@client_registration]]AddEventHandler("client:playerLoaded", function(source)
	print("player loaded", source)
end)

TriggerServerEvent("--[[@client_hover]]shared:requestSync")

--[[@client_net_registration]]RegisterNetEvent("shared:syncData", function(data)
	--[[@client_handler_def]]print("synced", data)
end)

TriggerServerEvent("--[[@client_shared_hover]]shared:bidirectionalNet")

AddEventHandler("client:implicitSource", function()
	print(source)
end)
