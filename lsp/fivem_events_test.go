package lsp

import "testing"

func TestFiveMEventFixtureLoading(t *testing.T) {
	h := newFiveMFixtureHarness(t, "resource_events")

	h.requireMarker("client_registration")
	h.requireMarker("client_hover")
	h.requireMarker("client_net_registration")
	h.requireMarker("client_handler_def")

	h.requireMarker("server_registration")
	h.requireMarker("server_hover")
	h.requireMarker("server_net_registration")
	h.requireMarker("server_direction_error")

	h.requireMarker("shared_registration")
	h.requireMarker("shared_hover")
	h.requireMarker("shared_wildcard")
}