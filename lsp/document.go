package lsp

import (
	"bytes"
	"iter"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
	"github.com/FRFlo/lugo/semantic"
	"github.com/FRFlo/lugo/token"
)

type DiagPragmas struct {
	FileDisabled map[string]bool
	LineDisabled map[uint32]map[string]bool
}

type evictedNodeTextEntry struct {
	node   ast.NodeID
	offset uint32
	length uint32
}

type Document struct {
	// Identity
	Server     *Server
	URI        string
	Path       string
	LowerPath  string
	Dir        string
	ModuleName string

	// Parser State
	Tree                       *ast.Tree
	Resolver                   *semantic.Resolver
	metatableIndexMu           sync.Mutex
	metatableCalls             *metatableAttachmentIndex
	evictedNodeTextMu          sync.RWMutex
	evictedNodeText            []evictedNodeTextEntry
	evictedNodeTextData        []byte
	evictedGlobalRefs          map[uint64]struct{}
	evictedGenericContracts    []evictedNodeTextEntry
	evictedGenericContractData []byte
	// Source bytes are owned by the canonical Tree. Use Tree.Source as the authoritative source.
	Errors []parser.ParseError

	// Caches
	TypeCache          typeCache
	Inferring          []bool
	LuaDocCache        []*LuaDoc
	ActualReads        []uint16
	MutatedLocals      []bool
	ExportedGlobalDefs []ExportedSymbol
	ExportedNode       ast.NodeID

	// FiveM Fields
	FiveMProfile          FiveMExecutionProfile
	FiveMLuaExports       []FiveMLuaExport
	FiveMEvents           []FiveMEventInfo
	FiveMManifestResource *FiveMResource

	// Metadata
	IsMeta             bool
	IsLibrary          bool
	IsWorkspace        bool
	IsFiveMManifest    bool
	FiveMProfileCached bool
	ModTime            time.Time
	DiagPragmas        DiagPragmas
}

// Source returns the canonical source bytes from the owning Tree.
func (doc *Document) Source() []byte {
	if doc != nil && doc.Tree != nil {
		return doc.Tree.Source
	}
	return nil
}

func (doc *Document) sourceSlice(start, end uint32) []byte {
	src := doc.Source()
	if start > end || end > uint32(len(src)) {
		return nil
	}

	return src[start:end]
}

func (doc *Document) nodeSource(id ast.NodeID) []byte {
	if id == ast.InvalidNode || doc == nil || doc.Tree == nil || int(id) >= len(doc.Tree.Nodes) {
		return nil
	}

	node := doc.Tree.Nodes[id]
	if source := doc.sourceSlice(node.Start, node.End); source != nil {
		return source
	}
	if doc.Source() == nil {
		doc.evictedNodeTextMu.RLock()
		var text []byte
		low, high := 0, len(doc.evictedNodeText)
		for low < high {
			mid := low + (high-low)/2
			entry := doc.evictedNodeText[mid]
			if entry.node < id {
				low = mid + 1
			} else {
				high = mid
			}
		}
		if low < len(doc.evictedNodeText) && doc.evictedNodeText[low].node == id {
			entry := doc.evictedNodeText[low]
			end := uint64(entry.offset) + uint64(entry.length)
			if end <= uint64(len(doc.evictedNodeTextData)) {
				text = doc.evictedNodeTextData[entry.offset:uint32(end)]
			}
		}
		doc.evictedNodeTextMu.RUnlock()
		return text
	}
	return nil
}

