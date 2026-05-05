--[[@shared_registration]]AddEventHandler("shared:configLoaded", function(config)
	print("config loaded", config)
end)

--[[@shared_hover]]TriggerEvent("shared:reloadUI")

--[[@shared_wildcard]]AddEventHandler("*", function(eventName, ...)
	print("event fired", eventName)
end)