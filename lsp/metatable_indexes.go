package lsp

import (
	"bytes"
	"sort"

	"github.com/FRFlo/lugo/ast"
)

type metatableFunctionRange struct {
	start uint32
	end   uint32
}

type metatableAttachmentCall struct {
	node      ast.NodeID
	meta      ast.NodeID
	start     uint32
	uncertain bool
	functions []metatableFunctionRange
	branches  []metatableFunctionRange
	controls  []metatableFunctionRange
}

type metatableAttachmentIndex struct {
	tree         *ast.Tree
	byDefinition map[ast.NodeID][]metatableAttachmentCall
	byGlobalName map[uint64][]metatableAttachmentCall
	fieldWrites  map[ast.NodeID]map[string][]metatableFieldWrite
}

type metatableFieldWrite struct {
	start     uint32
	value     ast.NodeID
	functions []metatableFunctionRange
	branches  []metatableFunctionRange
	controls  []metatableFunctionRange
}

func (doc *Document) metatableAttachmentCalls() *metatableAttachmentIndex {
	doc.metatableIndexMu.Lock()
	defer doc.metatableIndexMu.Unlock()
	if cached := doc.metatableCalls; cached != nil && cached.tree == doc.Tree {
		return cached
	}

	index := &metatableAttachmentIndex{
		tree:         doc.Tree,
		byDefinition: make(map[ast.NodeID][]metatableAttachmentCall),
		byGlobalName: make(map[uint64][]metatableAttachmentCall),
	}
	for i := range doc.Tree.Nodes {
		call := doc.Tree.Nodes[i]
		if call.Kind != ast.KindCallExpr || int(call.Left) >= len(doc.Tree.Nodes) || call.Count < 2 || call.Extra+1 >= uint32(len(doc.Tree.ExtraList)) {
			continue
		}
		callee := doc.Tree.Nodes[call.Left]
		if callee.Kind != ast.KindIdent || doc.referenceAt(call.Left) != ast.InvalidNode || !bytes.Equal(doc.nodeSource(call.Left), []byte("setmetatable")) {
			continue
		}
		receiverID := doc.Tree.ExtraList[call.Extra]
		if int(receiverID) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[receiverID].Kind != ast.KindIdent {
			continue
		}
		definition := doc.referenceAt(receiverID)
		var globalNameHash uint64
		if definition == ast.InvalidNode {
			name := doc.nodeSource(receiverID)
			if len(name) > 0 {
				globalNameHash = ast.HashBytes(name)
			}
			var latest ast.NodeID
			for _, globalID := range doc.Resolver.GlobalDefs {
				if int(globalID) < len(doc.Tree.Nodes) && bytes.Equal(doc.nodeSource(globalID), name) && doc.Tree.Nodes[globalID].Start <= call.Start && (latest == ast.InvalidNode || doc.Tree.Nodes[globalID].Start > doc.Tree.Nodes[latest].Start) {
					latest = globalID
				}
			}
			definition = latest
			if definition == ast.InvalidNode && globalNameHash == 0 {
				continue
			}
		}
		candidate := metatableAttachmentCall{node: ast.NodeID(i), meta: doc.Tree.ExtraList[call.Extra+1], start: call.Start}
		uncertain := false
		unreachable := false
		child := ast.NodeID(i)
		for parent := call.Parent; parent != ast.InvalidNode && int(parent) < len(doc.Tree.Nodes); parent = doc.Tree.Nodes[parent].Parent {
			ancestor := doc.Tree.Nodes[parent]
			if ancestor.Kind == ast.KindFunctionExpr {
				candidate.functions = append(candidate.functions, metatableFunctionRange{start: ancestor.Start, end: ancestor.End})
			}
			// These constructs can skip their body or select a different arm.
			// Until the index can prove a path executes, don't expose its
			// attachment as the certain state after the construct.
			switch ancestor.Kind {
			case ast.KindIf, ast.KindElseIf, ast.KindElse, ast.KindWhile, ast.KindForNum, ast.KindForIn, ast.KindRepeat:
				if ancestor.Kind == ast.KindElse && ancestor.Parent != ast.InvalidNode && int(ancestor.Parent) < len(doc.Tree.Nodes) {
					ifNode := doc.Tree.Nodes[ancestor.Parent]
					if ifNode.Kind == ast.KindIf && ifNode.Count == 1 && ifNode.Left != ast.InvalidNode && int(ifNode.Left) < len(doc.Tree.Nodes) && (doc.Tree.Nodes[ifNode.Left].Kind == ast.KindFalse || doc.Tree.Nodes[ifNode.Left].Kind == ast.KindNil) {
						break
					}
				}
				if ancestor.Kind == ast.KindIf && ancestor.Count == 1 && ancestor.Left != ast.InvalidNode && int(ancestor.Left) < len(doc.Tree.Nodes) && (doc.Tree.Nodes[ancestor.Left].Kind == ast.KindFalse || doc.Tree.Nodes[ancestor.Left].Kind == ast.KindNil) && doc.Tree.Nodes[child].Kind == ast.KindElse {
					break
				}
				if ancestor.Kind == ast.KindIf && ancestor.Left != ast.InvalidNode && int(ancestor.Left) < len(doc.Tree.Nodes) && doc.Tree.Nodes[ancestor.Left].Kind == ast.KindTrue {
					if ancestor.Right == child {
						break
					}
					if child != ancestor.Left {
						unreachable = true
						break
					}
				}
				uncertain = true
				branch := doc.Tree.Nodes[child]
				candidate.branches = append(candidate.branches, metatableFunctionRange{start: branch.Start, end: branch.End})
				candidate.controls = append(candidate.controls, metatableFunctionRange{start: ancestor.Start, end: ancestor.End})
			}
			if (ancestor.Kind == ast.KindIf || ancestor.Kind == ast.KindElseIf || ancestor.Kind == ast.KindWhile) && ancestor.Right == child && ancestor.Left != ast.InvalidNode && int(ancestor.Left) < len(doc.Tree.Nodes) {
				condition := doc.Tree.Nodes[ancestor.Left]
				if condition.Kind == ast.KindFalse || condition.Kind == ast.KindNil {
					unreachable = true
					break
				}
			}
			child = parent
		}
		if !unreachable {
			candidate.uncertain = uncertain
			if definition != ast.InvalidNode {
				index.byDefinition[definition] = append(index.byDefinition[definition], candidate)
			} else if globalNameHash != 0 && len(candidate.functions) == 0 {
				// Cross-document readers query the end-of-file state; retain only
				// the latest non-function call for this unresolved global name.
				// Same-document bindings keep the complete ordered history above.
				index.byGlobalName[globalNameHash] = []metatableAttachmentCall{candidate}
			}
		}
	}
	for definition := range index.byDefinition {
		sort.Slice(index.byDefinition[definition], func(i, j int) bool {
			return index.byDefinition[definition][i].start < index.byDefinition[definition][j].start
		})
	}
	doc.metatableCalls = index
	return index
}

