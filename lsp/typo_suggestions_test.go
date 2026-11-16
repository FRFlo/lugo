package lsp

import (
	"math/rand"
	"slices"
	"strconv"
	"testing"
)

func TestTypoSuggestionsMatchesLegacyAcrossGeneratedNames(t *testing.T) {
	idx := NewGlobalIndex()
	res := idx.EnsureResource("app")
	res.Dependencies = []ResourceURI{"dep"}
	idx.EnsureResource("dep")
	inputs := []SymbolName{
		"PlayerName", "PLAYERNAME", "playernane", "PläyerName", "Kelvin", "İstanbul", "ſtring", "kkfoo",
		SymbolName(string([]byte{'b', 'a', 'd', 0xff, 'n', 'a', 'm', 'e'})),
		"12345678", "123456789", "1234567890123456789012345678901234567890123456789012345678901234",
	}
	for i := 0; i < 400; i++ {
		name := SymbolName("PlayerGenerated" + strconv.Itoa(i))
		idx.AddSymbol("app", GlobalIndexScopeClient, name, &SymbolEntry{})
		if i%5 == 0 {
			inputs = append(inputs, name)
		}
	}
	for _, name := range []SymbolName{"PlayerNmae", "playername", "PläyerName", "Kelvin", "İstanbul", "ſtring", "KKfoo", "PlayerNmae"} {
		idx.AddSymbol("app", GlobalIndexScopeClient, name, &SymbolEntry{})
		idx.AddSymbol("dep", GlobalIndexScopeShared, name, &SymbolEntry{})
	}
	for _, query := range inputs {
		for _, scope := range []GlobalIndexScope{GlobalIndexScopeClient, GlobalIndexScopeServer, GlobalIndexScopeShared} {
			for _, max := range []int{0, 1, 2, 10, 100} {
				got := idx.TypoSuggestions(query, "app", scope, max)
				want := legacyTypoSuggestions(idx, query, "app", scope, max)
				if !slices.Equal(got, want) {
					t.Fatalf("query=%q scope=%q max=%d got=%#v want=%#v", query, scope, max, got, want)
				}
			}
		}
	}
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 200; n++ {
		buf := make([]byte, rng.Intn(30))
		for i := range buf {
			buf[i] = byte(rng.Intn(256))
		}
		query := SymbolName(string(buf))
		got := idx.TypoSuggestions(query, "app", GlobalIndexScopeClient, 25)
		want := legacyTypoSuggestions(idx, query, "app", GlobalIndexScopeClient, 25)
		if !slices.Equal(got, want) {
			t.Fatalf("random query=%q got=%#v want=%#v", query, got, want)
		}
	}
}
