package lsp

import (
	"bytes"
	"encoding/json"
	"iter"
	"slices"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

var luaKeywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for", "function",
	"goto", "if", "in", "local", "nil", "not", "or", "repeat", "return",
	"then", "true", "until", "while",
}

type GlobalSymbol struct {
	URI           string
	Name          string
	Parent        string
	NodeID        ast.NodeID
	IsRoot        bool
	IsDeprecated  bool
	DeprecatedMsg string
}

type ExportedSymbol struct {
	NodeID        ast.NodeID
	Key           GlobalKey
	IsRoot        bool
	IsDeprecated  bool
	DeprecatedMsg string
}

type GlobalKey struct {
	ReceiverHash uint64 // 0 if it's a root global
	PropHash     uint64
}

type GlobalReference struct {
	Doc    *Document
	URI    string
	NodeID ast.NodeID
}

type CallerKey struct {
	URI string
	Def ast.NodeID
}

type TargetKey struct {
	URI string
	Def ast.NodeID
}

type RefKey struct {
	URI string
	ID  ast.NodeID
}

type SymbolContext struct {
	TargetDoc      *Document
	IdentName      string
	DisplayName    string
	TargetURI      string
	GlobalDefs     []GlobalSymbol
	GKey           GlobalKey
	IdentNodeID    ast.NodeID
	TargetDefID    ast.NodeID
	RecDefID       ast.NodeID
	IsProp         bool
	IsGlobal       bool
	FiveMExportRes string
}

type symbolReceiverInfo struct {
	Type                   TypeSet
	Known                  bool
	NodeID                 ast.NodeID
	CanReuseDefinitionType bool
}

func (s *Server) handleDefinition(req Request) {
	var params TextDocumentPositionParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	uri := s.normalizeURI(params.TextDocument.URI)

	doc, ok := s.Documents[uri]
	if !ok {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})

		return
	}

	offset := doc.Tree.Offset(params.Position.Line, params.Position.Character)
	if doc.IsFiveMManifest {
		if res := s.parseFiveMManifest(doc); res != nil && res.Manifest != nil {
			for _, entry := range res.Manifest.Entries {
				if entry.ValueRange.Start.Line == params.Position.Line && params.Position.Character >= entry.ValueRange.Start.Character && params.Position.Character <= entry.ValueRange.End.Character {
					target := s.manifestPathTarget(doc, entry)
					if _, ok := s.Documents[target]; ok {
						WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: []Location{{URI: target, Range: Range{Start: Position{}, End: Position{}}}}})
						return
					}
				}
			}
		}
	}
	if locs := s.getFiveMEventDefinitionLocations(doc, offset); len(locs) > 0 {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: locs})

		return
	}
	// Built-in events have no source file. Return the stable virtual resource
	// instead of an empty/opaque definition so clients can navigate to it.
	if event, ok := doc.fiveMEventAtOffset(offset); ok {
		if _, builtin := EventsBuiltin[event.Name]; builtin {
			WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: []Location{{
				URI:   builtinFiveMEventsURI,
				Range: Range{Start: Position{}, End: Position{}},
			}}})
			return
		}
	}

	ctx := s.resolveSymbolAt(uri, offset)

	if ctx != nil {
		var locs []Location

		if len(ctx.GlobalDefs) > 0 {
			for _, def := range ctx.GlobalDefs {
				if tDoc, ok := s.Documents[def.URI]; ok {
					locs = append(locs, Location{
						URI:   def.URI,
						Range: getNodeRange(tDoc.Tree, def.NodeID),
					})
				}
			}
		} else if ctx.TargetDefID != ast.InvalidNode && ctx.TargetDoc != nil {
			locs = append(locs, Location{
				URI:   ctx.TargetURI,
				Range: getNodeRange(ctx.TargetDoc.Tree, ctx.TargetDefID),
			})
		}

		if len(locs) > 0 {
			WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: locs})

			return
		}
	}

	WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})
}

func (s *Server) handleTypeDefinition(req Request) {
	var params TypeDefinitionParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	uri := s.normalizeURI(params.TextDocument.URI)

	doc, ok := s.Documents[uri]
	if !ok {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})

		return
	}

	offset := doc.Tree.Offset(params.Position.Line, params.Position.Character)
	ctx := s.resolveSymbolAt(uri, offset)
	if ctx == nil {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})

		return
	}

	locs := s.typeDefinitionLocations(doc, ctx)
	if len(locs) > 0 {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: locs})

		return
	}

	WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})
}

func (s *Server) handleImplementation(req Request) {
	var params ImplementationParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	uri := s.normalizeURI(params.TextDocument.URI)

	doc, ok := s.Documents[uri]
	if !ok {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})

		return
	}

	offset := doc.Tree.Offset(params.Position.Line, params.Position.Character)
	ctx := s.resolveSymbolAt(uri, offset)
	if ctx == nil {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})

		return
	}

	locs := s.implementationLocations(doc, ctx)
	if len(locs) > 0 {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: locs})

		return
	}

	WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})
}

func (s *Server) typeDefinitionLocations(srcDoc *Document, ctx *SymbolContext) []Location {
	if s == nil || ctx == nil || srcDoc == nil {
		return nil
	}

	var locs []Location
	seen := make(map[TargetKey]struct{})

	addNodeLocation := func(uri string, nodeID ast.NodeID) {
		if uri == "" || nodeID == ast.InvalidNode || strings.HasPrefix(uri, embeddedStdlibURIPrefix) {
			return
		}

		tDoc, ok := s.Documents[uri]
		if !ok || !s.canSeeSymbol(srcDoc, tDoc) || int(nodeID) >= len(tDoc.Tree.Nodes) {
			return
		}

		key := TargetKey{URI: uri, Def: nodeID}
		if _, ok := seen[key]; ok {
			return
		}

		seen[key] = struct{}{}
		locs = append(locs, Location{URI: uri, Range: getNodeRange(tDoc.Tree, nodeID)})
	}

	addTypeName := func(typeName string) {
		if typeName == "" || s.GlobalIndex == nil {
			return
		}

		var recHash uint64
		propName := typeName
		if lastDot := strings.LastIndexByte(typeName, '.'); lastDot != -1 {
			recHash = ast.HashBytes([]byte(typeName[:lastDot]))
			propName = typeName[lastDot+1:]
		}

		if syms, ok := s.getGlobalSymbols(srcDoc, recHash, ast.HashBytes([]byte(propName))); ok {
			for _, sym := range syms {
				addNodeLocation(sym.URI, sym.NodeID)
			}
		}

		typeHash := ast.HashBytes([]byte(typeName))
		if s.TableAliases != nil {
			if tableHash := s.TableAliases[typeHash]; tableHash != 0 {
				for _, entry := range s.GlobalIndex.AllSymbols() {
					if entry == nil || ast.HashBytes([]byte(entry.Name)) != tableHash {
						continue
					}

					addNodeLocation(entry.URI, entry.NodeID)
				}
			}
		}
	}

	addTypeSet := func(t TypeSet) {
		if t.CustomName != "" {
			addTypeName(t.CustomName)
		}

		if t.DeclNode != ast.InvalidNode && t.DeclURI != "" {
			addNodeLocation(t.DeclURI, t.DeclNode)
		}
	}

	if ctx.TargetDefID != ast.InvalidNode && ctx.TargetDoc != nil {
		if luadoc := ctx.TargetDoc.GetLuaDoc(ctx.TargetDefID); luadoc != nil {
			if luadoc.Type != nil {
				addTypeSet(ParseTypeString(luadoc.Type.Type))
			}

			if luadoc.Class != nil {
				addTypeName(luadoc.Class.Name)
			}
		}

		addTypeSet(ctx.TargetDoc.InferType(ctx.TargetDefID))
	} else if ctx.IdentNodeID != ast.InvalidNode && ctx.TargetDoc != nil {
		addTypeSet(ctx.TargetDoc.InferType(ctx.IdentNodeID))
	}

	return locs
}

func (s *Server) implementationLocations(srcDoc *Document, ctx *SymbolContext) []Location {
	if s == nil || ctx == nil || srcDoc == nil || s.GlobalIndex == nil {
		return nil
	}

	propHash := ctx.GKey.PropHash
	if propHash == 0 && ctx.IdentName != "" {
		propHash = ast.HashBytes([]byte(ctx.IdentName))
	}
	if propHash == 0 {
		return nil
	}

	recHashes := s.implementationReceiverHashes(srcDoc, ctx)
	if len(recHashes) == 0 {
		return nil
	}

	var locs []Location
	seen := make(map[TargetKey]struct{})

	addSymbol := func(sym GlobalSymbol) {
		if sym.URI == "" || sym.NodeID == ast.InvalidNode || strings.HasPrefix(sym.URI, embeddedStdlibURIPrefix) {
			return
		}

		tDoc, ok := s.Documents[sym.URI]
		if !ok || !s.canSeeSymbol(srcDoc, tDoc) || int(sym.NodeID) >= len(tDoc.Tree.Nodes) {
			return
		}

		key := TargetKey{URI: sym.URI, Def: sym.NodeID}
		if _, ok := seen[key]; ok {
			return
		}

		seen[key] = struct{}{}
		locs = append(locs, Location{URI: sym.URI, Range: getNodeRange(tDoc.Tree, sym.NodeID)})
	}

	for _, recHash := range recHashes {
		if syms, ok := s.getGlobalSymbols(srcDoc, recHash, propHash); ok {
			for _, sym := range syms {
				addSymbol(sym)
			}
		}
	}

	return locs
}

func (s *Server) implementationReceiverHashes(doc *Document, ctx *SymbolContext) []uint64 {
	var recHashes []uint64

	addHash := func(hash uint64) {
		if hash != 0 {
			recHashes = appendUniqueHash(recHashes, hash)
		}
	}

	addClassType := func(t TypeSet) {
		if t.CustomName == "" {
			return
		}

		classHash := ast.HashBytes([]byte(t.CustomName))
		addHash(classHash)

		if s.TableAliases != nil {
			addHash(s.TableAliases[classHash])
		}
	}

	addHash(ctx.GKey.ReceiverHash)

	if ctx.RecDefID != ast.InvalidNode {
		addClassType(doc.InferType(ctx.RecDefID))
	}

	if ctx.IsProp && ctx.IdentNodeID != ast.InvalidNode && int(ctx.IdentNodeID) < len(doc.Tree.Nodes) {
		pID := doc.Tree.Nodes[ctx.IdentNodeID].Parent
		if pID != ast.InvalidNode && int(pID) < len(doc.Tree.Nodes) {
			recID := doc.Tree.Nodes[pID].Left
			if recID != ast.InvalidNode {
				addClassType(doc.InferType(recID))
			}
		}
	}

	if ctx.TargetDefID != ast.InvalidNode && ctx.TargetDoc != nil {
		addClassType(ctx.TargetDoc.InferType(ctx.TargetDefID))
	}

	if s.TableAliases != nil {
		for classHash, tableHash := range s.TableAliases {
			for _, recHash := range recHashes {
				if recHash == classHash || recHash == tableHash {
					addHash(classHash)
					addHash(tableHash)
				}
			}
		}
	}

	return recHashes
}

