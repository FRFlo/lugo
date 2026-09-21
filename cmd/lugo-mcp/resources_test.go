package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPResourceTemplatesAndURIs(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "client", "main.lua"), []byte("return 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &server{workspace: workspace, root: root}
	impl := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	s.registerResources(impl)

	ctx := context.Background()
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := impl.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.Resources) != 1 || resources.Resources[0].URI != "lugo://workspace/summary" {
		t.Fatalf("unexpected resources: %+v", resources.Resources)
	}
	templates, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates.ResourceTemplates) != 2 {
		t.Fatalf("got %d resource templates, want 2", len(templates.ResourceTemplates))
	}
	want := map[string]bool{
		"lugo://workspace/document/{+path}": false,
		"lugo://workspace/resource/{name}":  false,
	}
	for _, template := range templates.ResourceTemplates {
		if _, ok := want[template.URITemplate]; !ok {
			t.Errorf("unexpected resource template %q", template.URITemplate)
		}
		want[template.URITemplate] = true
	}
	for uri, found := range want {
		if !found {
			t.Errorf("missing resource template %q", uri)
		}
	}

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "lugo://workspace/document/client/main.lua"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 1 || result.Contents[0].Text != "return 42\n" {
		t.Fatalf("unexpected document resource: %+v", result.Contents)
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.lua"), []byte("return 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "lugo://workspace/document/escape/outside.lua"}); err == nil {
		t.Fatal("symlinked document resource unexpectedly succeeded")
	}
}
