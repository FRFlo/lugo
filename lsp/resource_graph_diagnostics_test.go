package lsp

import (
	"testing"
)

func TestWorkspaceResourceGraphDiagnosticsUseManifestRanges(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("alpha/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
dependency 'beta'
dependency 'alpha'
dependency 'missing'
dependency 'virtual'
`)
	h.writeWorkspaceFile("beta/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
dependency 'alpha'
`)
	h.writeWorkspaceFile("provider_one/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
provide 'virtual'
`)
	h.writeWorkspaceFile("provider_two/fxmanifest.lua", `fx_version 'cerulean'
game 'gta5'
provide 'virtual'
`)
	h.reindex()

	diags := h.diagnostics("alpha/fxmanifest.lua")
	want := map[string]bool{
		"fivem-circular-dependency":  false,
		"fivem-self-dependency":      false,
		"fivem-missing-dependency":   false,
		"fivem-ambiguous-dependency": false,
	}
	for _, diag := range diags {
		if _, ok := want[diag.Code]; ok {
			want[diag.Code] = true
			if diag.Range.Start == diag.Range.End {
				t.Errorf("%s has no manifest-related range: %+v", diag.Code, diag)
			}
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("workspace diagnostics missing %s (all diagnostics: %+v)", code, diags)
		}
	}
}