func (s *Server) getFiveMEventDefinitionLocations(doc *Document, offset uint32) []Location {
	if s == nil || doc == nil || doc.Tree == nil {
		return nil
	}

	sourceEvent, ok := doc.fiveMEventAtOffset(offset)
	if !ok {
		return nil
	}

	type eventDefinitionKey struct {
		URI    string
		NodeID ast.NodeID
	}

	var locs []Location
	seen := make(map[eventDefinitionKey]struct{})
	sourceURI := s.documentURI(doc)
	addLocation := func(targetDoc *Document, targetURI string, target FiveMEventInfo) {
		if targetDoc == nil || target.NodeID == ast.InvalidNode {
			return
		}

		key := eventDefinitionKey{URI: targetURI, NodeID: target.NodeID}
		if _, ok := seen[key]; ok {
			return
		}

		seen[key] = struct{}{}
		locs = append(locs, Location{
			URI:   targetURI,
			Range: targetDoc.fiveMEventNameRange(target.NodeID),
		})
	}

	addMatching := func(kinds ...FiveMEventKind) {
		for targetURI, targetDoc := range s.Documents {
			if targetDoc == nil {
				continue
			}

			for _, target := range targetDoc.FiveMEvents {
				if target.Name != sourceEvent.Name || target.NodeID == ast.InvalidNode {
					continue
				}

				if targetURI == sourceURI && target.NodeID == sourceEvent.NodeID {
					continue
				}

				for _, kind := range kinds {
					if target.Kind == kind {
						addLocation(targetDoc, targetURI, target)
						break
					}
				}
			}
		}
	}

	switch sourceEvent.Kind {
	case FiveMEventTriggerLocal, FiveMEventTriggerServer, FiveMEventTriggerClient:
		addMatching(FiveMEventAddHandler)
		if len(locs) == 0 {
			addMatching(FiveMEventRegisterNet)
		}
	case FiveMEventAddHandler:
		addMatching(FiveMEventRegisterNet)
		if len(locs) == 0 {
			addMatching(FiveMEventAddHandler)
		}
	case FiveMEventRegisterNet:
		addMatching(FiveMEventAddHandler)
	}

	return locs
}

func (doc *Document) fiveMEventAtOffset(offset uint32) (FiveMEventInfo, bool) {
	if doc == nil || doc.Tree == nil {
		return FiveMEventInfo{}, false
	}

	for _, ev := range doc.FiveMEvents {
		if ev.Kind == FiveMFrameworkRegisterServerCallback || ev.Kind == FiveMFrameworkTriggerServerCallback {
			continue
		}
		if ev.NodeID == ast.InvalidNode || int(ev.NodeID) >= len(doc.Tree.Nodes) {
			continue
		}

		node := doc.Tree.Nodes[ev.NodeID]
		if node.Kind != ast.KindString {
			continue
		}

		start, end := doc.fiveMEventNameOffsets(ev.NodeID)
		if start <= offset && offset <= end {
			return ev, true
		}
	}

	return FiveMEventInfo{}, false
}

func (doc *Document) fiveMEventNameRange(nodeID ast.NodeID) Range {
	start, end := doc.fiveMEventNameOffsets(nodeID)

	return getRange(doc.Tree, start, end)
}

func (doc *Document) fiveMEventNameOffsets(nodeID ast.NodeID) (uint32, uint32) {
	node := doc.Tree.Nodes[nodeID]
	start := node.Start
	end := node.End
	src := doc.Source()

	if start < end && end <= uint32(len(src)) {
		first := src[start]
		if (first == '\'' || first == '"') && src[end-1] == first {
			start++
			end--
		}
	}

	return start, end
}

func (s *Server) documentURI(doc *Document) string {
	for uri, candidate := range s.Documents {
		if candidate == doc {
			return uri
		}
	}

	return ""
}

func (s *Server) globalIndexContext(doc *Document) (ResourceURI, GlobalIndexScope) {
	if doc == nil {
		return "", GlobalIndexScopeShared
	}

	profile := s.getDocumentFiveMProfile(doc)
	if profile.IsResourceProfile() && profile.ResourceRoot != "" {
		scope := GlobalIndexScopeShared
		switch profile.Env() {
		case EnvClient:
			scope = GlobalIndexScopeClient
		case EnvServer:
			scope = GlobalIndexScopeServer
		}
		return ResourceURI(profile.ResourceRoot), scope
	}

	return ResourceURI(doc.URI), GlobalIndexScopeShared
}

func globalSymbolFromEntry(entry *SymbolEntry) GlobalSymbol {
	if entry == nil {
		return GlobalSymbol{}
	}

	return GlobalSymbol{
		URI:           entry.URI,
		Name:          string(entry.Name),
		Parent:        entry.Parent,
		NodeID:        entry.NodeID,
		IsRoot:        entry.IsRoot,
		IsDeprecated:  entry.IsDeprecated,
		DeprecatedMsg: entry.DeprecatedMsg,
	}
}

func (s *Server) visibleGlobalSymbolsFromEntries(srcDoc *Document, entries []*SymbolEntry, max int) []GlobalSymbol {
	if len(entries) == 0 {
		return nil
	}

	// Hash buckets are often sparse after visibility filtering, so keep their
	// output lazy. A consistently visible prefix is a useful signal for dense
	// buckets, where reserving capacity avoids repeated slice and map growth.
	capacity := 0
	const sampleSize = 8
	if len(entries) >= 1024 {
		visible := 0
		for _, entry := range entries[:sampleSize] {
			if entry == nil {
				continue
			}
			tgtDoc, ok := s.Documents[entry.URI]
			if ok && s.canSeeSymbol(srcDoc, tgtDoc) {
				visible++
			}
		}
		if visible == sampleSize {
			capacity = len(entries)
		}
	}

	var seen map[TargetKey]struct{}
	if capacity > 0 {
		seen = make(map[TargetKey]struct{}, capacity)
	}
	filtered := make([]GlobalSymbol, 0, capacity)

	for _, entry := range entries {
		if entry == nil {
			continue
		}

		tgtDoc, ok := s.Documents[entry.URI]
		if !ok || !s.canSeeSymbol(srcDoc, tgtDoc) {
			continue
		}

		target := TargetKey{URI: entry.URI, Def: entry.NodeID}
		if seen == nil {
			seen = make(map[TargetKey]struct{})
		}
		if _, ok := seen[target]; ok {
			continue
		}

		seen[target] = struct{}{}
		filtered = append(filtered, globalSymbolFromEntry(entry))
	}

	slices.SortStableFunc(filtered, func(a, b GlobalSymbol) int {
		pa, pb := s.globalSymbolPriority(a.URI), s.globalSymbolPriority(b.URI)
		if pa != pb {
			return pa - pb
		}
		if a.IsRoot != b.IsRoot {
			if a.IsRoot {
				return -1
			}
			return 1
		}
		if a.URI != b.URI {
			return strings.Compare(a.URI, b.URI)
		}
		if a.NodeID < b.NodeID {
			return -1
		}
		if a.NodeID > b.NodeID {
			return 1
		}
		return 0
	})
	if max > 0 && len(filtered) > max {
		filtered = filtered[:max]
	}

	return filtered
}

func (s *Server) symbolInformationFromEntry(entry *SymbolEntry) (SymbolInformation, bool) {
	if entry == nil {
		return SymbolInformation{}, false
	}
	if strings.HasPrefix(entry.URI, embeddedStdlibURIPrefix) {
		return SymbolInformation{}, false
	}

	doc, ok := s.Documents[entry.URI]
	if !ok {
		return SymbolInformation{}, false
	}

	kind := SymbolKindVariable
	valID := doc.getAssignedValue(entry.NodeID)

	if valID != ast.InvalidNode {
		valKind := doc.Tree.Nodes[valID].Kind
		if valKind == ast.KindFunctionExpr {
			if entry.Key.ReceiverHash != 0 {
				kind = SymbolKindMethod
			} else {
				kind = SymbolKindFunction
			}
		} else if valKind == ast.KindTableExpr {
			kind = SymbolKindClass
		} else if entry.Key.ReceiverHash != 0 {
			kind = SymbolKindField
		}
	} else if entry.Key.ReceiverHash != 0 {
		kind = SymbolKindField
	}

	return SymbolInformation{
		Name: string(entry.Name),
		Kind: kind,
		Location: Location{
			URI:   entry.URI,
			Range: getNodeRange(doc.Tree, entry.NodeID),
		},
	}, true
}

func (s *Server) setGlobalIndexSymbol(resource ResourceURI, scope GlobalIndexScope, name SymbolName, entry *SymbolEntry) {
	if s == nil || s.GlobalIndex == nil || resource == "" || name == "" || entry == nil {
		return
	}

	s.GlobalIndex.mu.Lock()
	defer s.GlobalIndex.mu.Unlock()

	res := s.GlobalIndex.ensureResourceLocked(resource)
	entry.Name = name
	if entry.URI == "" {
		entry.URI = string(resource)
	}
	for _, old := range append([]*SymbolEntry(nil), res.Candidates[scope][name]...) {
		if old != nil && old.URI == entry.URI && old.NodeID == entry.NodeID && old.Key == entry.Key && old.Parent == entry.Parent && (old.Export != nil) == (entry.Export != nil) && (old.Export == nil || old.Export.SourceURI == entry.Export.SourceURI) {
			s.GlobalIndex.removeCandidateLocked(res, scope, name, old)
		}
	}
	s.GlobalIndex.addCandidateLocked(res, scope, name, entry)
}

func (idx *GlobalIndex) removeEntryFromHashLocked(stale *SymbolEntry) {
	if idx == nil || stale == nil || stale.Key == (GlobalKey{}) {
		return
	}

	entries := idx.HashIndex[stale.Key]
	for i, entry := range entries {
		if entry == stale || (entry != nil && entry.URI == stale.URI && entry.NodeID == stale.NodeID) {
			copy(entries[i:], entries[i+1:])
			entries[len(entries)-1] = nil
			entries = entries[:len(entries)-1]
			break
		}
	}

	if len(entries) == 0 {
		delete(idx.HashIndex, stale.Key)
	} else {
		idx.HashIndex[stale.Key] = entries
	}
}

func (s *Server) handleReferences(req Request) {
	var params ReferenceParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	uri := s.normalizeURI(params.TextDocument.URI)

	doc, ok := s.Documents[uri]
	if !ok {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})

		return
	}

	offset := doc.Tree.Offset(params.Position.Line, params.Position.Character)
	if locations := s.getFiveMEventReferenceLocations(doc, offset); len(locations) > 0 {
		WriteMessage(s.Writer, Response{
			RPC:    "2.0",
			ID:     req.ID,
			Result: locations,
		})

		return
	}

	ctx := s.resolveSymbolAt(uri, offset)

	if ctx == nil {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: []Location{}})

		return
	}

	locations := s.getReferences(ctx, params.Context.IncludeDeclaration)

	WriteMessage(s.Writer, Response{
		RPC:    "2.0",
		ID:     req.ID,
		Result: locations,
	})
}