// cacheEvictedMetatableNodeText retains small token spellings needed to rebuild
// metatable indexes, plus generic call contracts, after source eviction.
func (doc *Document) cacheEvictedMetatableNodeText() {
	if doc == nil || doc.Tree == nil || doc.Source() == nil {
		return
	}
	var text []evictedNodeTextEntry
	var seen map[ast.NodeID]struct{}
	var data []byte
	var globalRefs map[uint64]struct{}
	var metatableReceivers map[string]struct{}
	internedText := make(map[string]evictedNodeTextEntry)
	doc.evictedGenericContracts = nil
	doc.evictedGenericContractData = nil
	appendNodeText := func(id ast.NodeID) {
		if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		if value := doc.nodeSource(id); len(value) > 0 {
			key := string(value)
			entry, exists := internedText[key]
			if !exists {
				entry = evictedNodeTextEntry{offset: uint32(len(data)), length: uint32(len(value))}
				data = append(data, value...)
				internedText[key] = entry
			}
			if seen == nil {
				seen = make(map[ast.NodeID]struct{})
			}
			text = append(text, evictedNodeTextEntry{node: id, offset: entry.offset, length: entry.length})
			seen[id] = struct{}{}
		}
	}
	for id, node := range doc.Tree.Nodes {
		if node.Kind == ast.KindFunctionExpr && doc.cacheEvictedGenericContract(ast.NodeID(id)) {
			// Generic argument binding follows the actual parameter order,
			// which can differ from the order of @param annotations.
			for i := uint32(0); i < node.Count && node.Extra+i < uint32(len(doc.Tree.ExtraList)); i++ {
				appendNodeText(doc.Tree.ExtraList[node.Extra+i])
			}
		}
		if node.Kind == ast.KindRecordField && node.Left != ast.InvalidNode && int(node.Left) < len(doc.Tree.Nodes) && doc.Tree.Nodes[node.Left].Kind == ast.KindIdent {
			appendNodeText(node.Left)
		} else if node.Kind == ast.KindIndexField && node.Left != ast.InvalidNode && int(node.Left) < len(doc.Tree.Nodes) && doc.Tree.Nodes[node.Left].Kind == ast.KindString {
			appendNodeText(node.Left)
		} else if node.Kind == ast.KindMemberExpr && node.Right != ast.InvalidNode && int(node.Right) < len(doc.Tree.Nodes) && doc.Tree.Nodes[node.Right].Kind == ast.KindIdent {
			appendNodeText(node.Right)
		} else if node.Kind == ast.KindIndexExpr && node.Right != ast.InvalidNode && int(node.Right) < len(doc.Tree.Nodes) && doc.Tree.Nodes[node.Right].Kind == ast.KindString {
			appendNodeText(node.Right)
		} else if node.Kind == ast.KindCallExpr && node.Left != ast.InvalidNode && int(node.Left) < len(doc.Tree.Nodes) && doc.Tree.Nodes[node.Left].Kind == ast.KindIdent && bytes.Equal(doc.nodeSource(node.Left), []byte("setmetatable")) {
			appendNodeText(node.Left)
			if node.Count >= 2 && node.Extra+1 < uint32(len(doc.Tree.ExtraList)) {
				receiver := doc.Tree.ExtraList[node.Extra]
				appendNodeText(receiver)
				if name := doc.nodeSource(receiver); len(name) > 0 {
					if metatableReceivers == nil {
						metatableReceivers = make(map[string]struct{})
					}
					metatableReceivers[string(name)] = struct{}{}
				}
			}
		}
	}
	if doc.Resolver != nil {
		for _, ref := range doc.Resolver.GlobalRefs {
			if int(ref) >= len(doc.Tree.Nodes) {
				continue
			}
			if doc.Tree.Nodes[ref].Kind != ast.KindIdent {
				continue
			}
			appendNodeText(ref)
			if hash := ast.HashBytes(doc.nodeSource(ref)); hash != 0 {
				if globalRefs == nil {
					globalRefs = make(map[uint64]struct{})
				}
				globalRefs[hash] = struct{}{}
			}
		}
		for _, definition := range doc.Resolver.GlobalDefs {
			if int(definition) >= len(doc.Tree.Nodes) {
				continue
			}
			if _, needed := metatableReceivers[string(doc.nodeSource(definition))]; needed {
				appendNodeText(definition)
			}
		}
	}
	sort.Slice(text, func(i, j int) bool { return text[i].node < text[j].node })
	doc.evictedNodeTextMu.Lock()
	doc.evictedNodeText = text
	doc.evictedNodeTextData = data
	doc.evictedGlobalRefs = globalRefs
	doc.evictedNodeTextMu.Unlock()
}