// metatableFieldWrites builds the field-write index only when a consumer
// needs field values; ordinary identifier inference only needs attachments.
func (doc *Document) metatableFieldWrites() *metatableAttachmentIndex {
	for {
		index := doc.metatableAttachmentCalls()
		doc.metatableIndexMu.Lock()
		if doc.metatableCalls != index || index.tree != doc.Tree {
			doc.metatableIndexMu.Unlock()
			continue
		}
		if index.fieldWrites != nil {
			doc.metatableIndexMu.Unlock()
			return index
		}
		index.fieldWrites = make(map[ast.NodeID]map[string][]metatableFieldWrite)
		for i, member := range doc.Tree.Nodes {
			if (member.Kind != ast.KindMemberExpr && member.Kind != ast.KindIndexExpr) || member.Left == ast.InvalidNode || member.Right == ast.InvalidNode || int(member.Left) >= len(doc.Tree.Nodes) || int(member.Right) >= len(doc.Tree.Nodes) {
				continue
			}
			definition := doc.referenceAt(member.Left)
			if definition == ast.InvalidNode {
				continue
			}
			key := doc.Tree.Nodes[member.Right]
			var name string
			if member.Kind == ast.KindMemberExpr && key.Kind == ast.KindIdent {
				name = string(doc.nodeSource(member.Right))
			} else if member.Kind == ast.KindIndexExpr && key.Kind == ast.KindString {
				if decoded, ok := doc.evalNode(member.Right, 0); ok && decoded.kind == ast.KindString {
					name = decoded.str
				}
			}
			if name == "" || member.Parent == ast.InvalidNode || int(member.Parent) >= len(doc.Tree.Nodes) {
				continue
			}
			list := doc.Tree.Nodes[member.Parent]
			if list.Kind != ast.KindExprList || list.Parent == ast.InvalidNode || int(list.Parent) >= len(doc.Tree.Nodes) {
				continue
			}
			assign := doc.Tree.Nodes[list.Parent]
			if assign.Kind != ast.KindAssign || assign.Left != member.Parent {
				continue
			}
			unreachable := false
			var functions []metatableFunctionRange
			var branches, controls []metatableFunctionRange
			child := list.Parent
			for parent := assign.Parent; parent != ast.InvalidNode && int(parent) < len(doc.Tree.Nodes); parent = doc.Tree.Nodes[parent].Parent {
				ancestor := doc.Tree.Nodes[parent]
				if ancestor.Kind == ast.KindFunctionExpr {
					functions = append(functions, metatableFunctionRange{start: ancestor.Start, end: ancestor.End})
				}
				if ancestor.Kind == ast.KindIf || ancestor.Kind == ast.KindElseIf || ancestor.Kind == ast.KindElse {
					control := ancestor
					if ancestor.Kind != ast.KindIf && ancestor.Parent != ast.InvalidNode && int(ancestor.Parent) < len(doc.Tree.Nodes) {
						control = doc.Tree.Nodes[ancestor.Parent]
					}
					branches = append(branches, metatableFunctionRange{start: doc.Tree.Nodes[child].Start, end: doc.Tree.Nodes[child].End})
					controls = append(controls, metatableFunctionRange{start: control.Start, end: control.End})
				}
				if (ancestor.Kind == ast.KindIf || ancestor.Kind == ast.KindElseIf || ancestor.Kind == ast.KindWhile) && ancestor.Left != ast.InvalidNode && int(ancestor.Left) < len(doc.Tree.Nodes) {
					condition := doc.Tree.Nodes[ancestor.Left]
					if ancestor.Right == child && (condition.Kind == ast.KindFalse || condition.Kind == ast.KindNil) {
						unreachable = true
						break
					}
					if ancestor.Kind == ast.KindIf && condition.Kind == ast.KindTrue && child != ancestor.Right && child != ancestor.Left {
						unreachable = true
						break
					}
				}
				child = parent
			}
			if unreachable {
				continue
			}
			value := doc.getAssignedValue(ast.NodeID(i))
			if value == ast.InvalidNode {
				continue
			}
			if index.fieldWrites[definition] == nil {
				index.fieldWrites[definition] = make(map[string][]metatableFieldWrite)
			}
			index.fieldWrites[definition][name] = append(index.fieldWrites[definition][name], metatableFieldWrite{start: assign.Start, value: value, functions: functions, branches: branches, controls: controls})
		}
		for definition, fields := range index.fieldWrites {
			for name := range fields {
				sort.SliceStable(index.fieldWrites[definition][name], func(i, j int) bool {
					return index.fieldWrites[definition][name][i].start < index.fieldWrites[definition][name][j].start
				})
			}
		}
		doc.metatableIndexMu.Unlock()
		return index
	}
}