func (s *Server) handleDocumentSymbol(req Request) {
	var params DocumentSymbolParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	uri := s.normalizeURI(params.TextDocument.URI)

	doc, ok := s.Documents[uri]
	if !ok {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})

		return
	}

	var (
		walkTable func(tableID ast.NodeID, out *[]DocumentSymbol)
		walk      func(nodeID ast.NodeID, out *[]DocumentSymbol)
	)

	walkTable = func(tableID ast.NodeID, out *[]DocumentSymbol) {
		node := doc.Tree.Nodes[tableID]

		for i := uint32(0); i < node.Count; i++ {
			fieldID := doc.Tree.ExtraList[node.Extra+uint32(i)]
			fieldNode := doc.Tree.Nodes[fieldID]

			if fieldNode.Kind == ast.KindRecordField {
				keyNode := doc.Tree.Nodes[fieldNode.Left]
				valNode := doc.Tree.Nodes[fieldNode.Right]

				name := ast.String(doc.Source()[keyNode.Start:keyNode.End])
				if name == "" {
					name = "<error>"
				}

				kind := SymbolKindField

				var children []DocumentSymbol

				switch valNode.Kind {
				case ast.KindFunctionExpr:
					kind = SymbolKindMethod
					walk(valNode.Right, &children)
				case ast.KindTableExpr:
					kind = SymbolKindClass
					walkTable(fieldNode.Right, &children)
				case ast.KindCallExpr, ast.KindMethodCall:
					walk(fieldNode.Right, &children)
				}

				*out = append(*out, DocumentSymbol{
					Name:           name,
					Kind:           kind,
					Range:          getNodeRange(doc.Tree, fieldID),
					SelectionRange: getNodeRange(doc.Tree, fieldNode.Left),
					Children:       children,
				})
			}
		}
	}

	walk = func(nodeID ast.NodeID, out *[]DocumentSymbol) {
		if nodeID == ast.InvalidNode {
			return
		}

		node := doc.Tree.Nodes[nodeID]

		switch node.Kind {
		case ast.KindFile:
			walk(node.Left, out)
		case ast.KindBlock:
			for i := uint32(0); i < node.Count; i++ {
				walk(doc.Tree.ExtraList[node.Extra+uint32(i)], out)
			}
		case ast.KindLocalFunction, ast.KindFunctionStmt:
			nameNode := doc.Tree.Nodes[node.Left]

			name := ast.String(doc.Source()[nameNode.Start:nameNode.End])
			if name == "" {
				name = "<error>"
			}

			kind := SymbolKindFunction
			if nameNode.Kind == ast.KindMethodName {
				kind = SymbolKindMethod
			}

			var children []DocumentSymbol

			if node.Right != ast.InvalidNode {
				funcExpr := doc.Tree.Nodes[node.Right]
				if funcExpr.Kind == ast.KindFunctionExpr {
					walk(funcExpr.Right, &children)
				}
			}

			*out = append(*out, DocumentSymbol{
				Name:           name,
				Kind:           kind,
				Range:          getNodeRange(doc.Tree, nodeID),
				SelectionRange: getNodeRange(doc.Tree, node.Left),
				Children:       children,
			})
		case ast.KindLocalAssign, ast.KindAssign:
			lhsList := doc.Tree.Nodes[node.Left]
			rhsList := node.Right

			if rhsList != ast.InvalidNode {
				rhsNode := doc.Tree.Nodes[rhsList]

				for i := uint32(0); i < lhsList.Count && i < rhsNode.Count; i++ {
					lID := doc.Tree.ExtraList[lhsList.Extra+uint32(i)]
					lNode := doc.Tree.Nodes[lID]

					var (
						rID   = ast.InvalidNode
						rNode ast.Node
					)

					if i < rhsNode.Count {
						rID = doc.Tree.ExtraList[rhsNode.Extra+uint32(i)]
						rNode = doc.Tree.Nodes[rID]
					}

					name := ast.String(doc.Source()[lNode.Start:lNode.End])
					if name == "" {
						name = "<error>"
					}

					switch rNode.Kind {
					case ast.KindFunctionExpr:
						var children []DocumentSymbol

						walk(rNode.Right, &children)

						*out = append(*out, DocumentSymbol{
							Name:           name,
							Kind:           SymbolKindFunction,
							Range:          getNodeRange(doc.Tree, nodeID),
							SelectionRange: getNodeRange(doc.Tree, lID),
							Children:       children,
						})
					case ast.KindTableExpr:
						var children []DocumentSymbol

						walkTable(rID, &children)

						*out = append(*out, DocumentSymbol{
							Name:           name,
							Kind:           SymbolKindClass,
							Range:          getNodeRange(doc.Tree, nodeID),
							SelectionRange: getNodeRange(doc.Tree, lID),
							Children:       children,
						})
					default:
						if node.Kind == ast.KindLocalAssign {
							*out = append(*out, DocumentSymbol{
								Name:           name,
								Kind:           SymbolKindVariable,
								Range:          getNodeRange(doc.Tree, lID),
								SelectionRange: getNodeRange(doc.Tree, lID),
							})
						}

						if rNode.Kind == ast.KindCallExpr || rNode.Kind == ast.KindMethodCall {
							walk(rID, out)
						}
					}
				}
			}
		case ast.KindCallExpr, ast.KindMethodCall:
			var (
				funcName    string
				funcIdentID ast.NodeID
			)

			if node.Kind == ast.KindMethodCall {
				funcIdentID = node.Right
				if int(node.Left) < len(doc.Tree.Nodes) && int(node.Right) < len(doc.Tree.Nodes) {
					leftNode := doc.Tree.Nodes[node.Left]
					rightNode := doc.Tree.Nodes[node.Right]

					if leftNode.Start <= rightNode.End && rightNode.End <= uint32(len(doc.Source())) {
						funcName = ast.String(doc.Source()[leftNode.Start:rightNode.End])
					}
				}
			} else {
				if int(node.Left) < len(doc.Tree.Nodes) {
					leftNode := doc.Tree.Nodes[node.Left]
					if leftNode.Start <= leftNode.End && leftNode.End <= uint32(len(doc.Source())) {
						funcName = ast.String(doc.Source()[leftNode.Start:leftNode.End])
					}

					if leftNode.Kind == ast.KindMemberExpr {
						funcIdentID = leftNode.Right
					} else {
						funcIdentID = node.Left
					}
				}
			}

			var (
				targetFuncID = ast.InvalidNode
				targetDoc    *Document
				paramOffset  int
			)

			if funcIdentID != ast.InvalidNode && int(funcIdentID) < len(doc.Tree.Nodes) {
				ctx := s.resolveSymbolNode(uri, doc, funcIdentID)
				if ctx != nil && ctx.TargetDefID != ast.InvalidNode && ctx.TargetDoc != nil {
					valID := ctx.TargetDoc.getAssignedValue(ctx.TargetDefID)
					if valID != ast.InvalidNode && int(valID) < len(ctx.TargetDoc.Tree.Nodes) {
						if ctx.TargetDoc.Tree.Nodes[valID].Kind == ast.KindFunctionExpr {
							targetFuncID = valID
							targetDoc = ctx.TargetDoc

							paramOffset = getImplicitSelfOffset(ctx, node, ctx.TargetDoc, ctx.TargetDefID)
						}
					}
				}
			}

			for i := uint32(0); i < node.Count; i++ {
				argID := doc.Tree.ExtraList[node.Extra+uint32(i)]
				argNode := doc.Tree.Nodes[argID]

				switch argNode.Kind {
				case ast.KindFunctionExpr:
					paramName := "callback"

					// Attempt to map the argument back to the function's parameter list
					if targetFuncID != ast.InvalidNode && targetDoc != nil {
						targetFuncNode := targetDoc.Tree.Nodes[targetFuncID]
						paramIdx := int(i) + paramOffset
						if paramIdx >= 0 && paramIdx < int(targetFuncNode.Count) {
							if targetFuncNode.Extra+uint32(paramIdx) < uint32(len(targetDoc.Tree.ExtraList)) {
								pID := targetDoc.Tree.ExtraList[targetFuncNode.Extra+uint32(paramIdx)]
								if int(pID) < len(targetDoc.Tree.Nodes) {
									pNode := targetDoc.Tree.Nodes[pID]
									src := targetDoc.Source()
									if len(src) == 0 {
										continue
									}
									if pNode.Start <= pNode.End && pNode.End <= uint32(len(src)) {
										pNameStr := ast.String(src[pNode.Start:pNode.End])
										if pNameStr != "" && pNameStr != "..." {
											paramName = pNameStr
										}
									}
								}
							}
						}
					}

					name := "(anonymous function)"
					if funcName != "" {
						name = paramName + " in " + funcName
					}

					var selRange Range

					if argNode.Start+8 <= argNode.End {
						selRange = getRange(doc.Tree, argNode.Start, argNode.Start+8)
					} else {
						selRange = getNodeRange(doc.Tree, argID)
					}

					var children []DocumentSymbol

					walk(argNode.Right, &children)

					*out = append(*out, DocumentSymbol{
						Name:           name,
						Kind:           SymbolKindFunction,
						Range:          getNodeRange(doc.Tree, argID),
						SelectionRange: selRange,
						Children:       children,
					})
				case ast.KindCallExpr, ast.KindMethodCall:
					walk(argID, out)
				case ast.KindTableExpr:
					walkTable(argID, out)
				}
			}
		case ast.KindReturn:
			if node.Left != ast.InvalidNode {
				exprList := doc.Tree.Nodes[node.Left]

				for i := uint32(0); i < exprList.Count; i++ {
					exprID := doc.Tree.ExtraList[exprList.Extra+uint32(i)]
					exprNode := doc.Tree.Nodes[exprID]

					switch exprNode.Kind {
					case ast.KindFunctionExpr:
						var selRange Range

						if exprNode.Start+8 <= exprNode.End {
							selRange = getRange(doc.Tree, exprNode.Start, exprNode.Start+8)
						} else {
							selRange = getNodeRange(doc.Tree, exprID)
						}

						var children []DocumentSymbol

						walk(exprNode.Right, &children)

						*out = append(*out, DocumentSymbol{
							Name:           "(return function)",
							Kind:           SymbolKindFunction,
							Range:          getNodeRange(doc.Tree, exprID),
							SelectionRange: selRange,
							Children:       children,
						})
					case ast.KindCallExpr, ast.KindMethodCall:
						walk(exprID, out)
					}
				}
			}
		case ast.KindIf:
			walk(node.Right, out)

			for i := uint32(0); i < node.Count; i++ {
				walk(doc.Tree.ExtraList[node.Extra+uint32(i)], out)
			}
		case ast.KindElseIf, ast.KindWhile, ast.KindForIn, ast.KindForNum:
			walk(node.Right, out)
		case ast.KindElse, ast.KindRepeat, ast.KindDo:
			walk(node.Left, out)
		}
	}

	var symbols []DocumentSymbol

	walk(doc.Tree.Root, &symbols)

	if symbols == nil {
		symbols = []DocumentSymbol{}
	}

	WriteMessage(s.Writer, Response{
		RPC:    "2.0",
		ID:     req.ID,
		Result: symbols,
	})
}

func (s *Server) handleWorkspaceSymbol(req Request) {
	var params WorkspaceSymbolParams

	err := json.Unmarshal(req.Params, &params)
	if err != nil {
		return
	}

	queryLower := strings.ToLower(params.Query)

	var (
		results []SymbolInformation
		count   int
	)

	seenTargets := make(map[TargetKey]struct{})
	if s.GlobalIndex != nil {
		for _, entry := range s.GlobalIndex.WorkspaceSymbols(params.Query, MaxWorkspaceResults) {
			info, ok := s.symbolInformationFromEntry(entry)
			if !ok {
				continue
			}

			target := TargetKey{URI: entry.URI, Def: entry.NodeID}
			seenTargets[target] = struct{}{}
			results = append(results, info)
			count++

			if count >= MaxWorkspaceResults {
				break
			}
		}
	}

	if count < MaxWorkspaceResults {
		seen := make(map[string]struct{}, len(results))
		for _, result := range results {
			seen[result.Name] = struct{}{}
		}

		addEventSymbol := func(name string, location Location) bool {
			if !containsFold(name, queryLower) {
				return false
			}
			if _, ok := seen[name]; ok {
				return false
			}

			results = append(results, SymbolInformation{
				Name:     name,
				Kind:     SymbolKindEvent,
				Location: location,
			})
			seen[name] = struct{}{}
			count++

			return count >= MaxWorkspaceResults
		}

		builtinLocation := Location{
			URI: builtinFiveMEventsURI,
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 0},
			},
		}
		for eventName := range EventsBuiltin {
			if addEventSymbol(eventName, builtinLocation) {
				break
			}
		}

		if count < MaxWorkspaceResults {
			for uri, doc := range s.Documents {
				for _, event := range doc.FiveMEvents {
					if event.Kind == FiveMFrameworkRegisterServerCallback || event.Kind == FiveMFrameworkTriggerServerCallback {
						continue
					}
					if addEventSymbol(event.Name, Location{URI: uri, Range: getNodeRange(doc.Tree, event.NodeID)}) {
						break
					}
				}
				if count >= MaxWorkspaceResults {
					break
				}
			}
		}
	}

	if results == nil {
		results = []SymbolInformation{}
	}

	WriteMessage(s.Writer, Response{
		RPC:    "2.0",
		ID:     req.ID,
		Result: results,
	})
}

