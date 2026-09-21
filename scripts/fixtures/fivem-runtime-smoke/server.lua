local smokeEventSeen = false

RegisterNetEvent('lugo_runtime_smoke:event', function()
    smokeEventSeen = true
end)

exports('smokeExport', function()
    return 'ok'
end)

-- The smoke runner requests this route after the resource has loaded. SetHttpHandler
-- is available on FXServer and keeps the runtime contract independent of a framework.
SetHttpHandler(function(request, response)
    if request.path ~= '/lugo-runtime-smoke' then
        response.writeHead(404)
        response.send('not found')
        return
    end

    response.writeHead(200, { ['Content-Type'] = 'application/json' })
    response.send(json.encode({
        manifest = true,
        event = smokeEventSeen,
        export = exports[GetCurrentResourceName()]:smokeExport() == 'ok',
        nui = true,
    }))
end)
