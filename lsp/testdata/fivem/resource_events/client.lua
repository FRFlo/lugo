--[[@client_registration]]AddEventHandler("client:playerLoaded", function(source)
	print("player loaded", source)
end)

--[[@client_hover]]TriggerServerEvent("shared:requestSync")

--[[@client_net_registration]]RegisterNetEvent("shared:syncData", function(data)
	--[[@client_handler_def]]print("synced", data)
end)