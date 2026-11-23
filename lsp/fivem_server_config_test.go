package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFiveMServerConfigIncludesInOrder(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"server.cfg": "ensure first\nexec 'nested.cfg'\nstart last # comment\n",
		"nested.cfg": "ensure second; exec more.cfg\n",
		"more.cfg":   "ensure [group]\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := readFiveMServerConfig(filepath.Join(root, "server.cfg"))
	if !got.Complete || len(got.Problems) != 0 {
		t.Fatalf("unexpected errors: %#v", got)
	}
	want := []string{"first", "second", "[group]", "last"}
	if len(got.Commands) != len(want) {
		t.Fatalf("commands: %#v", got.Commands)
	}
	for i, name := range want {
		if got.Commands[i].Argument != name {
			t.Fatalf("command %d = %q, want %q", i, got.Commands[i].Argument, name)
		}
	}
}

func TestFiveMServerConfigReportsCyclesAndMissingFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "server.cfg")
	if err := os.WriteFile(path, []byte("exec server.cfg\nexec absent.cfg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readFiveMServerConfig(path)
	if got.Complete || len(got.Problems) != 2 {
		t.Fatalf("expected cycle and missing file: %#v", got)
	}
}

func TestFiveMServerConfigDiagnosticsPublishAndClear(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMServerConfig = true
	h.writeWorkspaceFile("server.cfg", "exec missing.cfg\n")
	h.server.publishFiveMServerConfigDiagnostics()
	body := h.lastNotification("textDocument/publishDiagnostics")
	var notification struct {
		Params PublishDiagnosticsParams `json:"params"`
	}
	if err := json.Unmarshal(body, &notification); err != nil {
		t.Fatal(err)
	}
	if notification.Params.URI != h.server.pathToURI(filepath.Join(h.root, "server.cfg")) || len(notification.Params.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", notification.Params)
	}
	h.resetRPC()
	h.writeWorkspaceFile("missing.cfg", "ensure chat\n")
	h.server.publishFiveMServerConfigDiagnostics()
	body = h.lastNotification("textDocument/publishDiagnostics")
	if err := json.Unmarshal(body, &notification); err != nil {
		t.Fatal(err)
	}
	if len(notification.Params.Diagnostics) != 0 {
		t.Fatalf("diagnostics were not cleared: %#v", notification.Params.Diagnostics)
	}
}

func TestFiveMEventLoadOrderDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMServerConfig = true
	h.writeWorkspaceFile("server.cfg", "ensure consumer\nexec resources.cfg\n")
	h.writeWorkspaceFile("resources.cfg", "ensure provider\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("consumer/main.lua", "TriggerEvent('example', function(v) end)\nCreateThread(function() TriggerEvent('example', function(v) end) end)\n")
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("provider/main.lua", "AddEventHandler('example', function(cb) cb(1) end)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "main.lua"))]
	diags := h.server.fiveMEventLoadOrderDiagnostics(doc)
	if len(diags) != 1 || diags[0].Code != "fivem-event-load-order" {
		t.Fatalf("expected one startup warning, got %#v", diags)
	}
	h.writeWorkspaceFile("server.cfg", "ensure provider\nensure consumer\n")
	if got := h.server.fiveMEventLoadOrderDiagnostics(doc); len(got) != 0 {
		t.Fatalf("unexpected warning for correct order: %#v", got)
	}
	h.writeWorkspaceFile("server.cfg", "ensure consumer\nexec missing.cfg\nensure provider\n")
	if got := h.server.fiveMEventLoadOrderDiagnostics(doc); len(got) != 0 {
		t.Fatalf("unexpected warning for incomplete config: %#v", got)
	}
}

func TestFiveMEventLoadOrderExplicitExternalConfig(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	external := filepath.Join(t.TempDir(), "custom.cfg")
	if err := os.WriteFile(external, []byte("ensure consumer\nensure provider\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.server.FiveMServerConfig = external
	h.server.DiagFiveMServerConfig = true
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("consumer/main.lua", "TriggerEvent('example')\n")
	h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("provider/main.lua", "AddEventHandler('example', function() end)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "main.lua"))]
	if got := h.server.fiveMEventLoadOrderDiagnostics(doc); len(got) != 1 {
		t.Fatalf("expected external config order warning, got %#v", got)
	}
}

func TestFiveMEventLoadOrderCategory(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMServerConfig = true
	h.writeWorkspaceFile("server.cfg", "ensure [consumers]\nensure [providers]\n")
	h.writeWorkspaceFile("[consumers]/consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("[consumers]/consumer/main.lua", "TriggerEvent('example')\n")
	h.writeWorkspaceFile("[providers]/provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("[providers]/provider/main.lua", "AddEventHandler('example', function() end)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "[consumers]", "consumer", "main.lua"))]
	if got := h.server.fiveMEventLoadOrderDiagnostics(doc); len(got) != 1 {
		t.Fatalf("expected category order warning, got %#v", got)
	}
	h.writeWorkspaceFile("server.cfg", "ensure [providers]\nensure [consumers]\n")
	if got := h.server.fiveMEventLoadOrderDiagnostics(doc); len(got) != 0 {
		t.Fatalf("unexpected warning after reorder: %#v", got)
	}
}

func TestFiveMServerConfigWatchedChangesRepublish(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.server.DiagFiveMServerConfig = true
	h.writeWorkspaceFile("server.cfg", "exec extra.cfg\n")
	h.writeWorkspaceFile("extra.cfg", "exec missing.cfg\n")
	h.server.publishFiveMServerConfigDiagnostics()
	h.writeWorkspaceFile("extra.cfg", "ensure resource\n")
	h.simulateWatchedFileChange(h.server.pathToURI(filepath.Join(h.root, "extra.cfg")))
	body := h.lastNotification("textDocument/publishDiagnostics")
	var notification struct {
		Params PublishDiagnosticsParams `json:"params"`
	}
	if err := json.Unmarshal(body, &notification); err != nil {
		t.Fatal(err)
	}
	if notification.Params.URI != h.server.pathToURI(filepath.Join(h.root, "extra.cfg")) || len(notification.Params.Diagnostics) != 0 {
		t.Fatalf("expected cleared diagnostics after watched edit, got %#v", notification.Params)
	}
}

func TestFiveMEventLoadOrderDependencies(t *testing.T) {
	for _, tc := range []struct {
		name, consumerDeps, middleDeps, config string
		wantWarning                            bool
	}{
		{"unrelated dependency", "dependency 'middle'\n", "", "ensure consumer\nensure provider\n", true},
		{"direct dependency", "dependency 'provider'\n", "", "ensure consumer\nensure provider\n", false},
		{"transitive dependency", "dependency 'middle'\n", "dependency 'provider'\n", "ensure consumer\nensure provider\n", false},
		{"unresolved dependency", "dependency 'missing'\n", "", "ensure consumer\nensure provider\n", false},
		{"dependency cycle", "dependency 'middle'\n", "dependency 'consumer'\n", "ensure consumer\nensure provider\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("server.cfg", tc.config)
			h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n"+tc.consumerDeps+"server_script 'main.lua'\n")
			h.writeWorkspaceFile("consumer/main.lua", "TriggerEvent('example')\n")
			h.writeWorkspaceFile("middle/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\n"+tc.middleDeps)
			h.writeWorkspaceFile("provider/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'main.lua'\n")
			h.writeWorkspaceFile("provider/main.lua", "AddEventHandler('example', function() end)\n")
			h.reindex()
			doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "main.lua"))]
			got := h.server.fiveMEventLoadOrderDiagnostics(doc)
			if (len(got) != 0) != tc.wantWarning {
				t.Fatalf("diagnostics = %#v, want warning = %v", got, tc.wantWarning)
			}
		})
	}
}

func TestFiveMEventLoadOrderProvidedDependency(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("server.cfg", "ensure consumer\nensure replacement\n")
	h.writeWorkspaceFile("consumer/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\ndependency 'original'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("consumer/main.lua", "TriggerEvent('example', function(value) print(value) end)\n")
	h.writeWorkspaceFile("replacement/fxmanifest.lua", "fx_version 'cerulean'\ngame 'gta5'\nprovide 'original'\nserver_script 'main.lua'\n")
	h.writeWorkspaceFile("replacement/main.lua", "AddEventHandler('example', function(cb) cb(42) end)\n")
	h.reindex()
	doc := h.server.Documents[h.server.pathToURI(filepath.Join(h.root, "consumer", "main.lua"))]
	if got := h.server.fiveMEventLoadOrderDiagnostics(doc); len(got) != 0 {
		t.Fatalf("replacement should start before consumer: %#v", got)
	}
	if got := doc.InferType(mustFindIdentNode(t, doc, "value")); got.Basics != TypeNumber {
		t.Fatalf("replacement callback type = %s, want number", got.Format())
	}
}