func (s *Server) resolveSymbolAt(uri string, offset uint32) *SymbolContext {
	doc, ok := s.Documents[uri]
	if !ok {
		return nil
	}

	nodeID := doc.Tree.NodeAt(offset)

	return s.resolveSymbolNode(uri, doc, nodeID)
}

func appendUniqueHash(hashes []uint64, hash uint64) []uint64 {
	for _, existing := range hashes {
		if existing == hash {
			return hashes
		}
	}

	return append(hashes, hash)
}

func (s *Server) resolveSymbolNode(uri string, doc *Document, nodeID ast.NodeID) *SymbolContext {
	return s.resolveSymbolNodeWithReceiverType(uri, doc, nodeID, nil)
}

func (s *Server) resolveSymbolNodeWithReceiverType(uri string, doc *Document, nodeID ast.NodeID, receiver *symbolReceiverInfo) *SymbolContext {
	if nodeID == ast.InvalidNode || int(nodeID) >= len(doc.Tree.Nodes) {
		return nil
	}

	identNode := doc.Tree.Nodes[nodeID]

	if identNode.Kind != ast.KindIdent && identNode.Kind != ast.KindVararg {
		return nil
	}

	identBytes := doc.nodeSource(nodeID)
	if len(identBytes) == 0 {
		return nil
	}

	identName := ast.String(identBytes)

	displayName := identName
	if displayName == "" {
		displayName = "<error>"
	}

	defID := doc.referenceAt(nodeID)
	parentID := identNode.Parent

	var (
		gKey      GlobalKey
		isProp    bool
		recDef    = ast.InvalidNode
		exportRes string
	)

	if parentID != ast.InvalidNode && int(parentID) < len(doc.Tree.Nodes) {
		pNode := doc.Tree.Nodes[parentID]

		isProp = (pNode.Kind == ast.KindMemberExpr || pNode.Kind == ast.KindMethodCall || pNode.Kind == ast.KindMethodName) && pNode.Right == nodeID
		isRecordKey := pNode.Kind == ast.KindRecordField && pNode.Left == nodeID

		if isProp {
			recID := pNode.Left

			if recID != ast.InvalidNode && int(recID) < len(doc.Tree.Nodes) {
				recNode := doc.Tree.Nodes[recID]

				if recNode.Start <= identNode.End && identNode.End <= uint32(len(doc.Source())) {
					displayName = ast.String(doc.Source()[recNode.Start:identNode.End])
				}

				if recNode.Start <= recNode.End && recNode.End <= uint32(len(doc.Source())) {
					recBytes := doc.Source()[recNode.Start:recNode.End]
					gKey = GlobalKey{ReceiverHash: ast.HashBytes(recBytes), PropHash: ast.HashBytes(identBytes)}
				}

				curr := recID

				for curr != ast.InvalidNode && int(curr) < len(doc.Tree.Nodes) {
					n := doc.Tree.Nodes[curr]

					if n.Kind == ast.KindIdent {
						recDef = doc.referenceAt(curr)

						break
					} else if n.Kind == ast.KindMemberExpr {
						curr = n.Left
					} else {
						break
					}
				}

				var modName string

				if recDef != ast.InvalidNode {
					valID := doc.getAssignedValue(recDef)
					modName = s.getRequireModName(doc, valID)
				} else {
					modName = s.getRequireModName(doc, recID)
				}

				if modName != "" {
					targetDoc := s.resolveModule(uri, modName)
					if targetDoc != nil {
						gKey.ReceiverHash = ast.HashBytesConcat([]byte("module:"), nil, []byte(targetDoc.URI))
					}
				}

				exportRes = s.getFiveMExportResource(doc, recID)
			}
		} else if isRecordKey {
			isProp = true

			gKey = GlobalKey{ReceiverHash: 0, PropHash: 0}
		} else {
			gKey = GlobalKey{ReceiverHash: 0, PropHash: ast.HashBytes(identBytes)}

			if defID == ast.InvalidNode && (pNode.Kind == ast.KindNameList || pNode.Kind == ast.KindFunctionExpr || pNode.Kind == ast.KindForNum || pNode.Kind == ast.KindLocalFunction || pNode.Kind == ast.KindFunctionStmt) {
				defID = nodeID
			}
		}
	} else {
		gKey = GlobalKey{ReceiverHash: 0, PropHash: ast.HashBytes(identBytes)}
	}

	var isModuleAccess bool

	if gKey.ReceiverHash != 0 {
		if recDef != ast.InvalidNode {
			valID := doc.getAssignedValue(recDef)

			if s.getRequireModName(doc, valID) != "" {
				isModuleAccess = true
			}
		} else if isProp {
			if s.getRequireModName(doc, doc.Tree.Nodes[parentID].Left) != "" {
				isModuleAccess = true
			}
		}
	}

	isGlobal := (defID == ast.InvalidNode && recDef == ast.InvalidNode && (!isProp || gKey.ReceiverHash != 0)) || isModuleAccess

	if defID == ast.InvalidNode && identName == "self" {
		isGlobal = false

		for name, id := range doc.LocalsAt(identNode.Start) {
			if bytes.Equal(name, []byte("self")) {
				defID = id

				break
			}
		}
	}

	ctx := &SymbolContext{
		TargetDoc:      doc,
		TargetURI:      uri,
		IdentNodeID:    nodeID,
		IdentName:      identName,
		DisplayName:    displayName,
		IsProp:         isProp,
		GKey:           gKey,
		IsGlobal:       isGlobal,
		RecDefID:       recDef,
		FiveMExportRes: exportRes,
	}
	if defID != ast.InvalidNode {
		ctx.TargetDefID = defID

		if !ctx.IsGlobal && ctx.TargetDoc != nil {
			for _, exp := range ctx.TargetDoc.ExportedGlobalDefs {
				if exp.NodeID == defID {
					ctx.IsGlobal = true
					ctx.GKey = exp.Key

					if gSyms, ok := s.getGlobalSymbols(doc, ctx.GKey.ReceiverHash, ctx.GKey.PropHash); ok {
						bestDefs := s.getBestDefsForContext(ctx, doc, nodeID, gSyms)

						ctx.GlobalDefs = bestDefs
					}

					break
				}
			}
		}
	} else if isProp && exportRes == "" {
		pID := doc.Tree.Nodes[nodeID].Parent
		if pID != ast.InvalidNode && int(pID) < len(doc.Tree.Nodes) {
			pNode := doc.Tree.Nodes[pID]

			var propType TypeSet

			switch pNode.Kind {
			case ast.KindMemberExpr:
				if int(pID) < len(doc.Inferring) && doc.Inferring[pID] {
					break
				}

				propType = doc.InferType(pID)
			case ast.KindMethodCall, ast.KindMethodName:
				if int(pID) < len(doc.Inferring) && doc.Inferring[pID] {
					break
				}

				propType = doc.InferType(pID)
			}

			if propType.DeclNode != ast.InvalidNode && propType.DeclURI != "" {
				if targetDoc, ok := s.Documents[propType.DeclURI]; ok {
					ctx.TargetDoc = targetDoc
					ctx.TargetURI = propType.DeclURI

					defForVal := targetDoc.getDefForValue(propType.DeclNode)
					if defForVal != ast.InvalidNode {
						ctx.TargetDefID = defForVal
					} else {
						ctx.TargetDefID = propType.DeclNode
					}
				}
			}
		}
	}

	if ctx.TargetDefID == ast.InvalidNode && gKey.PropHash != 0 {
		var resolved bool
		var scopedClassHierarchy bool

		if ctx.FiveMExportRes != "" {
			if resObj := s.resolveFiveMResource(ctx.FiveMExportRes); resObj != nil {
				if resDefs, isExported := s.getFiveMResourceExportDefinitions(resObj, identName); isExported {
					if len(resDefs) > 0 {
						bestDefs := s.getBestDefsForContext(ctx, doc, nodeID, resDefs)
						ctx.GlobalDefs = bestDefs

						if len(bestDefs) > 0 {
							if gDoc, docOk := s.Documents[bestDefs[0].URI]; docOk {
								ctx.TargetDoc = gDoc
								ctx.TargetDefID = bestDefs[0].NodeID
								ctx.TargetURI = bestDefs[0].URI

								resolved = true
							}
						}
					}
				}
			}
		}

		if !resolved {
			if isProp && parentID != ast.InvalidNode && int(parentID) < len(doc.Tree.Nodes) {
				receiverType := TypeSet{}
				if receiver != nil && receiver.Known {
					receiverType = receiver.Type
				} else {
					receiverType = doc.InferType(doc.Tree.Nodes[parentID].Left)
					if receiver != nil {
						receiver.Type = receiverType
						receiver.Known = true
					}
				}
				if receiver != nil && receiver.NodeID != ast.InvalidNode && int(receiver.NodeID) < len(doc.Tree.Nodes) && doc.Tree.Nodes[receiver.NodeID].Kind == ast.KindIdent && recDef != ast.InvalidNode && doc.referenceAt(receiver.NodeID) == recDef && !bytes.Equal(doc.nodeSource(receiver.NodeID), []byte("self")) {
					receiver.CanReuseDefinitionType = true
				}
				if receiverType.DeclNode != ast.InvalidNode {
					declDoc := doc
					if receiverType.DeclURI != "" && receiverType.DeclURI != doc.URI {
						declDoc = s.Documents[receiverType.DeclURI]
					}
					if declDoc != nil && int(receiverType.DeclNode) < len(declDoc.Tree.Nodes) {
						if value := declDoc.findFieldInTable(receiverType.DeclNode, identName); value != ast.InvalidNode && declDoc.Tree.Nodes[value].Kind != ast.KindNil {
							parent := declDoc.Tree.Nodes[value].Parent
							if parent != ast.InvalidNode && int(parent) < len(declDoc.Tree.Nodes) {
								ctx.TargetDoc = declDoc
								ctx.TargetURI = declDoc.URI
								ctx.TargetDefID = declDoc.Tree.Nodes[parent].Left
								resolved = true
							}
						}
					}
				}
				if targetDoc, targetNode := s.findMetatableMember(doc, receiverType, identName, identNode.Start); !resolved && targetDoc != nil {
					ctx.TargetDoc = targetDoc
					ctx.TargetURI = targetDoc.URI
					ctx.TargetDefID = targetNode
					resolved = true
				}
				if !resolved {
					if targetDoc, targetNode, _ := doc.newIndexWrite(receiverType, identName, identNode.Start); targetDoc != nil && targetNode != ast.InvalidNode {
						ctx.TargetDoc = targetDoc
						ctx.TargetURI = targetDoc.URI
						ctx.TargetDefID = targetNode
						resolved = true
					}
				}
				if !resolved {
					if targetDoc, targetNode := s.uniqueVisibleNewIndexWrite(doc, receiverType, identName); targetDoc != nil && targetNode != ast.InvalidNode {
						ctx.TargetDoc = targetDoc
						ctx.TargetURI = targetDoc.URI
						ctx.TargetDefID = targetNode
						resolved = true
					}
				}
				if !resolved && s.GlobalIndex != nil && receiverType.CustomName != "" {
					resource, scope := s.globalIndexContext(doc)
					className := luaDocGenericClassName(receiverType.CustomName)
					hierarchy := s.GlobalIndex.LookupLuaDocClassHierarchy(resource, scope, className)
					fields := s.luaDocClassMemberCandidates(resource, scope, className, identName, hierarchy)
					reachable := s.luaDocClassFieldReachability(doc, hierarchy)
					scopedClassHierarchy = len(hierarchy) > 0
					for _, candidate := range fields {
						entry := candidate.Entry
						if targetDoc := s.Documents[entry.URI]; targetDoc != nil && int(entry.NodeID) < len(targetDoc.Tree.Nodes) && s.canSeeLuaDocClassFieldLookup(targetDoc, candidate, reachable) {
							ctx.TargetDoc = targetDoc
							ctx.TargetURI = targetDoc.URI
							ctx.TargetDefID = entry.NodeID
							resolved = true
							break
						}
					}
				}
			}
		}

		if !resolved && !scopedClassHierarchy {
			recHashes := []uint64{gKey.ReceiverHash}

			if isProp && parentID != ast.InvalidNode && int(parentID) < len(doc.Tree.Nodes) {
				var recType TypeSet
				if receiver != nil && receiver.Known && receiver.CanReuseDefinitionType {
					recType = receiver.Type
				} else if recDef != ast.InvalidNode {
					recType = doc.InferType(recDef)
				} else {
					recID := doc.Tree.Nodes[parentID].Left
					if recID != ast.InvalidNode {
						recType = doc.InferType(recID)
					}
				}

				if recType.CustomName != "" {
					classQueue := []string{luaDocGenericClassName(recType.CustomName)}
					seenClasses := make(map[string]struct{})
					for len(classQueue) > 0 {
						className := classQueue[0]
						classQueue = classQueue[1:]
						if className == "" {
							continue
						}
						if _, ok := seenClasses[className]; ok {
							continue
						}
						seenClasses[className] = struct{}{}
						classHash := ast.HashBytes([]byte(className))
						recHashes = appendUniqueHash(recHashes, classHash)
						if s.TableAliases != nil {
							if tableHash := s.TableAliases[classHash]; tableHash != 0 {
								recHashes = appendUniqueHash(recHashes, tableHash)
							}
						}
						entry := s.inferFirstGlobalSymbolEntry(doc, 0, classHash)
						if entry != nil {
							classQueue = append(classQueue, symbolParents(entry)...)
						}
					}
				}
			}

			for _, recHash := range recHashes {
				if recHash == 0 && gKey.ReceiverHash != 0 {
					continue
				}

				if gSyms, ok := s.getGlobalSymbols(doc, recHash, gKey.PropHash); ok {
					bestDefs := s.getBestDefsForContext(ctx, doc, nodeID, gSyms)

					ctx.GlobalDefs = bestDefs

					if len(bestDefs) > 0 {
						if gDoc, docOk := s.Documents[bestDefs[0].URI]; docOk {
							ctx.TargetDoc = gDoc
							ctx.TargetDefID = bestDefs[0].NodeID
							ctx.TargetURI = bestDefs[0].URI
						}
					}

					break
				}
			}
		}
	}

	return ctx
}

