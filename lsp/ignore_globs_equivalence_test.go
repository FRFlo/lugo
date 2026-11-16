package lsp

import (
	"path/filepath"
	"testing"
)

func TestIgnoreGlobsCompareCompiledSemantics(t *testing.T) {
	s := NewServer("test")
	for _, tc := range []struct {
		name    string
		globs   []string
		changed bool
	}{
		{"empty", []string{"", "**/"}, false},
		{"add", []string{"vendor"}, true},
		{"equivalent prefixes and suffixes", []string{"**/vendor/**", "vendor", ""}, false},
		{"distinct fallback", []string{"a*b"}, true},
		{"fallback preserves original pattern", []string{"**/a*b"}, true},
		{"remove", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.setIgnoreGlobs(tc.globs); got != tc.changed {
				t.Fatalf("changed = %v, want %v", got, tc.changed)
			}
		})
	}
	if s.setIgnoreGlobs([]string{""}) {
		t.Fatal("empty pattern must be ignored")
	}
	s.setIgnoreGlobs([]string{"vendor/**"})
	if !s.isIgnored(filepath.Join("root", "vendor", "x.lua"), "x.lua") {
		t.Fatal("equivalent pattern must still ignore vendor")
	}
	if s.isIgnored(filepath.Join("root", "other.lua"), "other.lua") {
		t.Fatal("unrelated file must not be ignored")
	}
}