func (doc *Document) clearEvictedMetatableNodeText() {
	if doc == nil {
		return
	}
	doc.evictedNodeTextMu.Lock()
	doc.evictedNodeText = nil
	doc.evictedNodeTextData = nil
	doc.evictedGlobalRefs = nil
	doc.evictedGenericContracts = nil
	doc.evictedGenericContractData = nil
	doc.evictedNodeTextMu.Unlock()
	doc.FiveMManifestResource = nil
}

func (doc *Document) referenceAt(id ast.NodeID) ast.NodeID {
	if doc == nil || doc.Resolver == nil || id == ast.InvalidNode || int(id) >= len(doc.Resolver.References) {
		return ast.InvalidNode
	}

	return doc.Resolver.References[id]
}

func (doc *Document) parseDiagnosticPragmas() {
	doc.DiagPragmas.FileDisabled = make(map[string]bool)
	doc.DiagPragmas.LineDisabled = make(map[uint32]map[string]bool)

	for _, c := range doc.Tree.Comments {
		src := doc.Source()[c.Start:c.End]

		idx := bytes.Index(src, []byte("---@diagnostic"))
		if idx == -1 {
			continue
		}

		rest := src[idx+14:]
		rest = bytes.TrimLeft(rest, " \t")

		var action string

		if bytes.HasPrefix(rest, []byte("disable-line")) {
			action = "disable-line"
			rest = rest[12:]
		} else if bytes.HasPrefix(rest, []byte("disable-next-line")) {
			action = "disable-next-line"
			rest = rest[17:]
		} else if bytes.HasPrefix(rest, []byte("disable-file")) {
			action = "disable-file"
			rest = rest[12:]
		} else {
			continue
		}

		rest = bytes.TrimLeft(rest, " \t")
		spaceIdx := bytes.IndexByte(rest, ' ')

		var rulesBytes []byte

		if spaceIdx != -1 {
			rulesBytes = rest[:spaceIdx]
		} else {
			rulesBytes = rest
		}

		rulesStr := string(bytes.TrimSpace(rulesBytes))
		rules := strings.SplitSeq(rulesStr, ",")

		line, _ := doc.Tree.Position(c.Start)

		if action == "disable-file" {
			for rule := range rules {
				doc.DiagPragmas.FileDisabled[strings.TrimSpace(rule)] = true
			}
		} else {
			targetLine := line
			if action == "disable-next-line" {
				targetLine = line + 1
			}

			if doc.DiagPragmas.LineDisabled[targetLine] == nil {
				doc.DiagPragmas.LineDisabled[targetLine] = make(map[string]bool)
			}

			for rule := range rules {
				doc.DiagPragmas.LineDisabled[targetLine][strings.TrimSpace(rule)] = true
			}
		}
	}
}

type FiveMLuaExport struct {
	Name   string
	NodeID ast.NodeID
}

type FiveMEventKind int

const (
	FiveMEventAddHandler FiveMEventKind = iota
	FiveMEventRegisterNet
	FiveMEventTriggerLocal
	FiveMEventTriggerServer
	FiveMEventTriggerClient
	FiveMFrameworkRegisterServerCallback
	FiveMFrameworkTriggerServerCallback
)

type FiveMEventInfo struct {
	Name             string
	Kind             FiveMEventKind
	NodeID           ast.NodeID
	HandlerID        ast.NodeID
	ExtraPayloadArgs uint8
}

