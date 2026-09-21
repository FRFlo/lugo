package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func highLevelRequest(t *testing.T, args map[string]any) *mcp.CallToolRequest {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: data}}
}

func TestSymbolContextIsStructuredAndUsesRelativePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local value = 1\nreturn value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	result, err := s.symbolContext(context.Background(), highLevelRequest(t, map[string]any{
		"path": "main.lua", "line": 1, "character": 7,
	}))
	if err != nil {
		t.Fatal(err)
	}
	structured, ok := result.StructuredContent.(json.RawMessage)
	if len(result.Content) != 1 || !ok || len(structured) == 0 {
		t.Fatalf("symbol context must provide text and structured JSON: %+v", result)
	}
	var got map[string]any
	if err := json.Unmarshal(structured, &got); err != nil {
		t.Fatal(err)
	}
	if got["path"] != "main.lua" {
		t.Fatalf("path = %v, want workspace-relative path", got["path"])
	}
	if !strings.Contains(string(structured), "references") {
		t.Fatalf("symbol context missing references: %s", structured)
	}
}

func TestSymbolContextRejectsPathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	_, err = s.symbolContext(context.Background(), highLevelRequest(t, map[string]any{
		"path": "../outside.lua", "line": 0, "character": 0,
	}))
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("unsafe path error = %v, want workspace boundary error", err)
	}
}

func TestWorkspaceStatusExposesFreshnessAndDetectsChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.lua")
	if err := os.WriteFile(path, []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	result, err := s.workspaceStatus(context.Background(), highLevelRequest(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	var before struct {
		Revision   string `json:"revision"`
		SourceHash string `json:"sourceHash"`
		IndexedAt  string `json:"indexedAt"`
		Fresh      bool   `json:"fresh"`
	}
	if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &before); err != nil {
		t.Fatal(err)
	}
	if before.Revision == "" || before.SourceHash == "" || before.IndexedAt == "" || !before.Fresh {
		t.Fatalf("initial freshness = %+v", before)
	}
	if err := os.WriteFile(path, []byte("return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = s.workspaceStatus(context.Background(), highLevelRequest(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	var after struct {
		Revision, SourceHash string
		Fresh                bool `json:"fresh"`
	}
	if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &after); err != nil {
		t.Fatal(err)
	}
	if after.Fresh || after.SourceHash == before.SourceHash || after.Revision != before.Revision {
		t.Fatalf("changed freshness = %+v, before revision %q", after, before.Revision)
	}
}

func TestReindexValidatesSelectiveRelativePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	result, err := s.reindex(context.Background(), highLevelRequest(t, map[string]any{"paths": []string{"main.lua"}}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Paths    []string `json:"paths"`
		Revision string   `json:"revision"`
	}
	if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &got); err != nil {
		t.Fatal(err)
	}
	if got.Revision == "" || len(got.Paths) != 1 || got.Paths[0] != "main.lua" {
		t.Fatalf("reindex = %+v", got)
	}
	if _, err := s.reindex(context.Background(), highLevelRequest(t, map[string]any{"paths": []string{"../outside.lua"}})); err == nil {
		t.Fatal("reindex accepted path outside workspace")
	}
}

func TestWorkspaceStatusIsDeterministicStructuredSummary(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"z.lua", "a.lua"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("return 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	result, err := s.workspaceStatus(context.Background(), highLevelRequest(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		DocumentCount int      `json:"documentCount"`
		Documents     []string `json:"documents"`
	}
	structured, ok := result.StructuredContent.(json.RawMessage)
	if !ok {
		t.Fatalf("status did not return structured JSON: %#v", result.StructuredContent)
	}
	if err := json.Unmarshal(structured, &got); err != nil {
		t.Fatal(err)
	}
	if got.DocumentCount != len(got.Documents) || len(got.Documents) < 2 || got.Documents[0] != "a.lua" || got.Documents[len(got.Documents)-1] != "z.lua" {
		t.Fatalf("status = %+v, want sorted relative documents", got)
	}
	for _, document := range got.Documents {
		if filepath.IsAbs(document) {
			t.Fatalf("document %q is not relative", document)
		}
	}
}
