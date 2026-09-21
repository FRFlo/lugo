CreateThread(function()
    -- Exercise the event path when the client resource is loaded.
    TriggerServerEvent('lugo_runtime_smoke:event')
    SendNUIMessage({ action = 'lugo-runtime-smoke' })
end)

RegisterNUICallback('lugo_runtime_smoke', function(_, callback)
    callback({ ok = true })
end)
