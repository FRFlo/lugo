package lsp

import "testing"

func FuzzFiveMAssetMatchGlob(f *testing.F) {
	for _, seed := range [][2]string{
		{"", ""},
		{"*.lua", "init.lua"},
		{"*.lua", "dir/init.lua"},
		{"**/*.lua", "dir/init.lua"},
		{"*a*a*a*b", "aaaac"},
		{"?", "/"},
	} {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, pattern, candidate string) {
		// The recursive reference is intentionally restricted to keep fuzzing
		// bounded while checking the optimized matcher's byte-level semantics.
		if len(pattern) > 16 {
			pattern = pattern[:16]
		}
		if len(candidate) > 16 {
			candidate = candidate[:16]
		}

		got := fiveMAssetMatchGlob(pattern, candidate)
		want := fiveMAssetMatchGlobReference(pattern, candidate)
		if got != want {
			t.Fatalf("fiveMAssetMatchGlob(%q, %q) = %t, want %t", pattern, candidate, got, want)
		}
	})
}

func fiveMAssetMatchGlobReference(pattern, candidate string) bool {
	if pattern == "" {
		return candidate == ""
	}
	if pattern[0] == '*' {
		if len(pattern) > 1 && pattern[1] == '*' {
			for i := 0; i <= len(candidate); i++ {
				if fiveMAssetMatchGlobReference(pattern[2:], candidate[i:]) {
					return true
				}
			}
			return false
		}
		for i := 0; i <= len(candidate) && (i == 0 || candidate[i-1] != '/'); i++ {
			if fiveMAssetMatchGlobReference(pattern[1:], candidate[i:]) {
				return true
			}
		}
		return false
	}
	if pattern[0] == '?' {
		return len(candidate) > 0 && candidate[0] != '/' && fiveMAssetMatchGlobReference(pattern[1:], candidate[1:])
	}
	return len(candidate) > 0 && pattern[0] == candidate[0] && fiveMAssetMatchGlobReference(pattern[1:], candidate[1:])
}