func (doc *Document) getAssignedValue(id ast.NodeID) ast.NodeID {
	if id == ast.InvalidNode {
		return ast.InvalidNode
	}

	if uint(id) < uint(len(doc.Tree.Nodes)) {
		kind := doc.Tree.Nodes[id].Kind
		if kind == ast.KindFunctionExpr || kind == ast.KindTableExpr || kind == ast.KindString || kind == ast.KindHashedString || kind == ast.KindNumber || kind == ast.KindTrue || kind == ast.KindFalse || kind == ast.KindNil {
			return id
		}
	}

	curr := id

	for {
		parentID := doc.Tree.Nodes[curr].Parent
		if parentID == ast.InvalidNode {
			return ast.InvalidNode
		}

		parent := doc.Tree.Nodes[parentID]

		switch parent.Kind {
		case ast.KindLocalFunction:
			if parent.Left == curr {
				return parent.Right // Return the FunctionExpr body
			}

			return ast.InvalidNode
		case ast.KindFunctionStmt:
			return parent.Right
		case ast.KindRecordField, ast.KindIndexField:
			if parent.Left == curr {
				return parent.Right
			}

			return ast.InvalidNode
		case ast.KindNameList:
			grandParentID := doc.Tree.Nodes[parentID].Parent
			if grandParentID == ast.InvalidNode {
				return ast.InvalidNode
			}

			grandParentNode := doc.Tree.Nodes[grandParentID]
			if grandParentNode.Kind != ast.KindLocalAssign || grandParentNode.Right == ast.InvalidNode {
				return ast.InvalidNode
			}

			idx := doc.Tree.IndexOfExtra(parentID, curr)
			if idx == -1 {
				return ast.InvalidNode
			}

			rhsNode := doc.Tree.Nodes[grandParentNode.Right]
			if uint32(idx) >= rhsNode.Count {
				return ast.InvalidNode
			}

			return doc.Tree.ExtraList[rhsNode.Extra+uint32(idx)]
		case ast.KindExprList:
			grandParentID := doc.Tree.Nodes[parentID].Parent
			if grandParentID == ast.InvalidNode {
				return ast.InvalidNode
			}

			grandParentNode := doc.Tree.Nodes[grandParentID]
			if grandParentNode.Kind != ast.KindAssign || grandParentNode.Left != parentID || grandParentNode.Right == ast.InvalidNode {
				return ast.InvalidNode
			}

			idx := doc.Tree.IndexOfExtra(parentID, curr)
			if idx == -1 {
				return ast.InvalidNode
			}

			rhsNode := doc.Tree.Nodes[grandParentNode.Right]
			if uint32(idx) >= rhsNode.Count {
				return ast.InvalidNode
			}

			return doc.Tree.ExtraList[rhsNode.Extra+uint32(idx)]
		case ast.KindMemberExpr, ast.KindMethodName:
			curr = parentID
		default:
			return ast.InvalidNode
		}
	}
}

func (doc *Document) invalidateMetatableAttachmentIndex() {
	if doc == nil {
		return
	}
	doc.metatableIndexMu.Lock()
	doc.metatableCalls = nil
	doc.metatableIndexMu.Unlock()
}

func (doc *Document) getFunctionParams(funcExprID ast.NodeID, luadoc *LuaDoc) string {
	if doc == nil || doc.Tree == nil || funcExprID == ast.InvalidNode || int(funcExprID) >= len(doc.Tree.Nodes) {
		return ""
	}

	node := doc.Tree.Nodes[funcExprID]
	if node.Kind != ast.KindFunctionExpr {
		return ""
	}

	if luadoc != nil && node.Count == 0 && len(luadoc.Params) > 0 {
		var params []string

		for _, p := range luadoc.Params {
			if p.Type != "" {
				params = append(params, p.Name+": "+p.Type)
			} else {
				params = append(params, p.Name)
			}
		}

		return strings.Join(params, ", ")
	}

	paramTypes := make(map[string]string)

	if luadoc != nil {
		for _, p := range luadoc.Params {
			paramTypes[p.Name] = p.Type
		}
	}

	var params []string

	for i := uint32(0); i < node.Count; i++ {
		if node.Extra+uint32(i) >= uint32(len(doc.Tree.ExtraList)) {
			continue
		}

		pID := doc.Tree.ExtraList[node.Extra+uint32(i)]
		if pID == ast.InvalidNode || int(pID) >= len(doc.Tree.Nodes) {
			continue
		}

		name := ast.String(doc.nodeSource(pID))
		if name == "" {
			continue
		}

		if typ, ok := paramTypes[name]; ok && typ != "" {
			params = append(params, name+": "+typ)
		} else {
			params = append(params, name)
		}
	}

	return strings.Join(params, ", ")
}

