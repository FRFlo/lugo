// Package corpusbenchmark measures core stages on a user-supplied Lua corpus.
package corpusbenchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/lexer"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
	"github.com/FRFlo/lugo/token"
)

func loadCorpus(b *testing.B) ([][]byte, int64) {
	b.Helper()
	root := os.Getenv("LUGO_BENCH_CORPUS")
	if root == "" {
		b.Skip("set LUGO_BENCH_CORPUS to a Lua workspace")
	}
	var files [][]byte
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".lua") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, src)
		total += int64(len(src))
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	if len(files) == 0 {
		b.Fatal("no Lua files found")
	}
	b.Logf("%d Lua files, %d bytes", len(files), total)
	return files, total
}

func BenchmarkCorpusLexer(b *testing.B) {
	files, total := loadCorpus(b)
	b.ReportAllocs()
	b.SetBytes(total)
	b.ResetTimer()
	for b.Loop() {
		for _, src := range files {
			lex := lexer.New(src)
			for lex.Next().Kind != token.EOF {
			}
		}
	}
}

func BenchmarkCorpusParserCold(b *testing.B) {
	files, total := loadCorpus(b)
	b.ReportAllocs()
	b.SetBytes(total)
	b.ResetTimer()
	for b.Loop() {
		for _, src := range files {
			tree := ast.NewTree(src)
			parser.New(src, tree, 0).Parse()
		}
	}
}

func BenchmarkCorpusParserWarm(b *testing.B) {
	files, total := loadCorpus(b)
	tree := ast.NewTree(nil)
	p := parser.New(nil, tree, 0)
	b.ReportAllocs()
	b.SetBytes(total)
	b.ResetTimer()
	for b.Loop() {
		for _, src := range files {
			tree.Reset(src)
			p.Reset(src, tree)
			p.Parse()
		}
	}
}

func BenchmarkCorpusResolverCold(b *testing.B) {
	files, total := loadCorpus(b)
	trees := make([]*ast.Tree, len(files))
	for i, src := range files {
		trees[i] = ast.NewTree(src)
		parser.New(src, trees[i], 0).Parse()
	}
	b.ReportAllocs()
	b.SetBytes(total)
	b.ResetTimer()
	for b.Loop() {
		for _, tree := range trees {
			semantic.New(tree).Resolve(tree.Root)
		}
	}
}
