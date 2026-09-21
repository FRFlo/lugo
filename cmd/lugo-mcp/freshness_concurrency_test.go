package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentReindexStatusAndDiagnostics(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.lua"), []byte("local value = 1\nreturn value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := newTestWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newServer(workspace, root)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for range 8 {
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
				if _, err := s.workspaceStatus(context.Background(), highLevelRequest(t, map[string]any{})); err != nil {
					errs <- err
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 3 {
				if _, err := s.diagnostics(context.Background(), highLevelRequest(t, map[string]any{"path": "main.lua"})); err != nil {
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
}