func (doc *Document) findCommentIndex(offset uint32) int {
	var (
		low  int
		high = len(doc.Tree.Comments)
	)

	for low < high {
		mid := int(uint(low+high) >> 1)

		if doc.Tree.Comments[mid].End <= offset {
			low = mid + 1
		} else {
			high = mid
		}
	}

	return low - 1
}

// IterateCommentsAbove finds the contiguous block of comments directly above an AST node
// and yields each comment in reverse order (bottom-up).
func (doc *Document) IterateCommentsAbove(id ast.NodeID) iter.Seq[token.Token] {
	return func(yield func(token.Token) bool) {
		if id == ast.InvalidNode {
			return
		}

		src := doc.Source()
		if len(src) == 0 {
			return
		}

		stmtID := id

		for {
			parentID := doc.Tree.Nodes[stmtID].Parent
			if parentID == ast.InvalidNode {
				break
			}

			pKind := doc.Tree.Nodes[parentID].Kind
			if pKind == ast.KindBlock || pKind == ast.KindFile || pKind == ast.KindTableExpr {
				break
			}

			stmtID = parentID
		}

		stmtStart := doc.Tree.Nodes[stmtID].Start
		idx := doc.findCommentIndex(stmtStart)

		lastValidOffset := stmtStart

		for i := idx; i >= 0; i-- {
			c := doc.Tree.Comments[i]

			if c.End > lastValidOffset || lastValidOffset > uint32(len(src)) {
				break
			}

			gap := src[c.End:lastValidOffset]

			if hasAtMostOneNewline(gap) {
				if !yield(c) {
					return
				}

				lastValidOffset = c.Start
			} else {
				break
			}
		}
	}
}

// hasAtMostOneNewline preserves the comment attachment rule while stopping
// as soon as a second line break proves the gap is too large.
func hasAtMostOneNewline(gap []byte) bool {
	first := bytes.IndexByte(gap, '\n')
	return first < 0 || bytes.IndexByte(gap[first+1:], '\n') < 0
}

func (doc *Document) getCommentsAbove(id ast.NodeID) []byte {
	var validComments []token.Token

	for c := range doc.IterateCommentsAbove(id) {
		validComments = append(validComments, c)
	}

	if len(validComments) == 0 {
		return nil
	}

	doc.Server.sharedCommentBuf = doc.Server.sharedCommentBuf[:0]

	b := doc.Server.sharedCommentBuf

	for i := len(validComments) - 1; i >= 0; i-- {
		c := validComments[i]
		rawC := doc.sourceSlice(c.Start, c.End)
		if rawC == nil {
			continue
		}

		b = cleanLuaCommentBytes(b, rawC)

		if i > 0 && len(b) > 0 && b[len(b)-1] != '\n' {
			b = append(b, '\n')
		}
	}

	doc.Server.sharedCommentBuf = b

	return bytes.TrimSpace(b)
}

// GetLuaDoc parses and caches the LuaDoc for a given node.
func (doc *Document) GetLuaDoc(id ast.NodeID) *LuaDoc {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return nil
	}

	if int(id) >= len(doc.LuaDocCache) {
		if int(id) < cap(doc.LuaDocCache) {
			doc.LuaDocCache = doc.LuaDocCache[:len(doc.Tree.Nodes)]
		} else {
			newCache := make([]*LuaDoc, len(doc.Tree.Nodes))

			copy(newCache, doc.LuaDocCache)

			doc.LuaDocCache = newCache
		}
	}

	if ld := doc.LuaDocCache[id]; ld != nil {
		return ld
	}

	enableAlerts := doc.Server != nil && doc.Server.FeatureFormatAlerts

	comments := doc.getCommentsAbove(id)

	var ld LuaDoc

	if len(comments) > 0 {
		ld = parseLuaDoc(comments, enableAlerts)
	}

	doc.LuaDocCache[id] = &ld

	return &ld
}

