package lsp

import (
	"strconv"
	"strings"
	"testing"

	"github.com/coalaura/lugo/ast"
)

const (
	perfColdStartResources = 50
	perfSymbolsPerResource = 20
	perfHoverLookups       = 100
	perfDiffNodes          = 100
)

var perfScopeCycle = []GlobalIndexScope{
	GlobalIndexScopeClient,
	GlobalIndexScopeServer,
	GlobalIndexScopeShared,
}

var perfTypeCycle = []Type{
	{Primitive: TypeNumber},
	{Primitive: TypeString},
	{Primitive: TypeBoolean},
}

type perfSymbolFixture struct {
	Name  SymbolName
	Scope GlobalIndexScope
	Type  Type
}

type perfResourceFixture struct {
	URI     ResourceURI
	Symbols []perfSymbolFixture
}

func BenchmarkColdStart(b *testing.B) {
	plan := buildPerfResourcePlan(perfColdStartResources, perfSymbolsPerResource)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx := NewGlobalIndex()

		for _, res := range plan {
			idx.EnsureResource(res.URI)
			registerPerfResourceSymbols(idx, res)

			tree := buildPerfResolverTree(res)
			resolver := NewResolver(tree, ResolverOptions{
				Index:        idx,
				ResourceURI:  res.URI,
				Scope:        GlobalIndexScopeShared,
				SemanticData: NewSemanticDataTable(),
				FeatureFiveM: false,
			})

			if err := resolver.Resolve(tree.Root); err != nil {
				b.Fatalf("resolver failed for %s: %v", res.URI, err)
			}
		}
	}
}

func BenchmarkPerRequestHover(b *testing.B) {
	idx := NewGlobalIndex()
	resource := ResourceURI("perf://hover-resource")
	lookups := buildPerfResourcePlan(1, perfHoverLookups)[0]
	lookups.URI = resource
	registerPerfResourceSymbols(idx, lookups)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, sym := range lookups.Symbols {
			if got := idx.LookupByScope(resource, sym.Scope, sym.Name); got == nil {
				b.Fatalf("missing symbol %q in scope %s", sym.Name, sym.Scope)
			}
		}
	}
}

func BenchmarkTreeDiff(b *testing.B) {
	oldTree := buildPerfDiffTree(perfDiffNodes, ast.InvalidNode, ast.InvalidNode)
	newTree := buildPerfDiffTree(perfDiffNodes, 50, ast.NodeID(2))
	differ := TreeDiffer{}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result := differ.Diff(oldTree, newTree)
		if len(result.Modified) == 0 {
			b.Fatal("expected at least one modified node")
		}
	}
}

func buildPerfResourcePlan(resourceCount, symbolsPerResource int) []perfResourceFixture {
	plan := make([]perfResourceFixture, 0, resourceCount)
	for resourceIndex := 0; resourceIndex < resourceCount; resourceIndex++ {
		res := perfResourceFixture{
			URI:     ResourceURI("perf://resource/" + strconv.Itoa(resourceIndex)),
			Symbols: make([]perfSymbolFixture, 0, symbolsPerResource),
		}

		for symbolIndex := 0; symbolIndex < symbolsPerResource; symbolIndex++ {
			scope := perfScopeCycle[(resourceIndex+symbolIndex)%len(perfScopeCycle)]
			kind := perfTypeCycle[(resourceIndex+symbolIndex)%len(perfTypeCycle)]
			res.Symbols = append(res.Symbols, perfSymbolFixture{
				Name:  SymbolName("sym_" + strconv.Itoa(resourceIndex) + "_" + strconv.Itoa(symbolIndex)),
				Scope: scope,
				Type:  kind,
			})
		}

		plan = append(plan, res)
	}

	return plan
}

func registerPerfResourceSymbols(idx *GlobalIndex, res perfResourceFixture) {
	for _, sym := range res.Symbols {
		idx.AddSymbol(res.URI, sym.Scope, sym.Name, &SymbolEntry{
			Key:  GlobalKey{PropHash: ast.HashBytes([]byte(sym.Name))},
			Type: sym.Type,
		})
	}
}

func buildPerfResolverTree(res perfResourceFixture) *ast.Tree {
	var source strings.Builder
	source.Grow(len(res.Symbols) * 12)

	positions := make([][2]uint32, len(res.Symbols))
	for i, sym := range res.Symbols {
		if i > 0 {
			source.WriteByte('\n')
		}
		start := uint32(source.Len())
		source.WriteString(string(sym.Name))
		positions[i] = [2]uint32{start, uint32(source.Len())}
	}

	tree := ast.NewTree([]byte(source.String()))
	tree.Nodes = append(tree.Nodes, make([]ast.Node, 2+len(res.Symbols))...)
	tree.Root = 1

	fileID := ast.NodeID(1)
	blockID := ast.NodeID(2)
	tree.Nodes[fileID] = ast.Node{Kind: ast.KindFile, Left: blockID, End: uint32(len(tree.Source))}
	tree.Nodes[blockID] = ast.Node{Kind: ast.KindBlock, Parent: fileID, Count: uint16(len(res.Symbols)), Extra: 0, End: uint32(len(tree.Source))}
	tree.ExtraList = make([]ast.NodeID, len(res.Symbols))

	for i, pos := range positions {
		id := ast.NodeID(3 + i)
		tree.ExtraList[i] = id
		tree.Nodes[id] = ast.Node{
			Kind:   ast.KindIdent,
			Parent: blockID,
			Start:  pos[0],
			End:    pos[1],
		}
	}

	return tree
}

func buildPerfDiffTree(nodeCount int, tweakNode, tweakParent ast.NodeID) *ast.Tree {
	if nodeCount < 2 {
		nodeCount = 2
	}

	var source strings.Builder
	positions := make([][2]uint32, nodeCount)
	for i := 2; i <= nodeCount; i++ {
		if i > 2 {
			source.WriteByte('\n')
		}
		start := uint32(source.Len())
		source.WriteString("node_" + strconv.Itoa(i))
		positions[i-1] = [2]uint32{start, uint32(source.Len())}
	}

	tree := ast.NewTree([]byte(source.String()))
	tree.Nodes = append(tree.Nodes, make([]ast.Node, nodeCount)...)
	tree.Root = 1

	tree.Nodes[1] = ast.Node{Kind: ast.KindFile, Left: 2, End: uint32(len(tree.Source))}
	for i := 2; i <= nodeCount; i++ {
		parent := ast.NodeID(1)
		if ast.NodeID(i) == tweakNode {
			parent = tweakParent
		}
		pos := positions[i-1]
		tree.Nodes[i] = ast.Node{
			Kind:   ast.KindIdent,
			Parent: parent,
			Start:  pos[0],
			End:    pos[1],
		}
	}

	return tree
}
