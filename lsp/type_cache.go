package lsp

import "github.com/FRFlo/lugo/ast"

// typeCache stores only reusable inference results. The uint32 index is
// pointer-free and keeps the large TypeSet values tightly packed when only a
// small fraction of an AST is inferred.
type typeCache struct {
	slots  []uint32
	values []TypeSet
	nodes  int
}

func reusableType(t TypeSet) bool {
	return t.Basics != TypeUnknown || t.CustomName != "" || t.DeclNode != ast.InvalidNode || len(t.Alternatives) != 0
}

func (c *typeCache) Get(id ast.NodeID) TypeSet {
	if c == nil || c.slots == nil || uint64(id) >= uint64(len(c.slots)) {
		return TypeSet{}
	}
	index := c.slots[id]
	if index == 0 || uint64(index-1) >= uint64(len(c.values)) {
		return TypeSet{}
	}
	return c.values[index-1]
}

func (c *typeCache) Put(id ast.NodeID, t TypeSet) {
	if c == nil || !reusableType(t) || uint64(id) >= uint64(c.nodes) {
		return
	}
	if c.slots == nil {
		c.slots = make([]uint32, c.nodes)
	}
	if c.slots[id] == 0 {
		// An AST cannot practically contain enough cached results to overflow
		// this index: doing so would require more than 640 GiB of TypeSets.
		if len(c.values) == cap(c.values) {
			newCap := max(cap(c.values)*2, 8)
			if newCap > c.nodes {
				newCap = c.nodes
			}
			values := make([]TypeSet, len(c.values), newCap)
			copy(values, c.values)
			c.values = values
		}
		c.values = append(c.values, t)
		c.slots[id] = uint32(len(c.values))
		return
	}
	c.values[c.slots[id]-1] = t
}

// Invalidate forgets one cached result while keeping the compact slot map in
// place. A subsequent Put for the node reuses its value slot.
func (c *typeCache) Invalidate(id ast.NodeID) {
	if c == nil || uint64(id) >= uint64(len(c.slots)) {
		return
	}
	index := c.slots[id]
	if index == 0 || uint64(index-1) >= uint64(len(c.values)) {
		return
	}
	c.values[index-1] = TypeSet{}
}

// Reset drops all references from the previous tree and bounds retained
// capacity after large-to-small edits. Values are cleared before reuse because
// TypeSet contains strings, slices, and URI references.
func (c *typeCache) Reset(nodeCount int) {
	if c == nil {
		return
	}
	clear(c.values)
	if nodeCount < 0 {
		nodeCount = 0
	}
	if cap(c.slots) > max(nodeCount*2, 4096) {
		c.slots = nil
	} else if cap(c.slots) >= nodeCount {
		c.slots = c.slots[:nodeCount]
		clear(c.slots)
	} else {
		c.slots = nil
	}
	if cap(c.values) > max(nodeCount*2, 4096) {
		c.values = nil
	} else {
		c.values = c.values[:0]
	}
	c.nodes = nodeCount
}

func (c *typeCache) Drop() {
	if c == nil {
		return
	}
	clear(c.values)
	*c = typeCache{}
}

func (c *typeCache) Len() int    { return len(c.values) }
func (c *typeCache) Empty() bool { return c == nil || c.slots == nil }
