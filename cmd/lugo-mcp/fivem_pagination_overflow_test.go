package main

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFiveMPaginationLargeLimitDoesNotOverflow(t *testing.T) {
	limit := math.MaxInt
	data, err := marshalFivemList([]map[string]any{{"name": "a"}, {"name": "b"}}, fivemArgs{Limit: &limit, Cursor: "1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil || len(page.Items) != 1 || page.Total != 2 {
		t.Fatalf("page=%s err=%v", data, err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fxmanifest.lua"), []byte("fx_version 'cerulean'\ngame 'gta5'\nserver_script 'server.lua'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server.lua"), []byte("TriggerEvent('one')\nTriggerEvent('two')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	result, err := s.fivemEvents(context.Background(), highLevelRequest(t, map[string]any{"limit": limit, "cursor": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &page); err != nil || page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("events=%s err=%v", text, err)
	}
}