// findMetatableMember follows table-valued __index entries, stopping when a
// table repeats. It returns the key node so definition and hover can use the
// actual declaration location.
func (s *Server) findMetatableMember(srcDoc *Document, receiverType TypeSet, name string, offset uint32) (*Document, ast.NodeID) {
	if s == nil || srcDoc == nil || name == "" || receiverType.MetaNode == ast.InvalidNode {
		return nil, ast.InvalidNode
	}
	type tableKey struct {
		uri  string
		node ast.NodeID
	}
	seen := make(map[tableKey]struct{})
	var visit func(*Document, ast.NodeID) (*Document, ast.NodeID)
	visit = func(metaDoc *Document, metaNode ast.NodeID) (*Document, ast.NodeID) {
		if metaDoc == nil || metaNode == ast.InvalidNode || int(metaNode) >= len(metaDoc.Tree.Nodes) {
			return nil, ast.InvalidNode
		}
		key := tableKey{uri: metaDoc.URI, node: metaNode}
		if _, ok := seen[key]; ok {
			return nil, ast.InvalidNode
		}
		seen[key] = struct{}{}
		lookupOffset := ^uint32(0)
		if metaDoc == srcDoc {
			lookupOffset = offset
		}
		if alternatives := metaDoc.metatableFieldAlternativesAt(metaNode, "__index", lookupOffset); len(alternatives) > 1 {
			return nil, ast.InvalidNode
		}
		indexType := metaDoc.completionIndexTypeAt(metaNode, lookupOffset)
		indexDoc := metaDoc
		if indexType.DeclURI != "" && indexType.DeclURI != metaDoc.URI {
			indexDoc = s.Documents[indexType.DeclURI]
		}
		indexTable := indexType.DeclNode
		if indexDoc == nil || indexTable == ast.InvalidNode || int(indexTable) >= len(indexDoc.Tree.Nodes) {
			return nil, ast.InvalidNode
		}
		indexKey := tableKey{uri: indexDoc.URI, node: indexTable}
		if _, ok := seen[indexKey]; ok {
			return nil, ast.InvalidNode
		}
		seen[indexKey] = struct{}{}
		lookupOffset = metatableOffsetForDocument(srcDoc, indexDoc, offset)
		valueAlternatives := indexDoc.metatableFieldAlternativesAt(indexTable, name, lookupOffset)
		if len(valueAlternatives) > 1 {
			return nil, ast.InvalidNode
		}
		valueID := indexDoc.metatableFieldValueAt(indexTable, name, lookupOffset)
		if valueID != ast.InvalidNode && indexDoc.Tree.Nodes[valueID].Kind != ast.KindNil {
			if definition := indexDoc.getDefForValue(valueID); definition != ast.InvalidNode {
				return indexDoc, definition
			}
			fieldID := indexDoc.Tree.Nodes[valueID].Parent
			if fieldID != ast.InvalidNode && int(fieldID) < len(indexDoc.Tree.Nodes) {
				field := indexDoc.Tree.Nodes[fieldID]
				if (field.Kind == ast.KindRecordField || field.Kind == ast.KindIndexField) && field.Right == valueID {
					return indexDoc, field.Left
				}
			}
		}
		// An __index table can itself inherit from another table.
		if indexType.MetaNode != ast.InvalidNode {
			indexMetaDoc := metaDoc
			if indexType.MetaURI != "" && indexType.MetaURI != metaDoc.URI {
				indexMetaDoc = s.Documents[indexType.MetaURI]
			}
			return visit(indexMetaDoc, indexType.MetaNode)
		}
		return nil, ast.InvalidNode
	}
	metaDoc := srcDoc
	if receiverType.MetaURI != "" && receiverType.MetaURI != srcDoc.URI {
		metaDoc = s.Documents[receiverType.MetaURI]
	}
	return visit(metaDoc, receiverType.MetaNode)
}

func (s *Server) getFiveMExportResource(doc *Document, nodeID ast.NodeID) string {
	if doc == nil || nodeID == ast.InvalidNode || int(nodeID) >= len(doc.Tree.Nodes) {
		return ""
	}

	if !s.hasFiveMExportBridge(doc) {
		return ""
	}

	node := doc.Tree.Nodes[nodeID]

	switch node.Kind {
	case ast.KindString:
		pID := node.Parent
		if pID != ast.InvalidNode && int(pID) < len(doc.Tree.Nodes) {
			pNode := doc.Tree.Nodes[pID]
			if pNode.Kind == ast.KindIndexExpr && pNode.Right == nodeID {
				node = pNode
				nodeID = pID
			}
		}
	case ast.KindIdent:
		pID := node.Parent
		if pID != ast.InvalidNode && int(pID) < len(doc.Tree.Nodes) {
			pNode := doc.Tree.Nodes[pID]
			if pNode.Kind == ast.KindMemberExpr && pNode.Right == nodeID {
				node = pNode
				nodeID = pID
			}
		}
	}

	switch node.Kind {
	case ast.KindIndexExpr:
		if int(node.Left) < len(doc.Tree.Nodes) && doc.Tree.Nodes[node.Left].Kind == ast.KindIdent {
			leftNode := doc.Tree.Nodes[node.Left]
			if leftNode.Start <= leftNode.End && leftNode.End <= uint32(len(doc.Source())) {
				leftName := doc.Source()[leftNode.Start:leftNode.End]

				if bytes.Equal(leftName, []byte("exports")) && doc.referenceAt(node.Left) == ast.InvalidNode {
					if int(node.Right) < len(doc.Tree.Nodes) {
						rightNode := doc.Tree.Nodes[node.Right]

						if rightNode.Kind == ast.KindString {
							if rightNode.Start <= rightNode.End && rightNode.End <= uint32(len(doc.Source())) {
								return strings.ToLower(unquoteLuaString(string(doc.Source()[rightNode.Start:rightNode.End])))
							}
						}
					}
				}
			}
		}
	case ast.KindMemberExpr:
		if int(node.Left) < len(doc.Tree.Nodes) && doc.Tree.Nodes[node.Left].Kind == ast.KindIdent {
			leftNode := doc.Tree.Nodes[node.Left]
			if leftNode.Start <= leftNode.End && leftNode.End <= uint32(len(doc.Source())) {
				leftName := doc.Source()[leftNode.Start:leftNode.End]

				if bytes.Equal(leftName, []byte("exports")) && doc.referenceAt(node.Left) == ast.InvalidNode {
					if int(node.Right) < len(doc.Tree.Nodes) {
						rightNode := doc.Tree.Nodes[node.Right]

						if rightNode.Kind == ast.KindIdent {
							if rightNode.Start <= rightNode.End && rightNode.End <= uint32(len(doc.Source())) {
								return strings.ToLower(string(doc.Source()[rightNode.Start:rightNode.End]))
							}
						}
					}
				}
			}
		}
	}

	return ""
}

func (s *Server) resolveFiveMExportResource(doc *Document, nodeID ast.NodeID) (*FiveMResource, string) {
	exportRes := s.getFiveMExportResource(doc, nodeID)
	if exportRes == "" {
		return nil, ""
	}

	return s.resolveFiveMResource(exportRes), exportRes
}

func (s *Server) suggestFiveMResourceName(name string) string {
	if s == nil || name == "" {
		return ""
	}

	lowerName := strings.ToLower(name)
	if s.resolveFiveMResource(lowerName) != nil {
		return ""
	}

	var bestMatch string

	minDist := 3

	for _, candidate := range s.getFiveMResourceNames() {
		dist := levenshteinFast(lowerName, candidate, minDist-1)
		if dist < minDist {
			minDist = dist
			bestMatch = candidate
		}
	}

	return bestMatch
}

func (s *Server) getFiveMResourceExportDefinitions(res *FiveMResource, exportName string) ([]GlobalSymbol, bool) {
	if s == nil || res == nil || exportName == "" {
		return nil, false
	}

	isExported := slices.Contains(res.ClientExports, exportName) || slices.Contains(res.ServerExports, exportName)

	var defs []GlobalSymbol
	seen := make(map[TargetKey]bool)

	for _, d := range s.Documents {
		if s.getDocResourceRoot(d) != res.RootURI {
			continue
		}

		for _, exp := range d.FiveMLuaExports {
			if exp.Name != exportName {
				continue
			}

			key := TargetKey{URI: d.URI, Def: exp.NodeID}
			if seen[key] {
				continue
			}

			seen[key] = true
			defs = append(defs, GlobalSymbol{URI: d.URI, NodeID: exp.NodeID})
			isExported = true
		}
	}

	if len(defs) == 0 && isExported {
		key := GlobalKey{ReceiverHash: 0, PropHash: ast.HashBytes([]byte(exportName))}
		if s.GlobalIndex != nil {
			for _, entry := range s.GlobalIndex.SymbolsByHash(key) {
				if entry == nil {
					continue
				}

				symDoc, ok := s.Documents[entry.URI]
				if !ok || s.getDocResourceRoot(symDoc) != res.RootURI {
					continue
				}

				target := TargetKey{URI: entry.URI, Def: entry.NodeID}
				if seen[target] {
					continue
				}

				seen[target] = true
				defs = append(defs, globalSymbolFromEntry(entry))
			}
		}

	}

	return defs, isExported
}

