package lsp

import (
	"bytes"
	"reflect"
	"testing"
)

func TestNUIJSNamesUnicodeCaseFold(t *testing.T) {
	src := []byte(`GetParentReſourceName()/foo"`)
	m := nuiCallbackJSRE.FindSubmatchIndex(src)
	if m == nil {
		t.Fatal("fixture must match the case-insensitive regexp")
	}
	want := []nuiContractName{{name: string(src[m[2]:m[3]]), offset: m[2]}}
	if got := nuiJSNames(src, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("nuiJSNames() = %#v, want %#v", got, want)
	}
}

func TestNUIJSNamesPrefilterMatchesRegexResults(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "ordinary asset", src: `const title = "hello";`},
		{name: "resource callback", src: `fetch(GetParentResourceName() + "/save_item");`},
		{name: "mixed case callback", src: `fetch(gEtPaReNtReSoUrCeNaMe() + "/save-item");`},
		{name: "http callback", src: `$.post("HtTpS://example.test/resource/save-item", {});`},
		{name: "event message", src: `if (event.data.action === "open") {}`},
		{name: "short event message", src: `if (E.Data.Type == "close") {}`},
		{name: "near misses", src: `fetch("/save_item"); const x = event.detail.action;`},
		{name: "html inline handlers", src: `<script>fetch(GetParentResourceName()+"/foo")</script>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte(tt.src)
			var want []nuiContractName
			for _, m := range nuiCallbackJSRE.FindAllSubmatchIndex(src, -1) {
				want = append(want, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
			}
			for _, m := range nuiMessageJSRE.FindAllSubmatchIndex(src, -1) {
				want = append(want, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
			}
			if got := nuiJSNames(src, tt.name == "html inline handlers"); !reflect.DeepEqual(got, want) {
				t.Fatalf("nuiJSNames() = %#v, want regexp results %#v", got, want)
			}
		})
	}
}

func TestNUIContainsFolded(t *testing.T) {
	for _, tt := range []struct {
		src, needle string
		want        bool
	}{
		{src: "GetParentResourceName", needle: "getparentresourcename", want: true},
		{src: "HtTp", needle: "http", want: true},
		{src: "event.data.action", needle: "e.data.", want: false},
		{src: "abc", needle: "", want: false},
	} {
		if got := nuiContainsFolded([]byte(tt.src), tt.needle); got != tt.want {
			t.Errorf("nuiContainsFolded(%q, %q) = %t, want %t", tt.src, tt.needle, got, tt.want)
		}
	}
}

func TestNUILuaLiteralGuardsPreserveRegexMatches(t *testing.T) {
	for _, src := range [][]byte{
		[]byte(`RegisterNUICallback("save", function() end)`),
		[]byte(`SendNUIMessage({ action = "open" })`),
		[]byte(`SendNUIMessage({ type = "close" })`),
		[]byte(`-- RegisterNUICallback is mentioned in a comment`),
		[]byte(`local unrelated = "nothing to scan"`),
	} {
		for _, tt := range []struct {
			marker string
			re     interface {
				FindAllSubmatchIndex([]byte, int) [][]int
			}
		}{
			{marker: "RegisterNUICallback", re: nuiCallbackLuaRE},
			{marker: "SendNUIMessage", re: nuiMessageRE},
		} {
			got := tt.re.FindAllSubmatchIndex(src, -1)
			if !bytes.Contains(src, []byte(tt.marker)) {
				got = nil
			}
			want := tt.re.FindAllSubmatchIndex(src, -1)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("guard for %q on %q changed matches: got %v, want %v", tt.marker, src, got, want)
			}
		}
	}
}
