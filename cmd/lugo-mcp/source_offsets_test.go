package main

import (
	"testing"
	"unicode/utf8"

	"github.com/coalaura/lugo/lsp"
)

func TestSourceOffsetsUTF16(t *testing.T) {
	source := []byte("a😀中\nb\n")
	positions := []lsp.Position{
		{Line: 0, Character: 0}, {Line: 0, Character: 1}, {Line: 0, Character: 3},
		{Line: 0, Character: 4}, {Line: 1, Character: 1}, {Line: 2, Character: 0},
	}
	got, err := sourceOffsets(source, positions)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 1, 5, 8, 10, len(source)}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %v: offset = %d, want %d", positions[i], got[i], want[i])
		}
	}
	if _, err := sourceOffsets(source, []lsp.Position{{Line: 0, Character: 2}}); err == nil {
		t.Fatal("accepted a UTF-16 position inside a surrogate pair")
	}
}

func FuzzSourceOffsetsUTF16(f *testing.F) {
	f.Add("a😀中\nb")
	f.Add("")
	f.Fuzz(func(t *testing.T, source string) {
		if !utf8.ValidString(source) {
			t.Skip()
		}
		data := []byte(source)
		for line, offsets := range utf16LineOffsets(data) {
			for character, want := range offsets {
				got, err := sourceOffsets(data, []lsp.Position{{Line: uint32(line), Character: uint32(character)}})
				if err != nil || len(got) != 1 || got[0] != want {
					t.Fatalf("line %d character %d: offsets = %v, err = %v, want %d", line, character, got, err, want)
				}
			}
		}
	})
}

// utf16LineOffsets is an independent reference model: each entry maps a valid
// UTF-16 character position on a line to its byte offset.
func utf16LineOffsets(source []byte) []map[int]int {
	lines := []map[int]int{{0: 0}}
	line, character := 0, 0
	for offset := 0; offset < len(source); {
		if source[offset] == '\n' {
			offset++
			lines = append(lines, map[int]int{0: offset})
			line, character = line+1, 0
			continue
		}
		r, size := utf8.DecodeRune(source[offset:])
		character++
		if r > 0xffff {
			character++
		}
		offset += size
		lines[line][character] = offset
	}
	return lines
}