func (s *Server) getFiveMResourceExportNames(res *FiveMResource) []string {
	if s == nil || res == nil {
		return nil
	}

	var exports []string
	seen := make(map[string]bool)

	add := func(name string) {
		if name == "" || seen[name] {
			return
		}

		seen[name] = true
		exports = append(exports, name)
	}

	for _, name := range res.ClientExports {
		add(name)
	}

	for _, name := range res.ServerExports {
		add(name)
	}

	for _, d := range s.Documents {
		if s.getDocResourceRoot(d) != res.RootURI {
			continue
		}

		for _, exp := range d.FiveMLuaExports {
			add(exp.Name)
		}
	}

	return exports
}

func (s *Server) getBestDefsForContext(ctx *SymbolContext, doc *Document, identNodeID ast.NodeID, defs []GlobalSymbol) []GlobalSymbol {
	if len(defs) <= 1 {
		return defs
	}

	var (
		activeCallArgs = -1
		isMethodCall   bool
	)

	parentID := doc.Tree.Nodes[identNodeID].Parent
	if parentID != ast.InvalidNode && int(parentID) < len(doc.Tree.Nodes) {
		parentNode := doc.Tree.Nodes[parentID]
		if parentNode.Kind == ast.KindCallExpr && parentNode.Left == identNodeID {
			activeCallArgs = int(parentNode.Count)
		} else if parentNode.Kind == ast.KindMethodCall && parentNode.Right == identNodeID {
			activeCallArgs = int(parentNode.Count)
			isMethodCall = true
		} else if parentNode.Kind == ast.KindMemberExpr {
			grandParentID := parentNode.Parent
			if grandParentID != ast.InvalidNode && int(grandParentID) < len(doc.Tree.Nodes) {
				grandParentNode := doc.Tree.Nodes[grandParentID]
				if grandParentNode.Kind == ast.KindCallExpr && grandParentNode.Left == parentID {
					activeCallArgs = int(grandParentNode.Count)
				}
			}
		}
	}

	if activeCallArgs >= 0 {
		var (
			bestDefs  []GlobalSymbol
			bestScore = -1
		)

		for _, def := range defs {
			tDoc := s.Documents[def.URI]
			if tDoc == nil {
				continue
			}

			valID := tDoc.getAssignedValue(def.NodeID)
			if valID != ast.InvalidNode && int(valID) < len(tDoc.Tree.Nodes) && tDoc.Tree.Nodes[valID].Kind == ast.KindFunctionExpr {
				funcNode := tDoc.Tree.Nodes[valID]

				var paramOffset int

				if isMethodCall {
					paramOffset = getImplicitSelfOffset(ctx, ast.Node{Kind: ast.KindMethodCall}, tDoc, def.NodeID)
				} else {
					paramOffset = getImplicitSelfOffset(ctx, ast.Node{Kind: ast.KindCallExpr}, tDoc, def.NodeID)
				}

				var (
					expectedArgs int
					hasVararg    bool
				)

				if funcNode.Count > 0 {
					expectedArgs = int(funcNode.Count) - paramOffset

					lastParamID := tDoc.Tree.ExtraList[funcNode.Extra+uint32(funcNode.Count-1)]
					if tDoc.Tree.Nodes[lastParamID].Kind == ast.KindVararg {
						hasVararg = true
					}
				} else {
					luadoc := tDoc.GetLuaDoc(def.NodeID)

					if luadoc != nil {
						expectedArgs = len(luadoc.Params) - paramOffset

						for _, p := range luadoc.Params {
							if p.Name == "..." {
								hasVararg = true

								break
							}
						}
					} else {
						expectedArgs = -paramOffset
					}
				}

				if expectedArgs < 0 {
					expectedArgs = 0
				}

				var score int

				if expectedArgs == activeCallArgs {
					score = 2
				} else if expectedArgs > activeCallArgs {
					score = 1
				}

				if hasVararg {
					if activeCallArgs >= expectedArgs-1 {
						score = 2
					}
				}

				if score > bestScore {
					bestScore = score
					bestDefs = []GlobalSymbol{def}
				} else if score == bestScore {
					bestDefs = append(bestDefs, def)
				}
			}
		}

		if len(bestDefs) > 0 {
			return bestDefs
		}
	}

	return defs
}

func (s *Server) getReferences(ctx *SymbolContext, includeDeclaration bool) []Location {
	var locations []Location

	seen := make(map[RefKey]bool)

	addRef := func(dDoc *Document, dUri string, nodeID ast.NodeID) {
		if strings.HasPrefix(dUri, embeddedStdlibURIPrefix) {
			return
		}

		if !includeDeclaration && dUri == ctx.TargetURI && nodeID == ctx.TargetDefID {
			return
		}

		if nodeID == ast.InvalidNode || int(nodeID) >= len(dDoc.Tree.Nodes) {
			return
		}

		rk := RefKey{URI: dUri, ID: nodeID}

		if seen[rk] {
			return
		}

		seen[rk] = true

		node := dDoc.Tree.Nodes[nodeID]

		startLine, startCol := dDoc.Tree.Position(node.Start)
		endLine, endCol := dDoc.Tree.Position(node.End)

		locations = append(locations, Location{
			URI: dUri,
			Range: Range{
				Start: Position{Line: startLine, Character: startCol},
				End:   Position{Line: endLine, Character: endCol},
			},
		})
	}

	if ctx.TargetDefID != ast.InvalidNode {
		for i, def := range ctx.TargetDoc.Resolver.References {
			if def == ctx.TargetDefID {
				addRef(ctx.TargetDoc, ctx.TargetURI, ast.NodeID(i))
			}
		}
		// Resolver references do not bind accesses supplied by a metatable.
		// Re-resolve member accesses against the declaration and include only
		// those that land on this exact key node.
		if ctx.TargetDefID < ast.NodeID(len(ctx.TargetDoc.Tree.Nodes)) {
			target := ctx.TargetDoc.Tree.Nodes[ctx.TargetDefID]
			parentID := target.Parent
			isRecordField := false
			isMethod := false
			targetKeyID := ctx.TargetDefID
			if parentID != ast.InvalidNode && int(parentID) < len(ctx.TargetDoc.Tree.Nodes) {
				parent := ctx.TargetDoc.Tree.Nodes[parentID]
				isRecordField = (parent.Kind == ast.KindRecordField || parent.Kind == ast.KindIndexField) && parent.Left == ctx.TargetDefID
				isMethod = parent.Kind == ast.KindMethodName && parent.Right == ctx.TargetDefID
				if parent.Kind == ast.KindIndexField && parent.Right == ctx.TargetDefID && parent.Left != ast.InvalidNode && int(parent.Left) < len(ctx.TargetDoc.Tree.Nodes) && ctx.TargetDoc.Tree.Nodes[parent.Left].Kind == ast.KindString {
					isRecordField = true
					targetKeyID = parent.Left
				}
			}
			isVirtualField := target.Start == target.End
			if ctx.IsProp && (isRecordField || isVirtualField || isMethod) {
				nameBytes := []byte(ctx.IdentName)
				propHash := ast.HashBytes(nameBytes)
				for uri, candidateDoc := range s.Documents {
					hasMemberCandidate := false
					for _, field := range candidateDoc.Resolver.FieldDefs {
						if field.PropHash == propHash {
							hasMemberCandidate = true
							break
						}
					}
					if !hasMemberCandidate {
						source := candidateDoc.Source()
						hasMemberCandidate = bytes.Contains(source, nameBytes) || bytes.ContainsRune(source, '\\')
					}
					if !hasMemberCandidate {
						continue
					}
					for i := 1; i < len(candidateDoc.Tree.Nodes); i++ {
						nodeID := ast.NodeID(i)
						node := candidateDoc.Tree.Nodes[nodeID]
						if node.Kind != ast.KindMemberExpr && node.Kind != ast.KindMethodCall && node.Kind != ast.KindIndexExpr {
							continue
						}
						if node.Right == ast.InvalidNode || int(node.Right) >= len(candidateDoc.Tree.Nodes) {
							continue
						}
						keyID := node.Right
						key := candidateDoc.Tree.Nodes[keyID]
						if node.Kind == ast.KindIndexExpr {
							if key.Kind != ast.KindString {
								continue
							}
							value, ok := candidateDoc.evalNode(keyID, 0)
							if !ok || value.kind != ast.KindString || value.str != ctx.IdentName {
								continue
							}
							receiverType := candidateDoc.InferType(node.Left)
							matched := false
							if s.GlobalIndex != nil && receiverType.CustomName != "" {
								resource, scope := s.globalIndexContext(candidateDoc)
								className := luaDocGenericClassName(receiverType.CustomName)
								hierarchy := s.GlobalIndex.LookupLuaDocClassHierarchy(resource, scope, className)
								reachable := s.luaDocClassFieldReachability(candidateDoc, hierarchy)
								for _, candidate := range s.luaDocClassMemberCandidates(resource, scope, className, ctx.IdentName, hierarchy) {
									field := candidate.Entry
									if field.URI == ctx.TargetURI && field.NodeID == ctx.TargetDefID && s.canSeeLuaDocClassFieldLookup(ctx.TargetDoc, candidate, reachable) {
										matched = true
										break
									}
								}
							}
							if !matched {
								if targetDoc, targetID := s.findMetatableMember(candidateDoc, receiverType, ctx.IdentName, key.Start); targetDoc != nil && targetDoc.URI == ctx.TargetURI && (targetID == ctx.TargetDefID || targetID == targetKeyID) {
									matched = true
								}
							}
							if matched {
								addRef(candidateDoc, uri, keyID)
							}
							continue
						}
						if key.Kind != ast.KindIdent || !bytes.Equal(candidateDoc.nodeSource(keyID), nameBytes) {
							continue
						}
						resolved := s.resolveSymbolNode(uri, candidateDoc, keyID)
						if resolved != nil && resolved.TargetURI == ctx.TargetURI && resolved.TargetDefID == ctx.TargetDefID {
							addRef(candidateDoc, uri, keyID)
						}
					}
				}
			}
		}
	}

	if ctx.FiveMExportRes != "" {
		for dUri, dDoc := range s.Documents {
			if !s.hasFiveMExportBridge(dDoc) {
				continue
			}

			for i := 1; i < len(dDoc.Tree.Nodes); i++ {
				nodeID := ast.NodeID(i)
				node := dDoc.Tree.Nodes[nodeID]

				if node.Kind != ast.KindMethodCall && node.Kind != ast.KindMemberExpr {
					continue
				}

				if node.Right == ast.InvalidNode || int(node.Right) >= len(dDoc.Tree.Nodes) {
					continue
				}

				rightNode := dDoc.Tree.Nodes[node.Right]
				if rightNode.Start > rightNode.End || rightNode.End > uint32(len(dDoc.Source())) {
					continue
				}

				if ast.String(dDoc.Source()[rightNode.Start:rightNode.End]) != ctx.IdentName {
					continue
				}

				if s.getFiveMExportResource(dDoc, node.Left) == ctx.FiveMExportRes {
					addRef(dDoc, dUri, node.Right)
				}
			}
		}
	}

	for ref := range s.iterateGlobalReferences(ctx) {
		addRef(ref.Doc, ref.URI, ref.NodeID)
	}

	if locations == nil {
		locations = []Location{}
	}

	return locations
}

