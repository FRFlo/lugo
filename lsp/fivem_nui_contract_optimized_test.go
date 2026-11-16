package lsp

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var nuiCallbackJSCombinedReferenceRE = regexp.MustCompile(`(?i)(?:GetParentResourceName\(\)|\$\{\s*GetParentResourceName\(\)\s*\}|['"]https?://[^'"]+)/(?:[^/"'` + "`" + `]*?/)?([A-Za-z_][A-Za-z0-9_:-]*)['"` + "`" + `]`)

func TestNUIJSNamesSplitCallbackRegexMatchesCombinedReference(t *testing.T) {
	tests := []string{
		`"https://host/${GetParentResourceName()}/foo"`,
		"fetch(`https://${GetParentResourceName()}/one`);",
		"fetch(`https://${  GetParentResourceName()  }/two`);",
		`fetch(GetParentResourceName()+"/three"); $.post("https://host/resource/four", {});`,
		"fetch(`https://${GetParentReſourceName()}/five`);",
		"fetch(`https://${getName()}/nope`); fetch(`https://${GetParentResourceName()}/${dynamic}`);",
		"fetch(`https://${GetParentResourceName()}/a`); fetch(GetParentResourceName()+`/b`);",
	}
	for _, source := range tests {
		src := []byte(source)
		var want []nuiContractName
		for _, m := range nuiCallbackJSCombinedReferenceRE.FindAllSubmatchIndex(src, -1) {
			want = append(want, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
		}
		for _, m := range nuiMessageJSRE.FindAllSubmatchIndex(src, -1) {
			want = append(want, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
		}
		if got := nuiJSNames(src, false); !reflect.DeepEqual(got, want) {
			t.Errorf("nuiJSNames(%q) = %#v; combined reference = %#v", source, got, want)
		}
	}
}

func TestNUIJSNamesCandidateScanMatchesRegexOnAdversarialInputs(t *testing.T) {
	atoms := []string{
		`"http://host/`, `'https://a/`, `${`, `GetParentResourceName()`,
		`GetParentReſourceName()`, "}/foo`", `/bar"`, `event.data.type=='ok'`,
		"é😀", "\xff", `'`, ` `, `/`, `e.data.action="x"`, `event.data.action===\"x\"`,
	}
	rng := rand.New(rand.NewSource(19))
	for caseIndex := 0; caseIndex < 20000; caseIndex++ {
		var source strings.Builder
		for part := 0; part < rng.Intn(15); part++ {
			source.WriteString(atoms[rng.Intn(len(atoms))])
		}
		assertNUIJSNamesEqual(t, []byte(source.String()))
	}
	for _, source := range []string{
		"prefix" + strings.Repeat("GetParentRe", 1000) + strings.Repeat("z", 12000),
		"prefix" + strings.Repeat("event.data.", 1000) + strings.Repeat("z", 12000),
		`é😀GetParentReſourceName()/ſave"`,
		`"https://host/${GetParentResourceName()}/foo"`,
		"${ GetParentResourceName() }/foo`",
		"event.data.action===\"é😀\"",
		"\xffGEtParentResourceName()/ab\"",
		"GetParentResourceName()/Kelvin\"",
	} {
		assertNUIJSNamesEqual(t, []byte(source))
	}
}

func TestNUIJSNamesCandidateScanMatchesCorpus(t *testing.T) {
	root := os.Getenv("LUGO_NUI_BENCH_CORPUS")
	if root == "" {
		t.Skip("set LUGO_NUI_BENCH_CORPUS to a read-only NUI corpus")
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".html", ".js", ".ts", ".jsx", ".tsx":
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if got, want := nuiJSNames(source, strings.EqualFold(filepath.Ext(path), ".html")), nuiJSNamesRegex(source); !reflect.DeepEqual(got, want) {
				return fmt.Errorf("scanner mismatch in %s: got %#v, want %#v", path, got, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertNUIJSNamesEqual(t *testing.T, source []byte) {
	t.Helper()
	want := nuiJSNamesRegex(source)
	if got := nuiJSNames(source, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("nuiJSNames(%q) = %#v; regex reference = %#v", source, got, want)
	}
}

func BenchmarkNUIJSNamesCorpus(b *testing.B) {
	root := os.Getenv("LUGO_NUI_BENCH_CORPUS")
	if root == "" {
		b.Skip("set LUGO_NUI_BENCH_CORPUS to a read-only Lua/NUI corpus")
	}
	var sources [][]byte
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".html", ".js", ".ts", ".jsx", ".tsx":
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sources = append(sources, src)
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	if len(sources) == 0 {
		b.Fatal("corpus contains no NUI source files")
	}
	b.Run("existing-regex-scanner", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, src := range sources {
				_ = nuiJSNamesRegex(src)
			}
		}
	})
	b.Run("candidate-scan", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, src := range sources {
				_ = nuiJSNames(src, false)
			}
		}
	})
}