// LocalsAt walks up the AST from the given offset and yields every local variable in scope.
func (doc *Document) LocalsAt(offset uint32) iter.Seq2[[]byte, ast.NodeID] {
	return func(yield func([]byte, ast.NodeID) bool) {
		nodeID := doc.Tree.NodeAt(offset)
		if nodeID == ast.InvalidNode {
			return
		}

		curr := nodeID

		for curr != ast.InvalidNode {
			node := doc.Tree.Nodes[curr]

			switch node.Kind {
			case ast.KindBlock, ast.KindFile:
				// Binary search for the active statement
				low, high := 0, int(node.Count)

				for low < high {
					mid := int(uint(low+high) >> 1)
					stmtID := doc.Tree.ExtraList[node.Extra+uint32(mid)]

					if doc.Tree.Nodes[stmtID].Start >= offset {
						high = mid
					} else {
						low = mid + 1
					}
				}

				lastStmtIdx := low - 1

				for i := lastStmtIdx; i >= 0; i-- {
					stmtID := doc.Tree.ExtraList[node.Extra+uint32(i)]
					stmtNode := doc.Tree.Nodes[stmtID]

					switch stmtNode.Kind {
					case ast.KindLocalAssign:
						if offset >= stmtNode.End {
							nameList := doc.Tree.Nodes[stmtNode.Left]

							// Iterate backwards to support `local a, a = 1, 2`
							for j := int(nameList.Count) - 1; j >= 0; j-- {
								identID := doc.Tree.ExtraList[nameList.Extra+uint32(j)]
								identNode := doc.Tree.Nodes[identID]

								if identNode.Start <= identNode.End && identNode.End <= uint32(len(doc.Source())) {
									if !yield(doc.Source()[identNode.Start:identNode.End], identID) {
										return
									}
								}
							}
						}
					case ast.KindLocalFunction:
						identNode := doc.Tree.Nodes[stmtNode.Left]

						if identNode.Start <= identNode.End && identNode.End <= uint32(len(doc.Source())) {
							if !yield(doc.Source()[identNode.Start:identNode.End], stmtNode.Left) {
								return
							}
						}
					}
				}
			case ast.KindFunctionExpr, ast.KindFunctionStmt:
				var funcExpr = curr

				if node.Kind == ast.KindFunctionStmt {
					funcExpr = node.Right

					if int(node.Left) < len(doc.Tree.Nodes) && doc.Tree.Nodes[node.Left].Kind == ast.KindMethodName {
						if !yield([]byte("self"), node.Left) {
							return
						}
					}
				}

				if funcExpr != ast.InvalidNode {
					exprNode := doc.Tree.Nodes[funcExpr]

					for i := uint32(0); i < exprNode.Count; i++ {
						paramID := doc.Tree.ExtraList[exprNode.Extra+uint32(i)]
						paramNode := doc.Tree.Nodes[paramID]

						if paramNode.Start <= paramNode.End && paramNode.End <= uint32(len(doc.Source())) {
							if !yield(doc.Source()[paramNode.Start:paramNode.End], paramID) {
								return
							}
						}
					}
				}
			case ast.KindForNum:
				var exprsEnd uint32

				if node.Count > 0 {
					lastExprID := doc.Tree.ExtraList[node.Extra+uint32(node.Count-1)]
					exprsEnd = doc.Tree.Nodes[lastExprID].End
				} else {
					exprsEnd = doc.Tree.Nodes[node.Left].End
				}

				if offset > exprsEnd {
					identNode := doc.Tree.Nodes[node.Left]

					if identNode.Start <= identNode.End && identNode.End <= uint32(len(doc.Source())) {
						if !yield(doc.Source()[identNode.Start:identNode.End], node.Left) {
							return
						}
					}
				}
			case ast.KindForIn:
				exprListID := ast.NodeID(node.Extra)
				if exprListID != ast.InvalidNode && offset > doc.Tree.Nodes[exprListID].End {
					nameList := doc.Tree.Nodes[node.Left]

					for i := uint32(0); i < nameList.Count; i++ {
						identID := doc.Tree.ExtraList[nameList.Extra+uint32(i)]
						identNode := doc.Tree.Nodes[identID]

						if identNode.Start <= identNode.End && identNode.End <= uint32(len(doc.Source())) {
							if !yield(doc.Source()[identNode.Start:identNode.End], identID) {
								return
							}
						}
					}
				}
			}

			curr = node.Parent
		}
	}
}