func (s *Server) iterateGlobalReferences(ctx *SymbolContext) iter.Seq[GlobalReference] {
	return func(yield func(GlobalReference) bool) {
		if !ctx.IsGlobal {
			return
		}

		if ctx.FiveMExportRes != "" {
			if resObj := s.resolveFiveMResource(ctx.FiveMExportRes); resObj != nil {
				for dURI, dDoc := range s.Documents {
					if !s.hasFiveMExportBridge(dDoc) {
						continue
					}

					for i := 1; i < len(dDoc.Tree.Nodes); i++ {
						node := dDoc.Tree.Nodes[i]
						if node.Kind != ast.KindMethodCall && node.Kind != ast.KindMemberExpr {
							continue
						}

						if node.Right == ast.InvalidNode || int(node.Right) >= len(dDoc.Tree.Nodes) {
							continue
						}

						right := dDoc.Tree.Nodes[node.Right]
						if right.Start > right.End || right.End > uint32(len(dDoc.Source())) {
							continue
						}

						if ast.String(dDoc.Source()[right.Start:right.End]) != ctx.IdentName {
							continue
						}

						exportRes := s.getFiveMExportResource(dDoc, node.Left)
						if exportRes == "" {
							continue
						}

						targetRes := s.resolveFiveMResource(exportRes)
						if targetRes == nil || targetRes.RootURI != resObj.RootURI {
							continue
						}

						if !yield(GlobalReference{Doc: dDoc, URI: dURI, NodeID: node.Right}) {
							return
						}
					}
				}
			}
		}

		for dUri, dDoc := range s.Documents {
			if !s.canSeeSymbol(dDoc, ctx.TargetDoc) {
				continue
			}

			src := dDoc.Source()
			if len(src) == 0 {
				continue
			}

			if ctx.GKey.ReceiverHash == 0 {
				for _, id := range dDoc.Resolver.GlobalDefs {
					if ast.HashBytes(src[dDoc.Tree.Nodes[id].Start:dDoc.Tree.Nodes[id].End]) == ctx.GKey.PropHash {
						if !yield(GlobalReference{Doc: dDoc, URI: dUri, NodeID: id}) {
							return
						}
					}
				}

				for _, id := range dDoc.Resolver.GlobalRefs {
					if ast.HashBytes(src[dDoc.Tree.Nodes[id].Start:dDoc.Tree.Nodes[id].End]) == ctx.GKey.PropHash {
						if dDoc.referenceAt(id) == ast.InvalidNode {
							if !yield(GlobalReference{Doc: dDoc, URI: dUri, NodeID: id}) {
								return
							}
						}
					}
				}
			} else {
				for _, fd := range dDoc.Resolver.FieldDefs {
					if fd.ReceiverHash == ctx.GKey.ReceiverHash && fd.PropHash == ctx.GKey.PropHash {
						if !yield(GlobalReference{Doc: dDoc, URI: dUri, NodeID: fd.NodeID}) {
							return
						}
					}
				}

				for _, pf := range dDoc.Resolver.PendingFields {
					if pf.ReceiverHash == ctx.GKey.ReceiverHash && pf.PropHash == ctx.GKey.PropHash {
						if dDoc.referenceAt(pf.PropNodeID) == ast.InvalidNode {
							if !yield(GlobalReference{Doc: dDoc, URI: dUri, NodeID: pf.PropNodeID}) {
								return
							}
						}
					}
				}
			}
		}
	}
}

func (s *Server) getGlobalSymbols(srcDoc *Document, recHash, propHash uint64) ([]GlobalSymbol, bool) {
	queue := []uint64{recHash}
	seenReceivers := make(map[uint64]struct{})

	for len(queue) > 0 {
		currRec := queue[0]
		queue = queue[1:]
		if _, seen := seenReceivers[currRec]; seen {
			continue
		}
		seenReceivers[currRec] = struct{}{}

		key := GlobalKey{ReceiverHash: currRec, PropHash: propHash}
		if syms := s.visibleGlobalSymbolsFromEntries(srcDoc, s.GlobalIndex.SymbolsByHash(key), 10); len(syms) > 0 {
			return syms, true
		}

		if currRec == 0 {
			continue
		}

		classKey := GlobalKey{ReceiverHash: 0, PropHash: currRec}
		classEntries := s.GlobalIndex.SymbolsByHash(classKey)
		if classSyms := s.visibleGlobalSymbolsFromEntries(srcDoc, classEntries, 1); len(classSyms) > 0 {
			var parentEntry *SymbolEntry
			for _, entry := range classEntries {
				if entry != nil && entry.URI == classSyms[0].URI && entry.NodeID == classSyms[0].NodeID {
					parentEntry = entry
					break
				}
			}
			parents := symbolParents(parentEntry)
			if len(parents) > 0 {
				for _, parent := range parents {
					queue = append(queue, ast.HashBytes([]byte(parent)))
				}
				continue
			}
		}

		if nextRec := s.getGlobalAlias(srcDoc, currRec); nextRec != 0 {
			queue = append(queue, nextRec)
			continue
		}
	}

	return nil, false
}

func (s *Server) canSeeLibrarySymbol(srcDoc, tgtDoc *Document) bool {
	if tgtDoc == nil {
		return false
	}

	if srcDoc == nil {
		return true
	}

	if !strings.HasPrefix(tgtDoc.URI, embeddedStdlibURIPrefix) {
		return true
	}

	name := strings.TrimPrefix(tgtDoc.URI, embeddedStdlibURIPrefix)
	if strings.HasPrefix(name, "natives_") {
		profile := s.getDocumentFiveMProfile(srcDoc)
		switch {
		case strings.HasSuffix(name, "_shared.lua"):
			return true
		case strings.HasSuffix(name, "_client.lua"):
			return profile.Kind == FiveMProfileClient || profile.Kind == FiveMProfileShared
		case strings.HasSuffix(name, "_server.lua"):
			return profile.Kind == FiveMProfileServer || profile.Kind == FiveMProfileShared
		default:
			return false
		}
	}

	profile := s.getDocumentFiveMProfile(srcDoc)
	if !profile.IsResourceProfile() {
		return name != "fivem_client.lua" && name != "fivem_server.lua" && name != "io.lua" && name != "os.lua"
	}

	switch name {
	case "fivem_shared.lua":
		return true
	case "fivem_client.lua":
		return profile.Kind == FiveMProfileClient
	case "fivem_server.lua", "io.lua", "os.lua":
		return profile.Kind == FiveMProfileServer
	}

	return true
}

func (s *Server) globalSymbolPriority(uri string) int {
	if doc, ok := s.Documents[uri]; ok && !doc.IsLibrary {
		return 0
	}

	return 2
}

func (s *Server) setGlobalSymbol(key GlobalKey, uri string, nodeID ast.NodeID, name, parent string, isRoot bool, isDep bool, depMsg string) {
	if doc, ok := s.Documents[uri]; ok {
		doc.ExportedGlobalDefs = append(doc.ExportedGlobalDefs, ExportedSymbol{
			NodeID:        nodeID,
			Key:           key,
			IsRoot:        isRoot,
			IsDeprecated:  isDep,
			DeprecatedMsg: depMsg,
		})
	}

	if s.GlobalIndex != nil {
		resource := ResourceURI(uri)
		scope := GlobalIndexScopeShared
		if doc, ok := s.Documents[uri]; ok {
			resource, scope = s.globalIndexContext(doc)
			if resource == "" {
				resource = ResourceURI(uri)
			}
		}

		s.setGlobalIndexSymbol(resource, scope, SymbolName(name), &SymbolEntry{
			Name:          SymbolName(name),
			Key:           key,
			URI:           uri,
			NodeID:        nodeID,
			Parent:        parent,
			IsRoot:        isRoot,
			IsDeprecated:  isDep,
			DeprecatedMsg: depMsg,
		})
	}
}

func (s *Server) removeDocumentGlobals(uri string) {
	s.removeGlobalIndexDocumentSymbols(uri)
	s.removeTableAliasesForDocument(uri)
}

func (s *Server) setTableAlias(uri string, typeHash, tablePathHash uint64) {
	if s == nil || uri == "" || typeHash == 0 || tablePathHash == 0 {
		return
	}

	if s.TableAliases == nil {
		s.TableAliases = make(map[uint64]uint64)
	}
	if s.TableAliasSources == nil {
		s.TableAliasSources = make(map[uint64]map[string]uint64)
	}

	sources := s.TableAliasSources[typeHash]
	if sources == nil {
		sources = make(map[string]uint64)
		s.TableAliasSources[typeHash] = sources
	}

	sources[uri] = tablePathHash
	s.TableAliases[typeHash] = tablePathHash
}

func (s *Server) removeTableAliasesForDocument(uri string) {
	if s == nil || uri == "" || len(s.TableAliasSources) == 0 {
		return
	}

	for typeHash, sources := range s.TableAliasSources {
		delete(sources, uri)

		if len(sources) == 0 {
			delete(s.TableAliasSources, typeHash)
			delete(s.TableAliases, typeHash)

			continue
		}

		for _, tablePathHash := range sources {
			s.TableAliases[typeHash] = tablePathHash

			break
		}
	}
}

func (s *Server) removeGlobalIndexDocumentSymbols(uri string) {
	if s == nil || s.GlobalIndex == nil || uri == "" {
		return
	}

	s.GlobalIndex.mu.Lock()
	defer s.GlobalIndex.mu.Unlock()

	for _, res := range s.GlobalIndex.Resources {
		if res == nil {
			continue
		}
		for scope, names := range res.Candidates {
			for name, candidates := range names {
				for i := 0; i < len(candidates); {
					entry := candidates[i]
					if entry != nil && (entry.URI == uri || (entry.Export != nil && string(entry.Export.SourceURI) == uri)) {
						s.GlobalIndex.removeCandidateLocked(res, scope, name, entry)
						candidates = names[name]
					} else {
						i++
					}
				}
			}
		}
		// Preserve entries written through the legacy public tables while
		// removing their hash references.
		for _, table := range []SymbolTable{res.Client, res.Server, res.Shared} {
			for name, entry := range table {
				if entry != nil && entry.URI == uri {
					delete(table, name)
					s.GlobalIndex.removeEntryFromHashLocked(entry)
				}
			}
		}
	}
}

func (s *Server) getGlobalAlias(srcDoc *Document, hash uint64) uint64 {
	key := GlobalKey{ReceiverHash: 0, PropHash: hash}
	if entries := s.GlobalIndex.SymbolsByHash(key); len(entries) > 0 {
		for _, symbol := range s.visibleGlobalSymbolsFromEntries(srcDoc, entries, 0) {
			if alias := s.getGlobalAliasFromSymbol(symbol); alias != 0 {
				return alias
			}
		}
	}

	return 0
}

func (s *Server) getGlobalAliasFromSymbol(sym GlobalSymbol) uint64 {
	doc, ok := s.Documents[sym.URI]
	if !ok {
		return 0
	}

	valID := doc.getAssignedValue(sym.NodeID)
	if valID == ast.InvalidNode {
		return 0
	}

	node := doc.Tree.Nodes[valID]

	if node.Kind == ast.KindIdent || node.Kind == ast.KindMemberExpr {
		if node.Kind == ast.KindIdent && doc.referenceAt(valID) != ast.InvalidNode {
			return 0
		}

		src := doc.Source()
		if len(src) == 0 {
			return 0
		}

		return ast.HashBytes(src[node.Start:node.End])
	}

	return 0
}

