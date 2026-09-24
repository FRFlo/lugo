package lsp

import "testing"

func TestBoundedWatchedFileCount(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "zero", in: 0, want: 0},
		{name: "ordinary", in: 14, want: 14},
		{name: "cap", in: 1000, want: 1000},
		{name: "over cap", in: 1001, want: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boundedWatchedFileCount(tt.in); got != tt.want {
				t.Fatalf("boundedWatchedFileCount(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
