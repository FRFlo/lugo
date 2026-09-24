package token_test

import (
	"bytes"
	"testing"

	"github.com/FRFlo/lugo/token"
)

func TestTokenSetBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []token.Kind
		present []token.Kind
		absent  []token.Kind
	}{
		{"empty", nil, nil, []token.Kind{0, 63, 64, 127, 128, 255}},
		{"word edges", []token.Kind{0, 63, 64, 127}, []token.Kind{0, 63, 64, 127}, []token.Kind{1, 62, 65, 126, 128, 255}},
		{"out of range ignored", []token.Kind{128, 255, 64, 64}, []token.Kind{64}, []token.Kind{63, 65, 127, 128, 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := token.NewTokenSet(tc.members...)
			for _, kind := range tc.present {
				if !set.Contains(kind) {
					t.Errorf("set missing kind %d", kind)
				}
			}
			for _, kind := range tc.absent {
				if set.Contains(kind) {
					t.Errorf("set unexpectedly contains kind %d", kind)
				}
			}
		})
	}
}

func TestTokenTextByteBoundaries(t *testing.T) {
	source := []byte("a🙂\nb")
	for _, tc := range []struct {
		name       string
		start, end uint32
		want       []byte
	}{
		{"empty start", 0, 0, []byte{}},
		{"ascii", 0, 1, []byte("a")},
		{"multibyte", 1, 5, []byte("🙂")},
		{"last byte", 6, 7, []byte("b")},
		{"empty eof", 7, 7, []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := (token.Token{Start: tc.start, End: tc.end}).Text(source)
			if !bytes.Equal(got, tc.want) {
				t.Errorf("Text = %q, want %q", got, tc.want)
			}
		})
	}
}
