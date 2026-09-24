package main

import (
	"strings"
	"testing"
)

func TestCheckBenchmarks(t *testing.T) {
	const lexer = "BenchmarkLexer-12 100 480 ns/op 0 B/op 0 allocs/op\n"
	const parser = "BenchmarkParser-12 100 1781 ns/op 0 B/op 0 allocs/op\n"
	for _, tc := range []struct {
		name, output, wantError string
	}{
		{"zero allocations", lexer + parser, ""},
		{"lexer allocates", strings.Replace(lexer, "0 allocs/op", "1 allocs/op", 1) + parser, "BenchmarkLexer-: 1 allocs/op"},
		{"parser allocates", lexer + strings.Replace(parser, "0 allocs/op", "2 allocs/op", 1), "BenchmarkParser-: 2 allocs/op"},
		{"missing lexer", parser, "BenchmarkLexer was not reported"},
		{"missing parser", lexer, "BenchmarkParser was not reported"},
		{"missing allocs field", lexer + "BenchmarkParser-12 100 1781 ns/op\n", "missing allocs/op result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkBenchmarks(tc.output)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
}
