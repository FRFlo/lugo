package lsp

import (
	"hash/fnv"
	"slices"

	"github.com/coalaura/lugo/ast"
)

// TreeDiffResult holds the sets of nodes that were added, removed, or modified between two ASTs.
type TreeDiffResult struct {
	Added    []ast.NodeID
	Removed  []ast.NodeID
	Modified []ast.NodeID
}

// TreeDiffer provides methods to efficiently compare two AST trees and identify structural changes.
type TreeDiffer struct{}

type treeDiffEntry struct {
	id   ast.NodeID
	node ast.Node
}

// Diff performs a structural comparison between oldTree and newTree, returning the differences.
func (d TreeDiffer) Diff(oldTree, newTree *ast.Tree) TreeDiffResult {
	oldBuckets := buildTreeDiffBuckets(oldTree)
	newBuckets := buildTreeDiffBuckets(newTree)

	result := TreeDiffResult{}
	added := make(map[ast.NodeID]struct{})
	removed := make(map[ast.NodeID]struct{})
	modified := make(map[ast.NodeID]struct{})
	seen := make(map[uint64]struct{}, len(oldBuckets)+len(newBuckets))

	for hash := range oldBuckets {
		seen[hash] = struct{}{}
	}
	for hash := range newBuckets {
		seen[hash] = struct{}{}
	}

	for hash := range seen {
		oldEntries := oldBuckets[hash]
		newEntries := newBuckets[hash]

		limit := min(len(oldEntries), len(newEntries))

		for i := range limit {
			oldEntry := oldEntries[i]
			newEntry := newEntries[i]

			if oldEntry.id != newEntry.id {
				modified[oldEntry.id] = struct{}{}
				modified[newEntry.id] = struct{}{}
				continue
			}

			if oldEntry.node.Parent != newEntry.node.Parent || oldEntry.node.Left != newEntry.node.Left || oldEntry.node.Right != newEntry.node.Right {
				modified[oldEntry.id] = struct{}{}
			}
		}

		if len(oldEntries) > limit {
			for _, entry := range oldEntries[limit:] {
				removed[entry.id] = struct{}{}
			}
		}

		if len(newEntries) > limit {
			for _, entry := range newEntries[limit:] {
				added[entry.id] = struct{}{}
			}
		}
	}

	result.Added = sortedNodeIDs(added)
	result.Removed = sortedNodeIDs(removed)
	result.Modified = sortedNodeIDs(modified)

	return result
}

func buildTreeDiffBuckets(tree *ast.Tree) map[uint64][]treeDiffEntry {
	buckets := make(map[uint64][]treeDiffEntry)
	if tree == nil {
		return buckets
	}

	for i := 1; i < len(tree.Nodes); i++ {
		node := tree.Nodes[i]
		hash := hashTreeNode(tree, node)
		buckets[hash] = append(buckets[hash], treeDiffEntry{id: ast.NodeID(i), node: node})
	}

	return buckets
}

func hashTreeNode(tree *ast.Tree, node ast.Node) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte{byte(node.Kind)})

	if tree != nil && node.Start <= node.End && int(node.End) <= len(tree.Source) {
		_, _ = h.Write(tree.Source[node.Start:node.End])
	}

	return h.Sum64()
}

func sortedNodeIDs(ids map[ast.NodeID]struct{}) []ast.NodeID {
	if len(ids) == 0 {
		return nil
	}

	out := make([]ast.NodeID, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}

	slices.Sort(out)
	return out
}