func (s *Server) getGlobalPath(doc *Document, id ast.NodeID, depth int) []byte {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) || depth > 10 {
		return nil
	}

	node := doc.Tree.Nodes[id]

	switch node.Kind {
	case ast.KindIdent:
		defID := doc.referenceAt(id)
		if defID == ast.InvalidNode {
			if node.Start <= node.End && node.End <= uint32(len(doc.Source())) {
				return doc.Source()[node.Start:node.End]
			}
			return nil
		}

		valID := doc.getAssignedValue(defID)
		if valID != ast.InvalidNode && valID != id {
			return s.getGlobalPath(doc, valID, depth+1)
		}

		return nil
	case ast.KindMemberExpr:
		leftPath := s.getGlobalPath(doc, node.Left, depth+1)
		if leftPath != nil {
			if node.Right == ast.InvalidNode || int(node.Right) >= len(doc.Tree.Nodes) {
				return nil
			}

			rightNode := doc.Tree.Nodes[node.Right]

			if rightNode.Start <= rightNode.End && rightNode.End <= uint32(len(doc.Source())) {
				rightBytes := doc.Source()[rightNode.Start:rightNode.End]

				buf := make([]byte, 0, len(leftPath)+1+len(rightBytes))
				buf = append(buf, leftPath...)
				buf = append(buf, '.')
				buf = append(buf, rightBytes...)

				return buf
			}
		}
	}

	return nil
}

func (s *Server) suggestGlobal(srcDoc *Document, name string) string {
	if s.GlobalIndex != nil {
		resource, scope := s.globalIndexContext(srcDoc)
		for _, suggestion := range s.GlobalIndex.TypoSuggestions(SymbolName(name), resource, scope, 1) {
			if suggestion != "" {
				return string(suggestion)
			}
		}
	}

	var (
		bestMatch string
		minDist   = 3
	)

	nameLen := len(name)

	check := func(candidate string) {
		if candidate == "" {
			return
		}

		candLen := len(candidate)

		diff := candLen - nameLen
		if diff < 0 {
			diff = -diff
		}

		if diff > 3 {
			return
		}

		d := levenshteinFast(name, candidate, minDist-1)
		if d < minDist {
			minDist = d
			bestMatch = candidate
		}
	}

	// Prioritize known globals
	for k := range s.KnownGlobals {
		check(k)
	}

	return bestMatch
}

// uniqueVisibleNewIndexWrite resolves a redirected write from another visible
// document only when exactly one matching assignment exists. Cross-document
// offsets do not define execution order, so multiple candidates stay unresolved.
func (s *Server) uniqueVisibleNewIndexWrite(srcDoc *Document, receiverType TypeSet, name string) (*Document, ast.NodeID) {
	if s == nil || srcDoc == nil {
		return nil, ast.InvalidNode
	}
	candidates := s.newIndexWriteCandidates(name)
	var targetDoc *Document
	var targetNode ast.NodeID = ast.InvalidNode
	seen := make(map[string]map[ast.NodeID]struct{})
	for _, candidate := range candidates {
		candidateDoc := candidate.doc
		if candidateDoc == nil || candidateDoc == srcDoc || !s.canSeeSymbol(srcDoc, candidateDoc) {
			continue
		}
		writeDoc, writeNode, _ := candidateDoc.newIndexWrite(receiverType, name, candidate.assignStart+1)
		if writeDoc != candidateDoc || writeNode == ast.InvalidNode {
			continue
		}
		if seen[candidateDoc.URI] == nil {
			seen[candidateDoc.URI] = make(map[ast.NodeID]struct{})
		}
		if _, ok := seen[candidateDoc.URI][writeNode]; ok {
			continue
		}
		seen[candidateDoc.URI][writeNode] = struct{}{}
		if targetDoc != nil {
			return nil, ast.InvalidNode
		}
		targetDoc, targetNode = writeDoc, writeNode
	}
	return targetDoc, targetNode
}

func (s *Server) getDocResourceRoot(doc *Document) string {
	if doc == nil {
		return ""
	}

	return s.getDocumentFiveMProfile(doc).ResourceRoot
}

func (s *Server) canSeeSymbol(srcDoc, tgtDoc *Document) bool {
	if tgtDoc == nil {
		return false
	}

	if srcDoc == tgtDoc {
		return true
	}

	if tgtDoc.IsLibrary {
		return s.canSeeLibrarySymbol(srcDoc, tgtDoc)
	}

	srcProfile := s.getDocumentFiveMProfile(srcDoc)
	tgtProfile := s.getDocumentFiveMProfile(tgtDoc)

	srcRoot := srcProfile.ResourceRoot
	tgtRoot := tgtProfile.ResourceRoot

	if srcRoot == "" && tgtRoot == "" {
		return true
	}

	if srcRoot == tgtRoot {
		if srcRoot == "" {
			return false
		}

		if srcProfile.Kind == FiveMProfilePlainLua || tgtProfile.Kind == FiveMProfilePlainLua {
			return false
		}

		if srcProfile.Kind == FiveMProfileManifest || tgtProfile.Kind == FiveMProfileManifest {
			return false
		}

		srcEnv := srcProfile.Env()
		tgtEnv := tgtProfile.Env()

		switch srcEnv {
		case EnvClient:
			return tgtEnv == EnvClient || tgtEnv == EnvShared
		case EnvServer:
			return tgtEnv == EnvServer || tgtEnv == EnvShared
		case EnvShared:
			return tgtEnv == EnvShared
		}

		return false
	}

	srcRes := s.resolveFiveMResourceByRoot(srcRoot)
	if srcRes == nil || srcProfile.Kind == FiveMProfilePlainLua || srcProfile.Kind == FiveMProfileManifest {
		return false
	}

	return s.canSeeFiveMCrossResourceInclude(srcProfile, tgtDoc)
}

func (s *Server) canSeeLuaDocClassField(srcDoc, fieldDoc *Document, path []LuaDocClassHierarchyEntry) bool {
	if s == nil || srcDoc == nil || fieldDoc == nil || len(path) == 0 {
		return false
	}
	current := srcDoc
	for _, class := range path {
		if class.Entry == nil {
			return false
		}
		classDoc := s.Documents[class.Entry.URI]
		if classDoc == nil || !s.canSeeSymbol(current, classDoc) {
			return false
		}
		current = classDoc
	}
	return s.canSeeSymbol(current, fieldDoc)
}

func (s *Server) luaDocClassTableAlias(class *SymbolEntry) (uint64, string) {
	if s == nil || class == nil || class.Name == "" || s.TableAliasSources == nil {
		return 0, ""
	}
	classDoc := s.Documents[class.URI]
	if classDoc == nil {
		return 0, ""
	}
	sources := s.TableAliasSources[ast.HashBytes([]byte(class.Name))]
	if len(sources) == 0 {
		return 0, ""
	}
	if tableHash := sources[class.URI]; tableHash != 0 {
		return tableHash, s.getDocResourceRoot(classDoc)
	}
	uris := make([]string, 0, len(sources))
	for uri := range sources {
		uris = append(uris, uri)
	}
	slices.Sort(uris)
	for _, uri := range uris {
		aliasDoc := s.Documents[uri]
		if aliasDoc != nil && s.canSeeSymbol(classDoc, aliasDoc) {
			return sources[uri], s.getDocResourceRoot(aliasDoc)
		}
	}
	return 0, ""
}

func (s *Server) luaDocClassMemberCandidates(resource ResourceURI, scope GlobalIndexScope, className, fieldName string, hierarchy []LuaDocClassHierarchyEntry) []LuaDocClassFieldLookup {
	if s == nil || s.GlobalIndex == nil || len(hierarchy) == 0 {
		return nil
	}
	candidates := s.GlobalIndex.LookupLuaDocClassFieldCandidatesInHierarchy(scope, fieldName, hierarchy)
	propHash := ast.HashBytes([]byte(fieldName))
	for i, class := range hierarchy {
		tableHash, aliasResource := s.luaDocClassTableAlias(class.Entry)
		if tableHash == 0 {
			continue
		}
		for _, entry := range s.GlobalIndex.SymbolsByHash(GlobalKey{ReceiverHash: tableHash, PropHash: propHash}) {
			if aliasResource != "" {
				entryDoc := s.Documents[entry.URI]
				if entryDoc == nil || s.getDocResourceRoot(entryDoc) != aliasResource {
					continue
				}
			}
			candidates = append(candidates, LuaDocClassFieldLookup{Entry: entry, Hierarchy: hierarchy, ClassIndex: i})
		}
	}
	return candidates
}

func (s *Server) luaDocClassFields(resource ResourceURI, scope GlobalIndexScope, className string, hierarchy []LuaDocClassHierarchyEntry) []LuaDocClassFieldLookup {
	if s == nil || s.GlobalIndex == nil || len(hierarchy) == 0 {
		return nil
	}
	fields := s.GlobalIndex.LookupLuaDocClassFieldsInHierarchy(scope, hierarchy)
	for i, class := range hierarchy {
		tableHash, aliasResource := s.luaDocClassTableAlias(class.Entry)
		if tableHash == 0 {
			continue
		}
		keys := s.GlobalIndex.FieldKeysForReceivers(map[uint64]bool{tableHash: true})
		for _, key := range keys {
			for _, entry := range s.GlobalIndex.SymbolsByHash(key) {
				if aliasResource != "" {
					entryDoc := s.Documents[entry.URI]
					if entryDoc == nil || s.getDocResourceRoot(entryDoc) != aliasResource {
						continue
					}
				}
				fields = append(fields, LuaDocClassFieldLookup{Entry: entry, Hierarchy: hierarchy, ClassIndex: i})
			}
		}
	}
	return fields
}

func (s *Server) luaDocClassFieldReachability(srcDoc *Document, hierarchy []LuaDocClassHierarchyEntry) []bool {
	if s == nil || srcDoc == nil || len(hierarchy) == 0 {
		return nil
	}
	firstDoc := s.Documents[hierarchy[0].Entry.URI]
	if firstDoc == nil || !s.canSeeSymbol(srcDoc, firstDoc) {
		return nil
	}
	reachable := make([]bool, len(hierarchy))
	queue := make([]int, 0, len(hierarchy))
	reachable[0] = true
	queue = append(queue, 0)
	for head := 0; head < len(queue); head++ {
		childIndex := queue[head]
		childDoc := s.Documents[hierarchy[childIndex].Entry.URI]
		if childDoc == nil {
			continue
		}
		parentIndex := hierarchy[childIndex].FirstParent
		if parentIndex >= 0 && !reachable[parentIndex] {
			parentDoc := s.Documents[hierarchy[parentIndex].Entry.URI]
			if parentDoc != nil && s.canSeeSymbol(childDoc, parentDoc) {
				reachable[parentIndex] = true
				queue = append(queue, parentIndex)
			}
		}
		for _, parentIndex := range hierarchy[childIndex].ParentIndices {
			if reachable[parentIndex] {
				continue
			}
			parentDoc := s.Documents[hierarchy[parentIndex].Entry.URI]
			if parentDoc != nil && s.canSeeSymbol(childDoc, parentDoc) {
				reachable[parentIndex] = true
				queue = append(queue, parentIndex)
			}
		}
	}
	return reachable
}

func (s *Server) canSeeLuaDocClassFieldLookup(fieldDoc *Document, field LuaDocClassFieldLookup, reachable []bool) bool {
	if s == nil || fieldDoc == nil || field.ClassIndex < 0 || field.ClassIndex >= len(field.Hierarchy) || field.ClassIndex >= len(reachable) || !reachable[field.ClassIndex] {
		return false
	}
	classDoc := s.Documents[field.Hierarchy[field.ClassIndex].Entry.URI]
	return classDoc != nil && s.canSeeSymbol(classDoc, fieldDoc)
}

func (s *Server) isKnownGlobal(doc *Document, name []byte) bool {
	strName := ast.String(name)

	if s.KnownGlobals[strName] {
		return true
	}

	for _, glob := range s.KnownGlobalGlobs {
		if matchGlob(glob, strName) {
			return true
		}
	}

	if s.isFiveMGlobalAvailable(doc, strName) {
		return true
	}

	return false
}
