package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fileHash(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

func TestValidateWorkspaceEditAcceptsRelativePathAndCurrentHash(t *testing.T) {
	root := t.TempDir()
	source := "return 1\n"
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &server{root: root}
	result, err := s.validateEdits(context.Background(), highLevelRequest(t, map[string]any{
		"edits": []any{map[string]any{"path": "main.lua", "sourceHash": fileHash(source), "edits": []any{map[string]any{
			"range": map[string]any{"start": map[string]any{"line": 0, "character": 7}, "end": map[string]any{"line": 0, "character": 8}}, "newText": "2",
		}}}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &got); err != nil {
		t.Fatal(err)
	}
	if got["valid"] != true {
		t.Fatalf("validation = %#v", got)
	}
	if !strings.Contains(string(result.StructuredContent.(json.RawMessage)), "return 2") {
		t.Fatalf("preview missing edited source: %s", result.StructuredContent)
	}
}

func TestValidateWorkspaceEditRejectsStaleSourceAndOverlaps(t *testing.T) {
	root := t.TempDir()
	source := "return 1\n"
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &server{root: root}
	for name, args := range map[string]map[string]any{
		"stale": {"edits": []any{map[string]any{"path": "main.lua", "sourceHash": "wrong", "edits": []any{}}}},
		"overlap": {"edits": []any{map[string]any{"path": "main.lua", "edits": []any{
			map[string]any{"range": map[string]any{"start": map[string]any{"line": 0, "character": 0}, "end": map[string]any{"line": 0, "character": 5}}, "newText": "x"},
			map[string]any{"range": map[string]any{"start": map[string]any{"line": 0, "character": 4}, "end": map[string]any{"line": 0, "character": 6}}, "newText": "y"},
		}}}},
	} {
		result, err := s.validateEdits(context.Background(), highLevelRequest(t, args))
		if err != nil {
			t.Fatal(name, err)
		}
		var got struct {
			Valid bool `json:"valid"`
		}
		if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &got); err != nil {
			t.Fatal(err)
		}
		if got.Valid {
			t.Fatalf("%s unexpectedly valid", name)
		}
	}
}

func TestValidateWorkspaceEditRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	s := &server{root: root}
	result, err := s.validateEdits(context.Background(), highLevelRequest(t, map[string]any{
		"edits": []any{map[string]any{"path": "../outside.lua", "edits": []any{}}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &got); err != nil {
		t.Fatal(err)
	}
	if got.Valid || len(got.Errors) == 0 || !strings.Contains(got.Errors[0], "workspace") {
		t.Fatalf("result = %+v", got)
	}
}
