local greeting = exports.provider:GetGreeting('player')
TriggerEvent('provider:ping')
TriggerEvent('provider:missing') -- intentionally unresolved interaction
SendNUIMessage({ type = 'greeting', text = greeting })
