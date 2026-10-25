package lexer_test

import (
	"testing"

	"github.com/FRFlo/lugo/lexer"
	"github.com/FRFlo/lugo/token"
)

func FuzzLexerTokenOffsets(f *testing.F) {
	for _, seed := range [][]byte{
		{},
		[]byte("\uFEFFlocal value = 1"),
		[]byte("function f(...) return {x = ...} end"),
		[]byte("--[=[ unterminated"),
		{0x00, 0xEF, 0xBB, 0xBF},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source []byte) {
		l := lexer.New(source)
		var previousEnd uint32
		for steps := 0; steps <= len(source)+1; steps++ {
			tok := l.Next()
			if tok.Start > tok.End || int(tok.End) > len(source) {
				t.Fatalf("invalid token bounds: %+v for %d bytes", tok, len(source))
			}
			if tok.Start < previousEnd {
				t.Fatalf("token offsets moved backwards: %+v after %d", tok, previousEnd)
			}
			if tok.Kind == token.EOF {
				return
			}
			if tok.End <= previousEnd {
				t.Fatalf("lexer did not advance: %+v after %d", tok, previousEnd)
			}
			previousEnd = tok.End
		}
		t.Fatalf("lexer did not reach EOF within input-sized steps")
	})
}
