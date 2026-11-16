package lsp

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestFiveMAllowedIncludesScopeOrderAndEarlyReturn(t *testing.T) {
	entry := func(kind FiveMResourceGraphExpansionKind, include string) FiveMResourceGraphExpansion {
		return FiveMResourceGraphExpansion{Kind: kind, Include: include}
	}
	node := &FiveMResourceGraphNode{
		SharedEntries: []FiveMResourceGraphExpansion{entry(FiveMResourceGraphExpansionLocal, "shared.lua"), entry(FiveMResourceGraphExpansionInclude, "@shared/first.lua"), entry(FiveMResourceGraphExpansionInclude, "@shared/second.lua")},
		ClientEntries: []FiveMResourceGraphExpansion{entry(FiveMResourceGraphExpansionInclude, "@client/client.lua"), entry(FiveMResourceGraphExpansionLocal, "client.lua")},
		ServerEntries: []FiveMResourceGraphExpansion{entry(FiveMResourceGraphExpansionInclude, "@server/server.lua")},
	}

	tests := []struct {
		name string
		env  FileEnv
		want []string
	}{
		{name: "client", env: EnvClient, want: []string{"@shared/first.lua", "@shared/second.lua", "@client/client.lua"}},
		{name: "server", env: EnvServer, want: []string{"@shared/first.lua", "@shared/second.lua", "@server/server.lua"}},
		{name: "shared", env: EnvShared, want: []string{"@shared/first.lua", "@shared/second.lua"}},
		{name: "unknown", env: EnvUnknown, want: []string{"@shared/first.lua", "@shared/second.lua"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			node.allowedIncludes(tt.env, func(include FiveMResourceGraphExpansion) bool {
				got = append(got, include.Include)
				return true
			})
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("allowed includes = %#v, want %#v", got, tt.want)
			}
		})
	}

	visits := 0
	stopped := node.allowedIncludes(EnvClient, func(FiveMResourceGraphExpansion) bool {
		visits++
		return visits < 2
	})
	if !stopped || visits != 2 {
		t.Fatalf("early stop = (%v, %d visits), want (true, 2 visits)", stopped, visits)
	}
}

func TestFiveMCrossResourceIncludeMatrixAndManifestUpdate(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	consumerRoot := filepath.Join(root, "consumer")
	providerRoot := filepath.Join(root, "provider")
	consumerManifest := filepath.Join(consumerRoot, "fxmanifest.lua")
	addFiveMTestDocument(t, s, consumerManifest, `
client_script '@inventory/client.lua'
shared_script '@inventory/shared_*.lua'
client_script 'client_consumer.lua'
shared_script 'shared_consumer.lua'
`)
	clientConsumer := addFiveMTestDocument(t, s, filepath.Join(consumerRoot, "client_consumer.lua"), "return true")
	sharedConsumer := addFiveMTestDocument(t, s, filepath.Join(consumerRoot, "shared_consumer.lua"), "return true")
	addFiveMTestDocument(t, s, filepath.Join(providerRoot, "fxmanifest.lua"), `
provide 'inventory'
client_script 'client.lua'
server_script 'server.lua'
shared_script { 'shared_one.lua', 'shared_two.lua' }
`)
	clientTarget := addFiveMTestDocument(t, s, filepath.Join(providerRoot, "client.lua"), "return true")
	serverTarget := addFiveMTestDocument(t, s, filepath.Join(providerRoot, "server.lua"), "return true")
	sharedTarget := addFiveMTestDocument(t, s, filepath.Join(providerRoot, "shared_two.lua"), "return true")

	check := func(name string, profile FiveMExecutionProfile, target *Document, want bool) {
		t.Helper()
		if got := s.canSeeFiveMCrossResourceInclude(profile, target); got != want {
			t.Errorf("%s visibility = %v, want %v", name, got, want)
		}
	}

	clientProfile := s.getDocumentFiveMProfile(clientConsumer)
	sharedProfile := s.getDocumentFiveMProfile(sharedConsumer)
	if clientProfile.Kind != FiveMProfileClient || sharedProfile.Kind != FiveMProfileShared {
		t.Fatalf("consumer profiles = (%s, %s), want client/shared", clientProfile.Kind.String(), sharedProfile.Kind.String())
	}
	check("exact include through provide alias", clientProfile, clientTarget, true)
	check("glob include through provide alias", clientProfile, sharedTarget, true)
	check("unmatched target path", clientProfile, serverTarget, false)
	check("shared scope excludes client include", sharedProfile, clientTarget, false)
	check("shared scope retains shared glob", sharedProfile, sharedTarget, true)

	// Re-registering a changed manifest updates the graph immediately; no stale
	// include authorization should survive removal from the manifest.
	addFiveMTestDocument(t, s, consumerManifest, "client_script 'client_consumer.lua'\nshared_script 'shared_consumer.lua'\n")
	check("removed include after manifest update", clientProfile, clientTarget, false)
}
