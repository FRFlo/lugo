package corpusbenchmark

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
)

func TestCorpusParserReuseMatchesCold(t *testing.T) {
	root := os.Getenv("LUGO_BENCH_CORPUS")
	if root == "" {
		t.Skip("set LUGO_BENCH_CORPUS to a Lua workspace")
	}
	warmTree := ast.NewTree(nil)
	warmParser := parser.New(nil, warmTree, 0)
	files := 0
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
		coldTree := ast.NewTree(src)
		coldParser := parser.New(src, coldTree, 0)
		coldParser.Parse()
		warmTree.Reset(src)
		warmParser.Reset(src, warmTree)
		warmParser.Parse()
		files++
		if coldTree.Root != warmTree.Root || !reflect.DeepEqual(coldTree.Nodes, warmTree.Nodes) || !reflect.DeepEqual(coldTree.ExtraList, warmTree.ExtraList) || !reflect.DeepEqual(coldTree.Comments, warmTree.Comments) || !reflect.DeepEqual(coldTree.LineOffsets, warmTree.LineOffsets) || !reflect.DeepEqual(coldParser.Errors, warmParser.Errors) {
			t.Errorf("cold and reused parse differ for %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no Lua files found")
	}
	t.Logf("compared %d Lua files", files)
}
