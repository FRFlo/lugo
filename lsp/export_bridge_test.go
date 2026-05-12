package lsp

import "testing"

func TestExportBridgeLookup(t *testing.T) {
	t.Run("RegisterAndLookupClientExport", func(t *testing.T) {
		idx := NewGlobalIndex()
		bridge := &ExportBridge{Index: idx}

		entry := bridge.RegisterExport("file:///provider", GlobalIndexScopeClient, ExportData{
			Name:         "ping",
			Resource:     "ignored",
			Scope:        GlobalIndexScopeShared,
			SourceURI:    "file:///provider/client.lua",
			NodeID:       12,
			ResourceName: "provider",
		})
		if entry == nil || entry.Export == nil {
			t.Fatal("RegisterExport returned nil for client export")
		}
		if entry.Export.Resource != "file:///provider" || entry.Export.Scope != GlobalIndexScopeClient || entry.Export.Name != "ping" {
			t.Fatalf("registered client export = %+v, want resource+scope normalized", entry.Export)
		}

		if got := idx.LookupByScope("file:///provider", GlobalIndexScopeClient, "ping"); got == nil || got.Export == nil || got.Export.Name != "ping" {
			t.Fatalf("LookupByScope(client) = %+v, want ping export", got)
		}
	})

	t.Run("RegisterAndLookupServerExport", func(t *testing.T) {
		idx := NewGlobalIndex()
		bridge := &ExportBridge{Index: idx}

		entry := bridge.RegisterExport("file:///provider", GlobalIndexScopeServer, ExportData{
			Name:         "pong",
			SourceURI:    "file:///provider/server.lua",
			NodeID:       34,
			ResourceName: "provider",
		})
		if entry == nil || entry.Export == nil {
			t.Fatal("RegisterExport returned nil for server export")
		}
		if got := idx.LookupByScope("file:///provider", GlobalIndexScopeServer, "pong"); got == nil || got.Export == nil || got.Export.Name != "pong" {
			t.Fatalf("LookupByScope(server) = %+v, want pong export", got)
		}
	})

	t.Run("SharedExportVisibleToBothScopes", func(t *testing.T) {
		idx := NewGlobalIndex()
		bridge := &ExportBridge{Index: idx}

		bridge.RegisterExport("file:///provider", GlobalIndexScopeShared, ExportData{Name: "shared_ping", SourceURI: "file:///provider/shared.lua", NodeID: 56, ResourceName: "provider"})
		connectExportBridgeResources(idx, "file:///consumer", "file:///provider")

		if got := bridge.LookupExport("shared_ping", "file:///consumer", GlobalIndexScopeClient); got == nil || got.Export == nil || got.Export.Scope != GlobalIndexScopeShared {
			t.Fatalf("client lookup for shared export = %+v, want shared export", got)
		}
		if got := bridge.LookupExport("shared_ping", "file:///consumer", GlobalIndexScopeServer); got == nil || got.Export == nil || got.Export.Scope != GlobalIndexScopeShared {
			t.Fatalf("server lookup for shared export = %+v, want shared export", got)
		}
	})

	t.Run("CrossResourceLookupViaDependencies", func(t *testing.T) {
		idx := NewGlobalIndex()
		bridge := &ExportBridge{Index: idx}

		bridge.RegisterExport("file:///provider", GlobalIndexScopeClient, ExportData{Name: "dep_ping", SourceURI: "file:///provider/client.lua", NodeID: 78, ResourceName: "provider"})
		connectExportBridgeResources(idx, "file:///consumer", "file:///missing", "file:///provider")

		if got := bridge.LookupExport("dep_ping", "file:///consumer", GlobalIndexScopeClient); got == nil || got.Export == nil || got.Export.SourceURI != "file:///provider/client.lua" {
			t.Fatalf("dependency lookup = %+v, want provider export", got)
		}
		if got := bridge.LookupExport("dep_ping", "file:///consumer", GlobalIndexScopeServer); got != nil {
			t.Fatalf("server lookup for client export = %+v, want nil", got)
		}
	})

	t.Run("CrossResourceLookupViaDependents", func(t *testing.T) {
		idx := NewGlobalIndex()
		bridge := &ExportBridge{Index: idx}

		bridge.RegisterExport("file:///provider", GlobalIndexScopeServer, ExportData{Name: "dependent_ping", SourceURI: "file:///provider/server.lua", NodeID: 90, ResourceName: "provider"})
		connectExportBridgeDependents(idx, "file:///provider", "file:///consumer")

		if got := bridge.LookupExport("dependent_ping", "file:///consumer", GlobalIndexScopeServer); got == nil || got.Export == nil || got.Export.SourceURI != "file:///provider/server.lua" {
			t.Fatalf("dependent lookup = %+v, want provider export", got)
		}
	})

	t.Run("MissingExportReturnsNil", func(t *testing.T) {
		idx := NewGlobalIndex()
		bridge := &ExportBridge{Index: idx}

		connectExportBridgeResources(idx, "file:///consumer", "file:///provider")

		if got := bridge.LookupExport("missing", "file:///consumer", GlobalIndexScopeClient); got != nil {
			t.Fatalf("LookupExport returned %+v, want nil", got)
		}
	})
}

func connectExportBridgeDependents(idx *GlobalIndex, provider ResourceURI, consumers ...ResourceURI) {
	idx.EnsureResource(provider)
	for _, consumer := range consumers {
		idx.EnsureResource(consumer)
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.Resources[provider].Dependents = append([]ResourceURI(nil), consumers...)
}

func connectExportBridgeResources(idx *GlobalIndex, consumer ResourceURI, providers ...ResourceURI) {
	idx.EnsureResource(consumer)
	for _, provider := range providers {
		idx.EnsureResource(provider)
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.Resources[consumer].Dependencies = append([]ResourceURI(nil), providers...)
	for _, provider := range providers {
		idx.Resources[provider].Dependents = append(idx.Resources[provider].Dependents, consumer)
	}
}