type newIndexWriteCandidate struct {
	tree        *ast.Tree
	doc         *Document
	node        ast.NodeID
	member      ast.NodeID
	assignStart uint32
}

// metatableFieldValueAt returns a table field's latest value before offset,
// including ordinary assignments such as mt.__newindex = sink.
func (doc *Document) metatableFieldValueAt(tableID ast.NodeID, name string, offset uint32) ast.NodeID {
	if doc == nil || doc.Tree == nil || tableID == ast.InvalidNode || int(tableID) >= len(doc.Tree.Nodes) {
		return ast.InvalidNode
	}
	value := doc.findFieldInTable(tableID, name)
	latestStart := uint32(0)
	definition := doc.getDefForValue(tableID)
	if definition == ast.InvalidNode || doc.Resolver == nil {
		return value
	}
	index := doc.metatableFieldWrites()
	for _, write := range index.fieldWrites[definition][name] {
		if write.start >= offset {
			break
		}
		visible := true
		for _, function := range write.functions {
			if offset < function.start || offset > function.end {
				visible = false
				break
			}
		}
		for i, branch := range write.branches {
			control := write.controls[i]
			if offset < control.start || offset > control.end || offset < branch.start || offset > branch.end {
				visible = false
				break
			}
		}
		if visible && write.start >= latestStart {
			value, latestStart = write.value, write.start
		}
	}
	return value
}

