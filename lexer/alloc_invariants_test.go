package lexer_test

import (
	"testing"

	"github.com/FRFlo/lugo/lexer"
	"github.com/FRFlo/lugo/token"
)

func TestLexerSteadyStateAllocations(t *testing.T) {
	// Same workload as BenchmarkLexer; the source and lexer stay alive across runs.
	src := []byte(`
		local function factorial(n)
			if n == 0 then return 1 end
			return n * factorial(n - 1)
		end
		print(factorial(5))
	`)
	l := lexer.New(src)
	scan := func() {
		l.Reset(src)
		for l.Next().Kind != token.EOF {
		}
	}
	scan() // Prime any lazy state before measuring steady-state behavior.
	if got := testing.AllocsPerRun(100, scan); got != 0 {
		t.Fatalf("lexer allocations per scan = %g, want 0", got)
	}
}

func TestLexerNewAllocations(t *testing.T) {
	sources := [][]byte{
		[]byte("local value = 1"),
		[]byte("\uFEFFlocal value = 1"),
	}

	for _, source := range sources {
		scan := func() {
			l := lexer.New(source)
			for l.Next().Kind != token.EOF {
			}
		}

		scan() // Prime any lazy state before measuring allocations.
		if got := testing.AllocsPerRun(100, scan); got != 0 {
			t.Fatalf("lexer allocations per new scan = %g, want 0 (source %q)", got, source)
		}
	}
}
