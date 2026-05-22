# AST Module — Flat-Array Arena Architecture

**Location**: `ast/` — 2 files (ast.go: 461 lines, ast_test.go: 90 lines)

The AST uses a **flat-array arena** architecture instead of traditional pointer-based trees. All nodes live in a contiguous `[]Node` slice indexed by `NodeID (uint32)`. This is the foundation of Lugo's zero-allocation performance.

---

## Why Flat Arrays Instead of Pointers

| Aspect | Pointer Tree | Arena (this module) |
|--------|-------------|---------------------|
| Allocation | Per-node heap alloc | Single `append` to slice |
| Cache locality | Scattered heap | Contiguous memory |
| GC pressure | Every node tracked | Zero (all in one slice) |
| Node size | 64+ bytes (with pointers) | 48 bytes (fixed) |
| Arena reuse | Impossible | `Reset()` reuses capacity |
| Max nodes | Unlimited | 4B (`uint32`) |

---

## Core Data Structures

### NodeID (`uint32`)
Index into `Tree.Nodes`. `InvalidNode = 0` serves as null/nil sentinel.

### Node (48 bytes, fixed-size)
```go
type Node struct {
    Start, End uint32    // Byte offsets into source
    Parent     NodeID    // Parent node (upward traversal)
    Left       NodeID    // Primary child
    Right      NodeID    // Secondary child
    Extra      uint32    // Index into Tree.ExtraList (N-ary children)
    Count      uint16    // Number of items in ExtraList
    Kind       NodeKind  // One of 47 node kinds (1 byte)
    Flags      uint8     // Boolean flags
}
```

### Tree (Flat-Array Arena)
```go
type Tree struct {
    Source      []byte       // Original source (never copied)
    Root        NodeID       // KindFile node
    Nodes       []Node       // Main arena
    Comments    []Token      // Comment token boundaries
    ExtraList   []NodeID     // Flattened child lists
    LineOffsets []uint32     // Line start positions
}
```

---

## Node Kinds (47 total)

| Category | Kinds |
|----------|-------|
| **Root** | `KindFile`, `KindBlock` |
| **Statements** | `KindLocalAssign`, `KindAssign`, `KindBreak`, `KindReturn`, `KindLabel`, `KindGoto`, `KindDo`, `KindWhile`, `KindRepeat`, `KindIf`, `KindElseIf`, `KindElse`, `KindForNum`, `KindForIn`, `KindLocalFunction`, `KindFunctionStmt` |
| **Expressions** | `KindIdent`, `KindNumber`, `KindString`, `KindHashedString`, `KindBinaryExpr`, `KindUnaryExpr`, `KindParenExpr`, `KindNil`, `KindTrue`, `KindFalse`, `KindVararg`, `KindFunctionExpr`, `KindTableExpr` |
| **Access/Call** | `KindIndexExpr` (a[b]), `KindMemberExpr` (a.b), `KindCallExpr` (a(b)), `KindMethodCall` (a:b()) |
| **Table Fields** | `KindRecordField` (a=1), `KindIndexField` ([a]=1) |
| **Lists** | `KindExprList`, `KindNameList` |

---

## Child Relationship Patterns

### Binary nodes (if, while, assign, binary expr)
```
Left  → condition / LHS / primary child
Right → body / RHS / secondary child
```

### N-ary nodes (Block, File, ExprList)
```
Extra → starting index in ExtraList[Extra : Extra + Count]
Count → number of consecutive children in ExtraList
```

### Leaf nodes (identifiers, literals)
```
Left = Right = InvalidNode
Start/End = byte offsets into source
```

---

## Public API

### Construction
```go
tree := ast.NewTree(source []byte) *Tree        // Create with pre-allocated arenas
tree.Reset(source []byte)                        // Reuse for new source (zero-alloc on warm paths)
id := tree.AddNode(Node{Kind: ast.KindBlock})    // Append, set parent, return NodeID
```

### Navigation
```go
line, col := tree.Position(offset uint32)        // Byte offset → (line, col)
offset := tree.Offset(line, col uint32)          // (line, col) → byte offset
nodeID := tree.NodeAt(offset uint32)             // Narrowest node containing offset
```

### Utility
```go
hash := ast.HashBytes(b []byte) uint64                    // FNV-1a
hash := ast.HashBytesConcat(a, sep, b []byte) uint64     // Concat-hash (no alloc)
str := ast.String(b []byte) string                        // Zero-alloc []byte → string (unsafe)
s := node.String(tree.Source)                             // Zero-alloc node text
```

---

## Key Algorithms

### NodeAt (O(log depth) — narrowest node at offset)
1. Start at Root, verify offset in range
2. Check Left, Right children for containment
3. For N-ary nodes: **binary search** ExtraList (children sorted by source position)
4. Descend into matching child, repeat until no narrower match
5. Used by every LSP feature that resolves a position to a symbol

### Position/Offset (binary search on LineOffsets)
- `Position()`: Binary search `LineOffsets` for line, count UTF-8 runes for column
- `Offset()`: Direct index into `LineOffsets` + UTF-8 rune counting

### Pre-allocation Heuristics
```go
capNodes  := sourceLen / 10 + 1024    // ~10% of source length
capExtra  := capNodes / 2              // Half of node capacity
capLines  := sourceLen / 30 + 128      // ~1 line per 30 bytes
```

---

## Memory Layout Diagram

```
Tree {
  Source:     [b y t e   o f f s e t s ...]
  
  Nodes: [
    0: InvalidNode (sentinel)
    1: {Kind: KindFile, Left: 2, Right: InvalidNode, Extra: 0, Count: 1}
    2: {Kind: KindBlock, Left: 3, ...}
    3: {Kind: KindLocalAssign, Left: 4, Right: 6, ...}
    4: {Kind: KindNameList, Extra: 0, Count: 1}
    5: {Kind: KindIdent, Start: 0, End: 5}   -- "local"
    6: {Kind: KindExprList, Extra: 1, Count: 1}
    7: {Kind: KindNumber, Start: 14, End: 15} -- "1"
  ]
  
  ExtraList: [5, 7]     -- Children of nodes 4 and 6
  Comments:  [...]       -- Token boundaries
  LineOffsets: [0, 16]  -- Each line's start byte
}
```
