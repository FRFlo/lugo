package lsp

import (
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
)

func TestEvalLuaStringLiterals(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "single quote multiple characters", src: `'multi'`, want: "multi"},
		{name: "hex escape", src: `"\x41\x42"`, want: "AB"},
		{name: "decimal escape", src: `"\065\066"`, want: "AB"},
		{name: "z whitespace escape", src: "\"a\\z \t\tb\"", want: "ab"},
		{name: "long string", src: "[[\nhello\nworld]]", want: "hello\nworld"},
		{name: "long string equals", src: "[==[hello ]=] world]==]", want: "hello ]=] world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := newIndexTestDocument(t, "local value = "+tt.src)
			for id, node := range doc.Tree.Nodes {
				if node.Kind != ast.KindString {
					continue
				}
				got, ok := doc.evalNode(ast.NodeID(id), 0)
				if !ok {
					t.Fatal("string literal was not evaluable")
				}
				if got.str != tt.want {
					t.Fatalf("decoded string = %q, want %q", got.str, tt.want)
				}
				return
			}
			t.Fatal("string literal AST node not found")
		})
	}
}

func TestNewIndexWriteIndexUsesDecodedStringKeys(t *testing.T) {
	doc := newIndexTestDocument(t, `local table = {}
table["\065\066"] = 1
table[ [[AB]] ] = 2`)
	index := buildNewIndexWriteIndexFromSlice([]*Document{doc})
	if got := len(index["AB"]); got != 2 {
		t.Fatalf("writes under decoded key AB = %d, want 2", got)
	}
}

func TestEvalLuaStringUnicodeEscape(t *testing.T) {
	got, ok := decodeLuaString(`"\u{E9}"`)
	if !ok || got != "é" {
		t.Fatalf("decoded unicode escape = %q, %v; want é", got, ok)
	}
}

func TestEvalLuaUnicodeEscapeBounds(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
		ok        bool
	}{
		{`"\u{0}"`, "\x00", true},
		{`"\u{7fffffff}"`, "\xfd\xbf\xbf\xbf\xbf\xbf", true},
		{`"\u{80000000}"`, "", false},
		{`"\u{}"`, "", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, ok := decodeLuaString(tc.raw)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("decode %s = %q, %v; want %q, %v", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestEvalLuaStringNormalizesPairedNewlines(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"long lfcr", "[[a\n\rb]]", "a\nb"},
		{"long crlfcr", "[[a\r\n\rb]]", "a\n\nb"},
		{"escaped lfcr", "\"a\\\n\rb\"", "a\nb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decodeLuaString(tc.raw)
			if !ok || got != tc.want {
				t.Fatalf("decode = %q, %v; want %q", got, ok, tc.want)
			}
		})
	}
}

func TestDecodeLuaPlainStringDoesNotAllocate(t *testing.T) {
	raw := `"ordinary_field_name"`
	allocations := testing.AllocsPerRun(100, func() {
		decoded, ok := decodeLuaString(raw)
		if !ok || decoded != "ordinary_field_name" {
			t.Fatal("incorrect decoded literal")
		}
	})
	if allocations != 0 {
		t.Fatalf("plain string decoding allocated %.1f times, want zero", allocations)
	}
}

func BenchmarkDecodeLuaString(b *testing.B) {
	for _, literal := range []string{`"ordinary_field_name"`, `"escaped_\x61"`} {
		b.Run(literal, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = decodeLuaString(literal)
			}
		})
	}
}

func BenchmarkDecodeLuaStringDiscardedWhitespace(b *testing.B) {
	literal := `"\z` + strings.Repeat(" ", 8192) + `x"`
	b.ReportAllocs()
	for b.Loop() {
		_, _ = decodeLuaString(literal)
	}
}

func TestDecodeLuaDiscardedWhitespaceDoesNotReserveInputSize(t *testing.T) {
	result := testing.Benchmark(BenchmarkDecodeLuaStringDiscardedWhitespace)
	if got := result.AllocedBytesPerOp(); got > 128 {
		t.Fatalf("decoding discarded whitespace allocated %d B/op, want at most 128", got)
	}
}

func benchmarkDecodeLuaStringDiscardedWhitespace(b *testing.B) {
	literal := `"\z` + strings.Repeat(" ", 8192) + `x"`
	b.ResetTimer()
	for b.Loop() {
		_, _ = decodeLuaString(literal)
	}
}