type newIndexWriteIndex struct {
	version uint64
	byName  map[string][]newIndexWriteCandidate
}

func (s *Server) trackMetatableTypeDependency(sourceURI, dependentURI string) {
	if s == nil || sourceURI == "" || dependentURI == "" || sourceURI == dependentURI {
		return
	}
	s.metatableIndexMu.Lock()
	if s.metatableTypeDependents == nil {
		s.metatableTypeDependents = make(map[string]map[string]struct{})
	}
	dependents := s.metatableTypeDependents[sourceURI]
	if dependents == nil {
		dependents = make(map[string]struct{})
		s.metatableTypeDependents[sourceURI] = dependents
	}
	dependents[dependentURI] = struct{}{}
	s.metatableIndexMu.Unlock()
}

func (s *Server) forgetMetatableTypeDependenciesFrom(uri string) {
	if s == nil || uri == "" {
		return
	}
	s.metatableIndexMu.Lock()
	delete(s.metatableTypeDependents, uri)
	for source, dependents := range s.metatableTypeDependents {
		delete(dependents, uri)
		if len(dependents) == 0 {
			delete(s.metatableTypeDependents, source)
		}
	}
	s.metatableIndexMu.Unlock()
}

func (s *Server) forgetMetatableTypeDependencyConsumer(uri string) {
	if s == nil || uri == "" {
		return
	}
	s.metatableIndexMu.Lock()
	for source, dependents := range s.metatableTypeDependents {
		delete(dependents, uri)
		if len(dependents) == 0 {
			delete(s.metatableTypeDependents, source)
		}
	}
	s.metatableIndexMu.Unlock()
}

func (s *Server) invalidateMetatableIndexes(changedURI ...string) {
	if s == nil {
		return
	}
	if s.metatableInvalidationBatch != nil {
		for _, uri := range changedURI {
			if uri != "" {
				s.metatableInvalidationBatch[uri] = struct{}{}
			}
		}
		return
	}
	s.invalidateMetatableIndexesNow(changedURI...)
}

func (s *Server) flushMetatableInvalidationBatch() {
	if s == nil || s.metatableInvalidationBatch == nil {
		return
	}
	batch := s.metatableInvalidationBatch
	s.metatableInvalidationBatch = nil
	if len(batch) == 0 {
		return
	}
	changedURIs := make([]string, 0, len(batch))
	for uri := range batch {
		changedURIs = append(changedURIs, uri)
	}
	sort.Strings(changedURIs)
	for _, uri := range changedURIs {
		if doc := s.Documents[uri]; doc != nil {
			if doc.Tree == nil {
				doc.TypeCache.Drop()
			} else {
				doc.TypeCache.Reset(len(doc.Tree.Nodes))
			}
			clear(doc.Inferring)
		}
	}
	s.invalidateMetatableIndexesNow(changedURIs...)
}

