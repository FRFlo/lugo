package lsp

import "testing"

func TestIgnoreGlobsCompareAsSets(t *testing.T) {
	s := NewServer("test")
	for _, tc := range []struct {
		name     string
		patterns []string
		changed  bool
	}{
		{"initial", []string{"vendor/**", "*.generated.lua"}, true},
		{"reordered", []string{"*.generated.lua", "vendor/**"}, false},
		{"duplicates", []string{"vendor/**", "vendor/**", "*.generated.lua"}, false},
		{"removed", []string{"vendor/**"}, true},
		{"cleared", nil, true},
		{"empty", []string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.setIgnoreGlobs(tc.patterns); got != tc.changed {
				t.Fatalf("setIgnoreGlobs(%v) = %v, want %v", tc.patterns, got, tc.changed)
			}
		})
	}
}
