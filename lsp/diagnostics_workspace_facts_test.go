package lsp

import (
	"fmt"
	"reflect"
	"testing"
)

func TestWorkspaceGraphDiagnosticsSnapshotRefresh(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("alpha/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'beta'\n")
	h.writeWorkspaceFile("beta/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'alpha'\n")
	h.reindex()
	uri := h.server.pathToURI(h.root + "/alpha/fxmanifest.lua")
	doc := h.server.Documents[uri]
	uncached := h.server.buildFiveMResourceGraphDiagnostics(doc)
	if !hasDiagnosticCode(uncached, "fivem-circular-dependency") {
		t.Fatalf("expected a cycle before updating the manifest: %+v", uncached)
	}
	facts := h.server.beginWorkspaceDiagnosticFacts()
	cached := h.server.buildFiveMResourceGraphDiagnostics(doc)
	h.server.endWorkspaceDiagnosticFacts(facts)
	if !reflect.DeepEqual(cached, uncached) {
		t.Fatalf("cached diagnostics differ: got %+v, want %+v", cached, uncached)
	}

	h.writeWorkspaceFile("beta/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n")
	h.reindex()
	facts = h.server.beginWorkspaceDiagnosticFacts()
	defer h.server.endWorkspaceDiagnosticFacts(facts)
	if got := h.server.buildFiveMResourceGraphDiagnostics(h.server.Documents[uri]); hasDiagnosticCode(got, "fivem-circular-dependency") {
		t.Fatalf("cycle survived the next publication: %+v", got)
	}
}

func TestWorkspaceDiagnosticFactsCacheIsPublicationScoped(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n")
	h.writeWorkspaceFile("resource/client.lua", "ExecuteCommand('cached')\n")
	h.server.refreshWorkspace()

	facts := h.server.beginWorkspaceDiagnosticFacts()
	t.Cleanup(func() { h.server.endWorkspaceDiagnosticFacts(facts) })
	if got := h.server.workspaceDiagnosticFacts(); got != facts {
		t.Fatal("workspace diagnostic facts were not cached for this publication")
	}

	h.server.endWorkspaceDiagnosticFacts(facts)
	if h.server.workspaceDiagnosticFacts() != nil {
		t.Fatal("workspace diagnostic facts survived their publication")
	}

	h.writeWorkspaceFile("resource/commands.lua", "RegisterCommand('cached', function() end)\n")
	h.server.refreshWorkspace()
	client := h.server.Documents[h.server.pathToURI(h.root+"/resource/client.lua")]
	if hasDiagnosticCode(h.server.buildFiveMCommandDiagnostics(client), "fivem-command-missing-declaration") {
		t.Fatal("command diagnostics did not rebuild facts for the next publication")
	}
}

func BenchmarkPublishWorkspaceDiagnosticsFacts(b *testing.B) {
	for _, documentCount := range []int{1, 100} {
		b.Run(fmt.Sprintf("documents=%d", documentCount), func(b *testing.B) {
			h := newFiveMFixtureHarnessWithoutIndex(b)
			h.writeWorkspaceFile("resource/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n")
			for i := 0; i < documentCount; i++ {
				h.writeWorkspaceFile(fmt.Sprintf("resource/client%d.lua", i), "RegisterCommand('command', function() end)\nExecuteCommand('command')\n")
			}
			h.server.refreshWorkspace()

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				h.server.publishWorkspaceDiagnostics()
				h.rpcOut.Reset()
			}
		})
	}
}
