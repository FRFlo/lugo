local ped = --[[@native_client_call]]PlayerPedId()
local signature = PlayerPedId(--[[@native_native_signature]])
local nativeCompletion = --[[@native_native_completion]]PlayerPedId
local deprecated = --[[@native_native_deprecated]]ReleaseMissionAudioBank()
local legacyOnly = --[[@native_client_legacy_hidden]]GetVehicleMaxNumberOfPassengers
local serverOnly = --[[@native_client_server_hidden]]GetInvokingResource

return ped, legacyOnly, serverOnly
