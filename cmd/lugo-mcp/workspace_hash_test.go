package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// workspaceSourceHashLegacy is kept as a differential oracle for the exact
// traversal, path encoding, and error behavior that predates streaming reads.
func workspaceSourceHashLegacy(root string) (string, error) {
	var entries []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".lua") {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			entries = append(entries, filepath.ToSlash(rel)+"\x00"+sourceHash(data))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	return sourceHash([]byte(strings.Join(entries, "\n"))), nil
}

func TestWorkspaceSourceHashMatchesLegacy(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{
		"root.lua":            "print('root')\n",
		"nested/UPPER.LUA":    "return 42\n",
		"nested/écriture.lua": "-- utf8 path\n",
		"ignored.txt":         "not lua",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "linked-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "linked-dir", "hidden.lua"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "linked-dir"), filepath.Join(root, "directory-link.lua")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "root.lua"), filepath.Join(root, "file-link.LUA")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	compare := func(label string) {
		t.Helper()
		want, wantErr := workspaceSourceHashLegacy(root)
		got, gotErr := workspaceSourceHash(root)
		if (wantErr == nil) != (gotErr == nil) || (wantErr != nil && wantErr.Error() != gotErr.Error()) {
			t.Fatalf("%s error mismatch: legacy=%v current=%v", label, wantErr, gotErr)
		}
		if got != want {
			t.Fatalf("%s hash mismatch: legacy=%s current=%s", label, want, got)
		}
	}
	compare("initial tree")

	// A same-length content edit with the original mtime must still be seen;
	// source hashes are intentionally never served from a stat-based cache.
	path := filepath.Join(root, "nested", "UPPER.LUA")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("return 43\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	compare("same size and mtime content edit")
}

func TestWorkspaceSourceHashMatchesLegacyErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	want, wantErr := workspaceSourceHashLegacy(missing)
	got, gotErr := workspaceSourceHash(missing)
	if want != got || !sameHashError(wantErr, gotErr) {
		t.Fatalf("missing root mismatch: legacy (%q, %v), current (%q, %v)", want, wantErr, got, gotErr)
	}
	root := t.TempDir()
	broken := filepath.Join(root, "broken.lua")
	if err := os.Symlink(filepath.Join(root, "absent.lua"), broken); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	want, wantErr = workspaceSourceHashLegacy(root)
	got, gotErr = workspaceSourceHash(root)
	if want != got || !sameHashError(wantErr, gotErr) {
		t.Fatalf("broken symlink mismatch: legacy (%q, %v), current (%q, %v)", want, wantErr, got, gotErr)
	}
}

func sameHashError(a, b error) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Error() == b.Error()
}

func BenchmarkWorkspaceSourceHashCorpus(b *testing.B) {
	root := os.Getenv("LUGO_BENCH_CORPUS")
	if root == "" {
		b.Skip("set LUGO_BENCH_CORPUS to a Lua workspace")
	}
	for _, tc := range []struct {
		name string
		fn   func(string) (string, error)
	}{{"Legacy", workspaceSourceHashLegacy}, {"Streaming", workspaceSourceHash}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := tc.fn(root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
