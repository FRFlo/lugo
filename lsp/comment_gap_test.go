package lsp

import (
	"bytes"
	"testing"
)

func TestHasAtMostOneNewlineMatchesCount(t *testing.T) {
	const alphabet = "\n\r x"
	for length := 0; length <= 8; length++ {
		combinations := 1
		for range length {
			combinations *= len(alphabet)
		}
		for n := 0; n < combinations; n++ {
			gap := make([]byte, length)
			value := n
			for i := range gap {
				gap[i] = alphabet[value%len(alphabet)]
				value /= len(alphabet)
			}
			want := bytes.Count(gap, []byte{'\n'}) <= 1
			if got := hasAtMostOneNewline(gap); got != want {
				t.Fatalf("gap %q: got %v, want %v", gap, got, want)
			}
		}
	}

	for _, gap := range [][]byte{
		[]byte("\r\n"), []byte("first\r\nsecond"),
		bytes.Repeat([]byte{'x'}, 1<<20),
		append(bytes.Repeat([]byte{'x'}, 1<<20), '\n', '\n'),
	} {
		want := bytes.Count(gap, []byte{'\n'}) <= 1
		if got := hasAtMostOneNewline(gap); got != want {
			t.Fatalf("large gap length %d: got %v, want %v", len(gap), got, want)
		}
	}
}

// Run with -bench BenchmarkCommentGapAB -benchmem to compare the previous
// full count against the bounded search on identical representative gaps.
func BenchmarkCommentGapAB(b *testing.B) {
	short := []byte("\r\n")
	long := append(bytes.Repeat([]byte("source;\n"), 4096), '\n', '\n')
	for _, tc := range []struct {
		name string
		gap  []byte
	}{{"short", short}, {"long_two_newlines_near_start", long}} {
		b.Run(tc.name+"/count", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = bytes.Count(tc.gap, []byte{'\n'}) <= 1
			}
		})
		b.Run(tc.name+"/bounded_search", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = hasAtMostOneNewline(tc.gap)
			}
		})
	}
}
