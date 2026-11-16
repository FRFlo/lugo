package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
)

func BenchmarkWorkspaceParseTreeOwnership(b *testing.B) {
	var source strings.Builder
	for i := range 500 {
		fmt.Fprintf(&source, "local value%d = { key = %d, nested = { flag = true } }\n", i, i)
	}
	src := []byte(source.String())

	b.Run("reuse-private-tree", func(b *testing.B) {
		tree := ast.NewTree(src)
		p := parser.New(src, tree, 0)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			tree.Reset(src)
			p.Reset(src, tree)
			p.Parse()
		}
	})

	b.Run("copy-on-write-tree", func(b *testing.B) {
		p := parser.New(nil, ast.NewTree(nil), 0)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			tree := ast.NewTree(src)
			p.Reset(src, tree)
			p.Parse()
		}
	})
}

func BenchmarkWorkspaceRefreshReindexChangedDocument(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	var source strings.Builder
	for i := range 500 {
		fmt.Fprintf(&source, "local value%d = { key = %d, nested = { flag = true } }\n", i, i)
	}
	h.writeWorkspaceFile("index.lua", source.String()+"-- generation 0\n")
	h.reindex()

	path := filepath.Join(h.root, "index.lua")
	modTime := time.Now().Add(time.Hour)
	b.ReportAllocs()
	b.ResetTimer()
	for generation := 1; b.Loop(); generation++ {
		content := source.String() + fmt.Sprintf("-- generation %d\n", generation%2)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
		modTime = modTime.Add(time.Second)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			b.Fatal(err)
		}
		h.reindex()
	}
}
