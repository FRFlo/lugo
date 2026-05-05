--[[@shared_registration]]AddEventHandler("shared:configLoaded", function(config)
	print("config loaded", config)
end)

TriggerEvent("--[[@shared_hover]]shared:reloadUI")

TriggerEvent("--[[@event_trigger_def]]shared:reloadUI")

AddEventHandler("--[[@event_handler_def]]shared:reloadUI", function()
	print("reload ui")
end)

AddEventHandler("--[[@event_add_handler_def]]shared:requestSync", function()
	print("request sync")
end)

--[[@shared_net_registration]]RegisterNetEvent("shared:bidirectionalNet", function(payload)
	print("bidirectional net", payload)
end)

TriggerServerEvent("--[[@shared_trigger_server]]shared:bidirectionalNet")
TriggerClientEvent("--[[@shared_trigger_client]]shared:bidirectionalNet", -1)

--[[@shared_wildcard]]AddEventHandler("*", function(eventName, ...)
	print("event fired", eventName)
end)
