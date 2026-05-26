package lsp

import (
	"bytes"
	"encoding/json"
	"iter"
	"slices"
	"strings"

	"github.com/coalaura/lugo/ast"
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
	if locs := s.getFiveMEventDefinitionLocations(doc, offset); len(locs) > 0 {
		WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: locs})

		return
	}

	ctx := s.resolveSymbolAt(uri, offset)

	if ctx != nil {
		var locs []Location

		if len(ctx.GlobalDefs) > 0 {
			for _, def := range ctx.GlobalDefs {
				if strings.HasPrefix(def.URI, embeddedStdlibURIPrefix) {
					continue
				}

				if tDoc, ok := s.Documents[def.URI]; ok {
					locs = append(locs, Location{
						URI:   def.URI,
						Range: getNodeRange(tDoc.Tree, def.NodeID),
					})
				}
			}
		} else if ctx.TargetDefID != ast.InvalidNode && !strings.HasPrefix(ctx.TargetURI, embeddedStdlibURIPrefix) {
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

	filtered := make([]GlobalSymbol, 0, min(len(entries), max))
	seen := make(map[TargetKey]struct{}, len(entries))

	for _, entry := range entries {
		if entry == nil {
			continue
		}

		tgtDoc, ok := s.Documents[entry.URI]
		if !ok || !s.canSeeSymbol(srcDoc, tgtDoc) {
			continue
		}

		target := TargetKey{URI: entry.URI, Def: entry.NodeID}
		if _, ok := seen[target]; ok {
			continue
		}

		seen[target] = struct{}{}
		filtered = append(filtered, globalSymbolFromEntry(entry))
	}

	slices.SortStableFunc(filtered, func(a, b GlobalSymbol) int {
		return s.globalSymbolPriority(a.URI) - s.globalSymbolPriority(b.URI)
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
	table := res.tableForScope(scope)
	if previous := table[name]; previous != nil {
		s.GlobalIndex.removeEntryFromHashLocked(previous)
	}

	entry.Name = name
	if entry.URI == "" {
		entry.URI = string(resource)
	}
	table[name] = entry

	if entry.Key == (GlobalKey{}) {
		return
	}

	entries := s.GlobalIndex.HashIndex[entry.Key]
	priority := s.globalSymbolPriority(entry.URI)
	insertAt := len(entries)
	for i, existing := range entries {
		if existing == nil {
			continue
		}
		if existing.URI == entry.URI && existing.NodeID == entry.NodeID {
			copy(entries[i:], entries[i+1:])
			entries[len(entries)-1] = nil
			entries = entries[:len(entries)-1]
			insertAt = len(entries)
			break
		}
		if s.globalSymbolPriority(existing.URI) > priority {
			insertAt = i
			break
		}
	}

	entries = append(entries, nil)
	copy(entries[insertAt+1:], entries[insertAt:])
	entries[insertAt] = entry
	s.GlobalIndex.HashIndex[entry.Key] = entries
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

		for i := uint16(0); i < node.Count; i++ {
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
			for i := uint16(0); i < node.Count; i++ {
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

				for i := uint16(0); i < lhsList.Count && i < rhsNode.Count; i++ {
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

			for i := uint16(0); i < node.Count; i++ {
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

				for i := uint16(0); i < exprList.Count; i++ {
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

			for i := uint16(0); i < node.Count; i++ {
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
			URI: "builtin://fivem/events",
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
	if nodeID == ast.InvalidNode || int(nodeID) >= len(doc.Tree.Nodes) {
		return nil
	}

	identNode := doc.Tree.Nodes[nodeID]

	if identNode.Kind != ast.KindIdent && identNode.Kind != ast.KindVararg {
		return nil
	}

	if identNode.Start > identNode.End || identNode.End > uint32(len(doc.Source())) {
		return nil
	}

	identBytes := doc.Source()[identNode.Start:identNode.End]
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
			recHashes := []uint64{gKey.ReceiverHash}

			if isProp && parentID != ast.InvalidNode && int(parentID) < len(doc.Tree.Nodes) {
				var recType TypeSet
				if recDef != ast.InvalidNode {
					recType = doc.InferType(recDef)
				} else {
					recID := doc.Tree.Nodes[parentID].Left
					if recID != ast.InvalidNode {
						recType = doc.InferType(recID)
					}
				}

				if recType.CustomName != "" {
					classHash := ast.HashBytes([]byte(recType.CustomName))
					recHashes = appendUniqueHash(recHashes, classHash)

					if s.TableAliases != nil {
						if tableHash := s.TableAliases[classHash]; tableHash != 0 {
							recHashes = appendUniqueHash(recHashes, tableHash)
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

						exportRes := s.getFiveMExportResource(dDoc, node.Left)
						if exportRes == "" {
							continue
						}

						targetRes := s.resolveFiveMResource(exportRes)
						if targetRes == nil || targetRes.RootURI != resObj.RootURI {
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
	currRec := recHash

	for range 10 {
		key := GlobalKey{ReceiverHash: currRec, PropHash: propHash}
		if syms := s.visibleGlobalSymbolsFromEntries(srcDoc, s.GlobalIndex.SymbolsByHash(key), 10); len(syms) > 0 {
			return syms, true
		}

		if currRec == 0 {
			break
		}

		classKey := GlobalKey{ReceiverHash: 0, PropHash: currRec}
		if classSyms := s.visibleGlobalSymbolsFromEntries(srcDoc, s.GlobalIndex.SymbolsByHash(classKey), 1); len(classSyms) > 0 && classSyms[0].Parent != "" {
			currRec = ast.HashBytes([]byte(classSyms[0].Parent))

			continue
		}

		nextRec := s.getGlobalAlias(currRec)
		if nextRec == 0 {
			break
		}

		currRec = nextRec
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

		for _, table := range []SymbolTable{res.Client, res.Server, res.Shared} {
			for name, entry := range table {
				if entry != nil && entry.URI == uri {
					delete(table, name)
				}
			}
		}
	}

	for key, entries := range s.GlobalIndex.HashIndex {
		kept := entries[:0]
		for _, entry := range entries {
			if entry == nil || entry.URI == uri {
				continue
			}
			kept = append(kept, entry)
		}

		if len(kept) == 0 {
			delete(s.GlobalIndex.HashIndex, key)
		} else {
			s.GlobalIndex.HashIndex[key] = kept
		}
	}
}

func (s *Server) getGlobalAlias(hash uint64) uint64 {
	key := GlobalKey{ReceiverHash: 0, PropHash: hash}
	if entries := s.GlobalIndex.SymbolsByHash(key); len(entries) > 0 {
		for _, entry := range entries {
			if alias := s.getGlobalAliasFromSymbol(globalSymbolFromEntry(entry)); alias != 0 {
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