// ExtractLuaDocFields performs a highly optimized, zero-allocation byte scan
// for @field annotations in the comments directly above a node.
func (doc *Document) ExtractLuaDocFields(id ast.NodeID) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		fieldToken := []byte("@field")

		for c := range doc.IterateCommentsAbove(id) {
			raw := doc.sourceSlice(c.Start, c.End)
			if raw == nil {
				continue
			}

			idx := bytes.Index(raw, fieldToken)

			for idx != -1 {
				rest := raw[idx+6:]

				var j int

				for j < len(rest) && (rest[j] == ' ' || rest[j] == '\t') {
					j++
				}

				if bytes.HasPrefix(rest[j:], []byte("public ")) {
					j += 7
				} else if bytes.HasPrefix(rest[j:], []byte("private ")) {
					j += 8
				} else if bytes.HasPrefix(rest[j:], []byte("protected ")) {
					j += 10
				}

				for j < len(rest) && (rest[j] == ' ' || rest[j] == '\t') {
					j++
				}

				startName := j

				for j < len(rest) && rest[j] != ' ' && rest[j] != '\t' && rest[j] != '\n' && rest[j] != '\r' {
					j++
				}

				if j > startName {
					name := rest[startName:j]
					if len(name) > 0 && name[len(name)-1] == '?' {
						name = name[:len(name)-1]
					}

					if !yield(name) {
						return
					}
				}

				next := bytes.Index(rest, fieldToken)
				if next == -1 {
					break
				}

				idx += 6 + next
			}
		}
	}
}

// HasDeprecatedTag performs a fast, zero-allocation byte scan for @deprecated comments directly above a node.
func (doc *Document) HasDeprecatedTag(id ast.NodeID) (bool, string) {
	depToken := []byte("@deprecated")

	var (
		found bool
		msg   string
	)

	for c := range doc.IterateCommentsAbove(id) {
		raw := doc.sourceSlice(c.Start, c.End)
		if raw == nil {
			continue
		}

		_, after, ok := bytes.Cut(raw, depToken)
		if ok {
			rest := after

			endIdx := bytes.IndexByte(rest, '\n')
			if endIdx == -1 {
				endIdx = len(rest)
			}

			doc.Server.sharedDepBuf = doc.Server.sharedDepBuf[:0]

			msgBytes := cleanLuaCommentBytes(doc.Server.sharedDepBuf, rest[:endIdx])

			msg = string(bytes.TrimSpace(msgBytes))

			found = true

			doc.Server.sharedDepBuf = msgBytes

			break
		}
	}

	return found, msg
}

func cleanLuaCommentBytes(dst, raw []byte) []byte {
	for len(raw) > 0 {
		var line []byte

		idx := bytes.IndexByte(raw, '\n')
		if idx == -1 {
			line = raw
			raw = nil
		} else {
			line = raw[:idx]
			raw = raw[idx+1:]
		}

		line = bytes.TrimSpace(line)

		if bytes.HasPrefix(line, []byte("--[[")) {
			line = line[4:]
		} else if bytes.HasPrefix(line, []byte("---")) {
			line = line[3:]
		} else if bytes.HasPrefix(line, []byte("--")) {
			line = line[2:]
		}

		if bytes.HasSuffix(line, []byte("--]]")) {
			line = line[:len(line)-4]
		} else if bytes.HasSuffix(line, []byte("]]")) {
			line = line[:len(line)-2]
		}

		if len(line) > 0 && line[0] == ' ' {
			line = line[1:]
		}

		dst = append(dst, line...)

		if len(raw) > 0 || len(line) == 0 {
			dst = append(dst, '\n')
		}
	}

	return dst
}