func (s *Server) invalidateMetatableIndexesNow(changedURIs ...string) {
	if s == nil {
		return
	}
	s.metatableIndexMu.Lock()
	resetTypeCache := func(doc *Document) {
		if doc.Tree == nil {
			doc.TypeCache.Drop()
			return
		}
		doc.TypeCache.Reset(len(doc.Tree.Nodes))
	}
	s.metatableIndexVersion++
	s.newIndexWrites = nil
	if len(changedURIs) != 0 {
		newGlobalNames := make(map[uint64]struct{})
		changedEvents := make(map[string]struct{})
		changedSet := make(map[string]struct{}, len(changedURIs))
		queue := make([]string, 0, len(changedURIs))
		visited := make(map[string]struct{})
		for _, uri := range changedURIs {
			if uri == "" {
				continue
			}
			changedSet[uri] = struct{}{}
			queue = append(queue, uri)
			changed := s.Documents[uri]
			if changed != nil {
				// A previously unknown dispatcher can appear in a new file. Its
				// consumers depend on the provider resource even before a concrete
				// dispatcher document exists to receive a dependency edge.
				if root := s.getDocumentFiveMProfile(changed).ResourceRoot; root != "" && root != uri && len(s.metatableTypeDependents[root]) != 0 {
					queue = append(queue, root)
				}
				for _, event := range changed.FiveMEvents {
					changedEvents[event.Name] = struct{}{}
				}
			}
			if changed != nil && changed.Resolver != nil {
				for _, definition := range changed.Resolver.GlobalDefs {
					if hash := ast.HashBytes(changed.nodeSource(definition)); hash != 0 {
						newGlobalNames[hash] = struct{}{}
					}
				}
				for name := range changed.metatableAttachmentCalls().byGlobalName {
					newGlobalNames[name] = struct{}{}
				}
			}
			// A manifest edit changes visibility rules without changing any script
			// definitions. Invalidate the resource's documents as dependency roots
			// so cached resolutions based on the old profile are recomputed.
			if changed != nil && changed.IsFiveMManifest {
				root := s.getDocResourceRoot(changed)
				if root != "" {
					for resourceURI, resourceDoc := range s.Documents {
						if resourceDoc == nil || resourceDoc == changed || s.getDocResourceRoot(resourceDoc) != root {
							continue
						}
						visited[resourceURI] = struct{}{}
						resetTypeCache(resourceDoc)
						clear(resourceDoc.Inferring)
						queue = append(queue, resourceURI)
					}
				}
			}
		}
		// A newly indexed global may resolve a previously unknown name in any
		// document. Such consumers could not have registered a source edge yet,
		// so discover them by matching unresolved resolver references.
		for consumerURI, consumer := range s.Documents {
			if consumer == nil || (len(newGlobalNames) == 0 && len(changedEvents) == 0) {
				continue
			}
			if _, changed := changedSet[consumerURI]; changed {
				continue
			}
			matched := false
			for _, event := range consumer.FiveMEvents {
				if _, exists := changedEvents[event.Name]; exists {
					matched = true
					break
				}
			}
			if consumer.Resolver != nil && consumer.Tree != nil && consumer.Source() != nil {
				for _, ref := range consumer.Resolver.GlobalRefs {
					if int(ref) >= len(consumer.Tree.Nodes) {
						continue
					}
					if consumer.Tree.Nodes[ref].Kind == ast.KindIdent {
						if _, exists := newGlobalNames[ast.HashBytes(consumer.nodeSource(ref))]; exists {
							matched = true
							break
						}
					}
				}
			} else {
				consumer.evictedNodeTextMu.RLock()
				for hash := range consumer.evictedGlobalRefs {
					if _, exists := newGlobalNames[hash]; exists {
						matched = true
						break
					}
				}
				consumer.evictedNodeTextMu.RUnlock()
			}
			if matched {
				if _, exists := visited[consumerURI]; exists {
					continue
				}
				visited[consumerURI] = struct{}{}
				resetTypeCache(consumer)
				clear(consumer.Inferring)
				queue = append(queue, consumerURI)
			}
		}

		for len(queue) != 0 {
			uri := queue[0]
			queue = queue[1:]
			for dependentURI := range s.metatableTypeDependents[uri] {
				if _, exists := visited[dependentURI]; exists {
					continue
				}
				visited[dependentURI] = struct{}{}
				if dependent := s.Documents[dependentURI]; dependent != nil {
					resetTypeCache(dependent)
					clear(dependent.Inferring)
					queue = append(queue, dependentURI)
				}
			}
		}
	}
	s.metatableIndexMu.Unlock()
}

func (s *Server) newIndexWriteCandidates(name string) []newIndexWriteCandidate {
	if s == nil {
		return nil
	}
	s.metatableIndexMu.Lock()
	defer s.metatableIndexMu.Unlock()
	if s.newIndexWrites == nil || s.newIndexWrites.version != s.metatableIndexVersion {
		index := &newIndexWriteIndex{version: s.metatableIndexVersion, byName: make(map[string][]newIndexWriteCandidate)}
		for name, candidates := range buildNewIndexWriteIndex(s.Documents) {
			index.byName[name] = candidates
		}
		s.newIndexWrites = index
	}
	return s.newIndexWrites.byName[name]
}

