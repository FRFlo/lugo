package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFiveMPaginationIsDeterministic(t *testing.T) {
	limit := 1
	data, err := marshalFivemList([]map[string]any{{"name": "a"}, {"name": "b"}}, fivemArgs{Limit: &limit}, func(item map[string]any) map[string]any { return item })
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"nextCursor"`
		Truncated  bool             `json:"truncated"`
		Total      int              `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Total != 2 || !page.Truncated || page.NextCursor != "1" {
		t.Fatalf("page = %+v", page)
	}
}

func TestFiveMListDefaultsRemainArraysAndAcceptConsistentParameters(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fxmanifest.lua"), []byte("fx_version 'cerulean'\ngame 'gta5'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	for _, call := range []func(*mcp.CallToolRequest) (*mcp.CallToolResult, error){
		func(req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.fivemResources(context.Background(), req)
		},
		func(req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.fivemEvents(context.Background(), req)
		},
		func(req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.fivemExports(context.Background(), req)
		},
	} {
		result, err := call(highLevelRequest(t, map[string]any{}))
		if err != nil {
			t.Fatal(err)
		}
		var items []any
		if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &items); err != nil {
			t.Fatalf("default output is not array: %v", err)
		}
		result, err = call(highLevelRequest(t, map[string]any{"limit": 1, "cursor": "0", "detail": false}))
		if err != nil {
			t.Fatal(err)
		}
		var page map[string]any
		if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &page); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"items", "nextCursor", "truncated", "total"} {
			if _, ok := page[key]; !ok {
				t.Errorf("pagination missing %q", key)
			}
		}
	}
}
