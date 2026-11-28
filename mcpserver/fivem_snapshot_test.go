package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFiveMEventsUseIndexedDocumentEvents(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"resource/fxmanifest.lua": "fx_version 'cerulean'\ngame 'gta5'\nclient_script 'client.lua'\nserver_script 'server.lua'\nshared_script 'shared.lua'\n",
		"resource/client.lua":     "RegisterNetEvent('fixture:client', function() end)\nTriggerServerEvent('fixture:server')\n",
		"resource/server.lua":     "AddEventHandler('fixture:server', function() end)\nTriggerClientEvent('fixture:client', -1)\n",
		"resource/shared.lua":     "RegisterNetEvent('fixture:shared')\nTriggerEvent('fixture:local')\n",
		"other/fxmanifest.lua":    "fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n",
		"other/server.lua":        "AddEventHandler('other:only', function() end)\n",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	call := func(args map[string]any) []map[string]any {
		t.Helper()
		result, err := s.fivemEvents(context.Background(), highLevelRequest(t, args))
		if err != nil {
			t.Fatal(err)
		}
		text := result.Content[0].(*mcp.TextContent).Text
		var items []map[string]any
		if err := json.Unmarshal([]byte(text), &items); err != nil {
			var page struct {
				Items []map[string]any `json:"items"`
			}
			if pageErr := json.Unmarshal([]byte(text), &page); pageErr != nil {
				t.Fatal(err)
			}
			return page.Items
		}
		return items
	}
	all := call(map[string]any{})
	if len(all) < 7 {
		t.Fatalf("events = %v; want handlers and triggers from client/server/shared scripts", all)
	}
	for _, item := range all {
		if item["resource"] != "resource" && item["resource"] != "other" {
			t.Errorf("event assigned unexpected resource: %v", item)
		}
	}
	filtered := call(map[string]any{"resource": "resource", "limit": 2, "detail": false})
	if len(filtered) != 2 {
		t.Fatalf("filtered page length = %d, want 2: %v", len(filtered), filtered)
	}
	for _, item := range filtered {
		if item["resource"] != "resource" {
			t.Errorf("resource filter returned %v", item)
		}
	}
	var page struct {
		Total      int    `json:"total"`
		NextCursor string `json:"nextCursor"`
	}
	result, err := s.fivemEvents(context.Background(), highLevelRequest(t, map[string]any{"resource": "resource", "limit": 2, "detail": false}))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total < 6 || page.NextCursor != "2" {
		t.Fatalf("pagination = %+v, want total events and next cursor 2", page)
	}
}

func TestFiveMReadersConcurrentWithReindex(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fxmanifest.lua"), []byte("fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\nserver_export 'snapshot'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server.lua"), []byte("AddEventHandler('snapshot:event', function() end)\nexports('snapshot', function() end)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 3 {
				if _, err := s.reindex(context.Background(), highLevelRequest(t, map[string]any{})); err != nil {
					errs <- err
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 3 {
				if _, err := s.fivemEvents(context.Background(), highLevelRequest(t, map[string]any{})); err != nil {
					errs <- err
				}
				if _, err := s.fivemExports(context.Background(), highLevelRequest(t, map[string]any{})); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	result, err := s.fivemExports(context.Background(), highLevelRequest(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	var exports []struct {
		Resource string   `json:"resource"`
		Server   []string `json:"server"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &exports); err != nil {
		t.Fatal(err)
	}
	if len(exports) != 1 || len(exports[0].Server) != 1 || exports[0].Server[0] != "snapshot" {
		t.Fatalf("exports after concurrent reindex = %+v, want stable server export", exports)
	}
}