func buildNewIndexWriteCandidates(docs []*Document, name string) []newIndexWriteCandidate {
	return buildNewIndexWriteIndexFromSlice(docs)[name]
}

func buildNewIndexWriteIndex(docs map[string]*Document) map[string][]newIndexWriteCandidate {
	values := make([]*Document, 0, len(docs))
	for _, doc := range docs {
		values = append(values, doc)
	}
	return buildNewIndexWriteIndexFromSlice(values)
}

func buildNewIndexWriteIndexFromSlice(docs []*Document) map[string][]newIndexWriteCandidate {
	byName := make(map[string][]newIndexWriteCandidate)
	for _, candidateDoc := range docs {
		if candidateDoc == nil || candidateDoc.Tree == nil || candidateDoc.Resolver == nil {
			continue
		}
		for i, member := range candidateDoc.Tree.Nodes {
			if (member.Kind != ast.KindIndexExpr && member.Kind != ast.KindMemberExpr) || member.Right == ast.InvalidNode || int(member.Right) >= len(candidateDoc.Tree.Nodes) || member.Parent == ast.InvalidNode || int(member.Parent) >= len(candidateDoc.Tree.Nodes) {
				continue
			}
			key := candidateDoc.Tree.Nodes[member.Right]
			var name string
			node := ast.NodeID(i)
			if member.Kind == ast.KindMemberExpr && key.Kind == ast.KindIdent {
				name = string(candidateDoc.nodeSource(member.Right))
				node = member.Right
			} else if member.Kind == ast.KindIndexExpr && key.Kind == ast.KindString {
				value, ok := candidateDoc.evalNode(member.Right, 0)
				if !ok || value.kind != ast.KindString {
					continue
				}
				name = value.str
			} else {
				continue
			}
			list := candidateDoc.Tree.Nodes[member.Parent]
			if list.Kind != ast.KindExprList || list.Parent == ast.InvalidNode || int(list.Parent) >= len(candidateDoc.Tree.Nodes) {
				continue
			}
			assign := candidateDoc.Tree.Nodes[list.Parent]
			if assign.Kind != ast.KindAssign || assign.Left != member.Parent || name == "" {
				continue
			}
			byName[name] = append(byName[name], newIndexWriteCandidate{tree: candidateDoc.Tree, doc: candidateDoc, node: node, member: ast.NodeID(i), assignStart: assign.Start})
		}
	}
	for name := range byName {
		sort.Slice(byName[name], func(i, j int) bool {
			return byName[name][i].assignStart > byName[name][j].assignStart
		})
	}
	return byName
}

// metatableFieldAlternativesAt includes values from mutually exclusive writes
// completed before offset. The ordinary lookup remains the baseline branch;
// alternatives are deduplicated so the common single-value path stays cheap.
func (doc *Document) metatableFieldAlternativesAt(tableID ast.NodeID, name string, offset uint32) []ast.NodeID {
	base := doc.metatableFieldValueAt(tableID, name, offset)
	if base == ast.InvalidNode || doc == nil || doc.Resolver == nil || offset == ^uint32(0) {
		if base == ast.InvalidNode {
			return nil
		}
		return []ast.NodeID{base}
	}
	definition := doc.getDefForValue(tableID)
	index := doc.metatableFieldWrites()
	writes := index.fieldWrites[definition][name]
	var result []ast.NodeID
	seen := map[ast.NodeID]bool{base: true}
	for i := len(writes) - 1; i >= 0; i-- {
		w := writes[i]
		if w.start >= offset || len(w.branches) == 0 {
			continue
		}
		branch := w.branches[0]
		control := w.controls[0]
		if control.end >= offset {
			continue
		}
		// Keep only the last write within this arm.
		newer := false
		for j := i + 1; j < len(writes); j++ {
			other := writes[j]
			if len(other.branches) > 0 && other.controls[0] == control && other.branches[0] == branch && other.start < offset {
				newer = true
				break
			}
		}
		if !newer && !seen[w.value] {
			seen[w.value] = true
			result = append(result, w.value)
		}
	}
	if len(result) == 0 {
		return []ast.NodeID{base}
	}
	return append([]ast.NodeID{base}, result...)
}
