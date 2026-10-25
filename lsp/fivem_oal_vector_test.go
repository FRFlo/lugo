package lsp

import "testing"

func TestFiveMOALVectorArgumentDiagnostics(t *testing.T) {
	tests := []struct {
		name          string
		oal           bool
		wantOALVector int
	}{
		{name: "standard vector unpacking", oal: false, wantOALVector: 0},
		{name: "OAL requires explicit components", oal: true, wantOALVector: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			manifest := "fx_version 'cerulean'\ngame 'gta5'\n"
			if tt.oal {
				manifest += "use_experimental_fxv2_oal 'yes'\n"
			}
			manifest += "client_script 'client.lua'\n"
			h.writeWorkspaceFile("resource/fxmanifest.lua", manifest)
			h.writeWorkspaceFile("resource/client.lua", `local coords = vector3(1, 2, 3)
SetEntityCoords(PlayerPedId(), coords)
local currentCoords = GetEntityCoords(PlayerPedId())
SetEntityCoords(PlayerPedId(), currentCoords)
SetEntityCoords(PlayerPedId(), coords.x, coords.y, coords.z)
SetHandlingVector('AIRTUG', 'CHandlingData', 'vecCentreOfMassOffset', coords)
`)
			h.reindex()

			diags := h.diagnostics("resource/client.lua")
			count := 0
			for _, diag := range diags {
				if diag.Code == "fivem-oal-vector-argument" {
					count++
				}
			}
			if count != tt.wantOALVector {
				t.Fatalf("OAL vector diagnostic count = %d, want %d; diagnostics: %#v", count, tt.wantOALVector, diags)
			}
		})
	}
}

func TestFiveMOALVectorSettingIsResourceScoped(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	source := `local coords = GetEntityCoords(PlayerPedId())
SetEntityCoords(PlayerPedId(), coords)
`
	h.writeWorkspaceFile("oal/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nuse_experimental_fxv2_oal 'yes'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("oal/client.lua", source)
	h.writeWorkspaceFile("standard/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\n")
	h.writeWorkspaceFile("standard/client.lua", source)
	h.reindex()

	if !hasDiagnosticCode(h.diagnostics("oal/client.lua"), "fivem-oal-vector-argument") {
		t.Fatal("OAL-enabled resource did not report implicit vector expansion")
	}
	if hasDiagnosticCode(h.diagnostics("standard/client.lua"), "fivem-oal-vector-argument") {
		t.Fatal("resource without OAL setting reported an OAL-specific vector diagnostic")
	}
}
