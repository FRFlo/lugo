package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/coalaura/lugo/ast"
	"github.com/coalaura/lugo/lexer"
	"github.com/coalaura/lugo/token"
)

const (
	debugExportCategoryTokens      = "tokens"
	debugExportCategoryIdentifiers = "identifiers"
	debugExportCategoryAST         = "ast"
	debugExportCategorySemantic    = "semantic"
	debugExportCategoryGlobalIndex = "globalIndex"
)

var debugExportDefaultCategories = []string{
	debugExportCategoryTokens,
	debugExportCategoryIdentifiers,
	debugExportCategoryAST,
	debugExportCategorySemantic,
	debugExportCategoryGlobalIndex,
}

type debugExportPayload struct {
	Metadata    debugExportMetadata       `json:"metadata"`
	Documents   []debugExportDocument     `json:"documents"`
	GlobalIndex []debugExportGlobalSymbol `json:"globalIndex,omitempty"`
}

type debugExportMetadata struct {
	ServerVersion    string   `json:"serverVersion"`
	RootURI          string   `json:"rootUri,omitempty"`
	WorkspaceFolders []string `json:"workspaceFolders,omitempty"`
	Categories       []string `json:"categories"`
	DocumentCount    int      `json:"documentCount"`
	IsIndexing       bool     `json:"isIndexing"`
}

type debugExportDocument struct {
	URI          string                      `json:"uri"`
	Path         string                      `json:"path,omitempty"`
	ModuleName   string                      `json:"moduleName,omitempty"`
	Hash         uint64                      `json:"hash"`
	SourceBytes  int                         `json:"sourceBytes"`
	LineCount    int                         `json:"lineCount"`
	Open         bool                        `json:"open"`
	IsWorkspace  bool                        `json:"isWorkspace"`
	IsLibrary    bool                        `json:"isLibrary"`
	IsMeta       bool                        `json:"isMeta"`
	IsFiveM      bool                        `json:"isFiveM"`
	FiveMProfile string                      `json:"fiveMProfile,omitempty"`
	Errors       []debugExportParseError     `json:"errors,omitempty"`
	Tokens       []debugExportToken          `json:"tokens,omitempty"`
	Identifiers  []debugExportIdentifier     `json:"identifiers,omitempty"`
	AST          []debugExportASTNode        `json:"ast,omitempty"`
	Comments     []debugExportToken          `json:"comments,omitempty"`
	Semantic     *debugExportSemanticSummary `json:"semantic,omitempty"`
}

type debugExportParseError struct {
	Message string `json:"message"`
	Start   uint32 `json:"start"`
	End     uint32 `json:"end"`
}

type debugExportToken struct {
	Index int    `json:"index"`
	Kind  string `json:"kind"`
	Start uint32 `json:"start"`
	End   uint32 `json:"end"`
	Text  string `json:"text,omitempty"`
}

type debugExportIdentifier struct {
	Index int    `json:"index"`
	Start uint32 `json:"start"`
	End   uint32 `json:"end"`
	Text  string `json:"text"`
}

type debugExportASTNode struct {
	ID     ast.NodeID `json:"id"`
	Kind   string     `json:"kind"`
	Start  uint32     `json:"start"`
	End    uint32     `json:"end"`
	Parent ast.NodeID `json:"parent,omitempty"`
	Left   ast.NodeID `json:"left,omitempty"`
	Right  ast.NodeID `json:"right,omitempty"`
	Extra  uint32     `json:"extra,omitempty"`
	Count  uint16     `json:"count,omitempty"`
	Flags  uint8      `json:"flags,omitempty"`
	Text   string     `json:"text,omitempty"`
}

type debugExportSemanticSummary struct {
	References      []debugExportNodeLink     `json:"references,omitempty"`
	GlobalRefs      []debugExportNodeRef      `json:"globalRefs,omitempty"`
	GlobalDefs      []debugExportNodeRef      `json:"globalDefs,omitempty"`
	LocalDefs       []debugExportNodeRef      `json:"localDefs,omitempty"`
	FieldDefs       []debugExportFieldDef     `json:"fieldDefs,omitempty"`
	PendingFields   []debugExportFieldRef     `json:"pendingFields,omitempty"`
	DuplicateLocals []debugExportNodeRef      `json:"duplicateLocals,omitempty"`
	ShadowedOuter   []debugExportShadowPair   `json:"shadowedOuter,omitempty"`
	Reassignments   []debugExportReassignment `json:"reassignments,omitempty"`
}

type debugExportNodeRef struct {
	Node ast.NodeID `json:"node"`
	Name string     `json:"name,omitempty"`
}

type debugExportNodeLink struct {
	Node   ast.NodeID `json:"node"`
	Target ast.NodeID `json:"target"`
	Name   string     `json:"name,omitempty"`
}

type debugExportFieldDef struct {
	Node         ast.NodeID `json:"node"`
	Name         string     `json:"name,omitempty"`
	ReceiverDef  ast.NodeID `json:"receiverDef,omitempty"`
	ReceiverName string     `json:"receiverName,omitempty"`
	ReceiverHash uint64     `json:"receiverHash,omitempty"`
	PropHash     uint64     `json:"propHash"`
}

type debugExportFieldRef struct {
	Node         ast.NodeID `json:"node"`
	Name         string     `json:"name,omitempty"`
	ReceiverDef  ast.NodeID `json:"receiverDef,omitempty"`
	ReceiverName string     `json:"receiverName,omitempty"`
	ReceiverHash uint64     `json:"receiverHash,omitempty"`
	PropHash     uint64     `json:"propHash"`
}

type debugExportShadowPair struct {
	Shadowing debugExportNodeRef `json:"shadowing"`
	Shadowed  debugExportNodeRef `json:"shadowed"`
}

type debugExportReassignment struct {
	NameHash uint64             `json:"nameHash"`
	Def      debugExportNodeRef `json:"def"`
	Value    debugExportNodeRef `json:"value"`
}

type debugExportGlobalSymbol struct {
	Resource      string     `json:"resource"`
	Name          string     `json:"name"`
	URI           string     `json:"uri"`
	NodeID        ast.NodeID `json:"nodeId"`
	Parent        string     `json:"parent,omitempty"`
	ReceiverHash  uint64     `json:"receiverHash,omitempty"`
	PropHash      uint64     `json:"propHash,omitempty"`
	IsRoot        bool       `json:"isRoot,omitempty"`
	IsDeprecated  bool       `json:"isDeprecated,omitempty"`
	DeprecatedMsg string     `json:"deprecatedMsg,omitempty"`
	HasLuaDoc     bool       `json:"hasLuaDoc,omitempty"`
	HasFiveM      bool       `json:"hasFiveM,omitempty"`
	HasExport     bool       `json:"hasExport,omitempty"`
}

func (s *Server) handleDebugExport(req Request) {
	var params DebugExportParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		WriteMessage(s.Writer, Response{
			RPC: "2.0",
			ID:  req.ID,
			Error: ResponseError{
				Code:    -32602,
				Message: fmt.Sprintf("invalid debug export params: %v", err),
			},
		})
		return
	}

	content, err := s.buildDebugExport(params)
	if err != nil {
		WriteMessage(s.Writer, Response{
			RPC: "2.0",
			ID:  req.ID,
			Error: ResponseError{
				Code:    -32603,
				Message: err.Error(),
			},
		})
		return
	}

	WriteMessage(s.Writer, Response{
		RPC:    "2.0",
		ID:     req.ID,
		Result: DebugExportResult{Content: content},
	})
}

func (s *Server) buildDebugExport(params DebugExportParams) (string, error) {
	categories := normalizeDebugExportCategories(params.Categories)
	selected := make(map[string]bool, len(categories))
	for _, category := range categories {
		selected[category] = true
	}

	docs := s.sortedDebugExportDocuments()
	payload := debugExportPayload{
		Metadata: debugExportMetadata{
			ServerVersion:    s.Version,
			RootURI:          s.RootURI,
			WorkspaceFolders: slices.Clone(s.WorkspaceFolders),
			Categories:       categories,
			DocumentCount:    len(docs),
			IsIndexing:       s.IsIndexing,
		},
		Documents: make([]debugExportDocument, 0, len(docs)),
	}

	for _, doc := range docs {
		payload.Documents = append(payload.Documents, s.exportDebugDocument(doc, selected))
	}

	if selected[debugExportCategoryGlobalIndex] {
		payload.GlobalIndex = s.exportDebugGlobalIndex()
	}

	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func normalizeDebugExportCategories(categories []string) []string {
	if len(categories) == 0 {
		return slices.Clone(debugExportDefaultCategories)
	}

	valid := map[string]bool{
		debugExportCategoryTokens:      true,
		debugExportCategoryIdentifiers: true,
		debugExportCategoryAST:         true,
		debugExportCategorySemantic:    true,
		debugExportCategoryGlobalIndex: true,
	}

	out := make([]string, 0, len(categories))
	seen := make(map[string]bool, len(categories))
	for _, category := range categories {
		if !valid[category] || seen[category] {
			continue
		}
		seen[category] = true
		out = append(out, category)
	}

	if len(out) == 0 {
		return slices.Clone(debugExportDefaultCategories)
	}

	return out
}

func (s *Server) sortedDebugExportDocuments() []*Document {
	if s == nil || len(s.Documents) == 0 {
		return nil
	}

	keys := make([]string, 0, len(s.Documents))
	for uri := range s.Documents {
		keys = append(keys, uri)
	}
	sort.Strings(keys)

	docs := make([]*Document, 0, len(keys))
	for _, uri := range keys {
		if doc := s.Documents[uri]; doc != nil {
			docs = append(docs, doc)
		}
	}

	return docs
}

func (s *Server) exportDebugDocument(doc *Document, selected map[string]bool) debugExportDocument {
	source := debugExportDocumentSource(doc)
	lineCount := 0
	if doc.Tree != nil {
		lineCount = len(doc.Tree.LineOffsets)
	}

	out := debugExportDocument{
		URI:          doc.URI,
		Path:         doc.Path,
		ModuleName:   doc.ModuleName,
		Hash:         ast.HashBytes(source),
		SourceBytes:  len(source),
		LineCount:    lineCount,
		Open:         s.OpenFiles[doc.URI],
		IsWorkspace:  doc.IsWorkspace,
		IsLibrary:    doc.IsLibrary,
		IsMeta:       doc.IsMeta,
		IsFiveM:      doc.IsFiveMManifest || doc.FiveMProfile.IsResourceProfile(),
		FiveMProfile: debugExportFiveMProfile(doc.FiveMProfile),
	}

	if len(doc.Errors) > 0 {
		out.Errors = make([]debugExportParseError, 0, len(doc.Errors))
		for _, err := range doc.Errors {
			out.Errors = append(out.Errors, debugExportParseError{Message: err.Message, Start: err.Start, End: err.End})
		}
	}

	if selected[debugExportCategoryTokens] && len(source) > 0 {
		out.Tokens = exportDebugTokens(source, false)
	}
	if selected[debugExportCategoryIdentifiers] && len(source) > 0 {
		out.Identifiers = exportDebugIdentifiers(source)
	}
	if selected[debugExportCategoryAST] && doc.Tree != nil {
		out.AST = exportDebugAST(doc, source)
		out.Comments = exportDebugComments(doc, source)
	}
	if selected[debugExportCategorySemantic] && doc.Resolver != nil {
		out.Semantic = exportDebugSemantic(doc, source)
	}

	return out
}

func debugExportDocumentSource(doc *Document) []byte {
	if source := doc.Source(); len(source) > 0 {
		return source
	}
	if doc == nil || doc.Path == "" {
		return nil
	}

	source, err := os.ReadFile(doc.Path)
	if err != nil {
		return nil
	}
	return source
}

func exportDebugTokens(source []byte, identifiersOnly bool) []debugExportToken {
	lex := lexer.New(source)
	tokens := make([]debugExportToken, 0, len(source)/4)
	for {
		tok := lex.Next()
		if tok.Kind == token.EOF {
			break
		}
		if identifiersOnly && tok.Kind != token.Ident {
			continue
		}

		tokens = append(tokens, debugExportToken{
			Index: len(tokens),
			Kind:  debugExportTokenKind(tok.Kind),
			Start: tok.Start,
			End:   tok.End,
			Text:  string(tok.Text(source)),
		})
	}

	return tokens
}

func exportDebugIdentifiers(source []byte) []debugExportIdentifier {
	tokens := exportDebugTokens(source, true)
	idents := make([]debugExportIdentifier, 0, len(tokens))
	for _, tok := range tokens {
		idents = append(idents, debugExportIdentifier{Index: len(idents), Start: tok.Start, End: tok.End, Text: tok.Text})
	}

	return idents
}

func exportDebugAST(doc *Document, source []byte) []debugExportASTNode {
	tree := doc.Tree
	if tree == nil {
		return nil
	}

	nodes := make([]debugExportASTNode, 0, len(tree.Nodes))
	for id, node := range tree.Nodes {
		entry := debugExportASTNode{
			ID:     ast.NodeID(id),
			Kind:   debugExportNodeKind(node.Kind),
			Start:  node.Start,
			End:    node.End,
			Parent: node.Parent,
			Left:   node.Left,
			Right:  node.Right,
			Extra:  node.Extra,
			Count:  node.Count,
			Flags:  node.Flags,
		}
		if node.Kind == ast.KindIdent && node.Start <= node.End && node.End <= uint32(len(source)) {
			entry.Text = string(source[node.Start:node.End])
		}
		nodes = append(nodes, entry)
	}

	return nodes
}

func exportDebugComments(doc *Document, source []byte) []debugExportToken {
	if doc.Tree == nil || len(doc.Tree.Comments) == 0 {
		return nil
	}

	comments := make([]debugExportToken, 0, len(doc.Tree.Comments))
	for _, comment := range doc.Tree.Comments {
		entry := debugExportToken{
			Index: len(comments),
			Kind:  debugExportTokenKind(comment.Kind),
			Start: comment.Start,
			End:   comment.End,
		}
		if comment.Start <= comment.End && comment.End <= uint32(len(source)) {
			entry.Text = string(comment.Text(source))
		}
		comments = append(comments, entry)
	}

	return comments
}

func exportDebugSemantic(doc *Document, source []byte) *debugExportSemanticSummary {
	r := doc.Resolver
	if r == nil {
		return nil
	}

	out := &debugExportSemanticSummary{}
	for node, target := range r.References {
		if target != ast.InvalidNode {
			out.References = append(out.References, debugExportNodeLink{Node: ast.NodeID(node), Target: target, Name: debugExportNodeName(doc, source, ast.NodeID(node))})
		}
	}
	sortDebugNodeLinks(out.References)
	out.GlobalRefs = exportDebugNodeRefs(doc, source, r.GlobalRefs)
	out.GlobalDefs = exportDebugNodeRefs(doc, source, r.GlobalDefs)
	out.LocalDefs = exportDebugNodeRefs(doc, source, r.LocalDefs)
	out.DuplicateLocals = exportDebugNodeRefs(doc, source, r.DuplicateLocals)

	for _, field := range r.FieldDefs {
		out.FieldDefs = append(out.FieldDefs, debugExportFieldDef{
			Node:         field.NodeID,
			Name:         debugExportNodeName(doc, source, field.NodeID),
			ReceiverDef:  field.ReceiverDef,
			ReceiverName: string(field.ReceiverName),
			ReceiverHash: field.ReceiverHash,
			PropHash:     field.PropHash,
		})
	}
	sortDebugFieldDefs(out.FieldDefs)

	for _, field := range r.PendingFields {
		out.PendingFields = append(out.PendingFields, debugExportFieldRef{
			Node:         field.PropNodeID,
			Name:         debugExportNodeName(doc, source, field.PropNodeID),
			ReceiverDef:  field.ReceiverDef,
			ReceiverName: string(field.ReceiverName),
			ReceiverHash: field.ReceiverHash,
			PropHash:     field.PropHash,
		})
	}
	sortDebugFieldRefs(out.PendingFields)

	for _, pair := range r.ShadowedOuter {
		out.ShadowedOuter = append(out.ShadowedOuter, debugExportShadowPair{
			Shadowing: debugExportNodeRef{Node: pair.Shadowing, Name: debugExportNodeName(doc, source, pair.Shadowing)},
			Shadowed:  debugExportNodeRef{Node: pair.Shadowed, Name: debugExportNodeName(doc, source, pair.Shadowed)},
		})
	}
	sortDebugShadowPairs(out.ShadowedOuter)

	for _, reassignment := range r.Reassignments {
		out.Reassignments = append(out.Reassignments, debugExportReassignment{
			NameHash: reassignment.NameHash,
			Def:      debugExportNodeRef{Node: reassignment.DefID, Name: debugExportNodeName(doc, source, reassignment.DefID)},
			Value:    debugExportNodeRef{Node: reassignment.ValID, Name: debugExportNodeName(doc, source, reassignment.ValID)},
		})
	}
	sortDebugReassignments(out.Reassignments)

	return out
}

func exportDebugNodeRefs(doc *Document, source []byte, ids []ast.NodeID) []debugExportNodeRef {
	if len(ids) == 0 {
		return nil
	}

	out := make([]debugExportNodeRef, 0, len(ids))
	for _, id := range ids {
		out = append(out, debugExportNodeRef{Node: id, Name: debugExportNodeName(doc, source, id)})
	}
	sortDebugNodeRefs(out)

	return out
}

func sortDebugNodeRefs(refs []debugExportNodeRef) {
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].Node != refs[j].Node {
			return refs[i].Node < refs[j].Node
		}
		return refs[i].Name < refs[j].Name
	})
}

func sortDebugNodeLinks(links []debugExportNodeLink) {
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].Node != links[j].Node {
			return links[i].Node < links[j].Node
		}
		if links[i].Target != links[j].Target {
			return links[i].Target < links[j].Target
		}
		return links[i].Name < links[j].Name
	})
}

func sortDebugFieldDefs(fields []debugExportFieldDef) {
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Node != fields[j].Node {
			return fields[i].Node < fields[j].Node
		}
		if fields[i].ReceiverDef != fields[j].ReceiverDef {
			return fields[i].ReceiverDef < fields[j].ReceiverDef
		}
		if fields[i].ReceiverHash != fields[j].ReceiverHash {
			return fields[i].ReceiverHash < fields[j].ReceiverHash
		}
		if fields[i].PropHash != fields[j].PropHash {
			return fields[i].PropHash < fields[j].PropHash
		}
		return fields[i].Name < fields[j].Name
	})
}

func sortDebugFieldRefs(fields []debugExportFieldRef) {
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Node != fields[j].Node {
			return fields[i].Node < fields[j].Node
		}
		if fields[i].ReceiverDef != fields[j].ReceiverDef {
			return fields[i].ReceiverDef < fields[j].ReceiverDef
		}
		if fields[i].ReceiverHash != fields[j].ReceiverHash {
			return fields[i].ReceiverHash < fields[j].ReceiverHash
		}
		if fields[i].PropHash != fields[j].PropHash {
			return fields[i].PropHash < fields[j].PropHash
		}
		return fields[i].Name < fields[j].Name
	})
}

func sortDebugShadowPairs(pairs []debugExportShadowPair) {
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].Shadowing.Node != pairs[j].Shadowing.Node {
			return pairs[i].Shadowing.Node < pairs[j].Shadowing.Node
		}
		if pairs[i].Shadowed.Node != pairs[j].Shadowed.Node {
			return pairs[i].Shadowed.Node < pairs[j].Shadowed.Node
		}
		if pairs[i].Shadowing.Name != pairs[j].Shadowing.Name {
			return pairs[i].Shadowing.Name < pairs[j].Shadowing.Name
		}
		return pairs[i].Shadowed.Name < pairs[j].Shadowed.Name
	})
}

func sortDebugReassignments(reassignments []debugExportReassignment) {
	sort.SliceStable(reassignments, func(i, j int) bool {
		if reassignments[i].NameHash != reassignments[j].NameHash {
			return reassignments[i].NameHash < reassignments[j].NameHash
		}
		if reassignments[i].Def.Node != reassignments[j].Def.Node {
			return reassignments[i].Def.Node < reassignments[j].Def.Node
		}
		return reassignments[i].Value.Node < reassignments[j].Value.Node
	})
}

func debugExportNodeName(doc *Document, source []byte, id ast.NodeID) string {
	if doc == nil || doc.Tree == nil || id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) || len(source) == 0 {
		return ""
	}

	node := doc.Tree.Nodes[id]
	if node.End > uint32(len(source)) || node.Start > node.End {
		return ""
	}

	return string(source[node.Start:node.End])
}

func (s *Server) exportDebugGlobalIndex() []debugExportGlobalSymbol {
	if s == nil || s.GlobalIndex == nil {
		return nil
	}

	symbols := make([]debugExportGlobalSymbol, 0)
	for resource, entry := range s.GlobalIndex.AllSymbols() {
		if entry == nil {
			continue
		}
		symbols = append(symbols, debugExportGlobalSymbol{
			Resource:      string(resource),
			Name:          string(entry.Name),
			URI:           entry.URI,
			NodeID:        entry.NodeID,
			Parent:        entry.Parent,
			ReceiverHash:  entry.Key.ReceiverHash,
			PropHash:      entry.Key.PropHash,
			IsRoot:        entry.IsRoot,
			IsDeprecated:  entry.IsDeprecated,
			DeprecatedMsg: entry.DeprecatedMsg,
			HasLuaDoc:     entry.LuaDoc != nil,
			HasFiveM:      entry.FiveM != nil,
			HasExport:     entry.Export != nil,
		})
	}

	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].Resource != symbols[j].Resource {
			return symbols[i].Resource < symbols[j].Resource
		}
		if symbols[i].Name != symbols[j].Name {
			return symbols[i].Name < symbols[j].Name
		}
		if symbols[i].URI != symbols[j].URI {
			return symbols[i].URI < symbols[j].URI
		}
		return symbols[i].NodeID < symbols[j].NodeID
	})

	return symbols
}

func debugExportFiveMProfile(profile FiveMExecutionProfile) string {
	return profile.Kind.String()
}

func debugExportTokenKind(kind token.Kind) string {
	switch kind {
	case token.Illegal:
		return "Illegal"
	case token.EOF:
		return "EOF"
	case token.Comment:
		return "Comment"
	case token.Ident:
		return "Ident"
	case token.Number:
		return "Number"
	case token.String:
		return "String"
	case token.BacktickString:
		return "BacktickString"
	case token.And:
		return "And"
	case token.Break:
		return "Break"
	case token.Do:
		return "Do"
	case token.Else:
		return "Else"
	case token.ElseIf:
		return "ElseIf"
	case token.End:
		return "End"
	case token.False:
		return "False"
	case token.For:
		return "For"
	case token.Function:
		return "Function"
	case token.Goto:
		return "Goto"
	case token.If:
		return "If"
	case token.In:
		return "In"
	case token.Local:
		return "Local"
	case token.Nil:
		return "Nil"
	case token.Not:
		return "Not"
	case token.Or:
		return "Or"
	case token.Repeat:
		return "Repeat"
	case token.Return:
		return "Return"
	case token.Then:
		return "Then"
	case token.True:
		return "True"
	case token.Until:
		return "Until"
	case token.While:
		return "While"
	case token.Plus:
		return "Plus"
	case token.Minus:
		return "Minus"
	case token.Asterisk:
		return "Asterisk"
	case token.Slash:
		return "Slash"
	case token.FloorSlash:
		return "FloorSlash"
	case token.Modulo:
		return "Modulo"
	case token.Caret:
		return "Caret"
	case token.Hash:
		return "Hash"
	case token.BitAnd:
		return "BitAnd"
	case token.BitOr:
		return "BitOr"
	case token.BitXor:
		return "BitXor"
	case token.ShiftLeft:
		return "ShiftLeft"
	case token.ShiftRight:
		return "ShiftRight"
	case token.Concat:
		return "Concat"
	case token.Eq:
		return "Eq"
	case token.NotEq:
		return "NotEq"
	case token.Less:
		return "Less"
	case token.LessEq:
		return "LessEq"
	case token.Greater:
		return "Greater"
	case token.GreaterEq:
		return "GreaterEq"
	case token.Assign:
		return "Assign"
	case token.LParen:
		return "LParen"
	case token.RParen:
		return "RParen"
	case token.LBrace:
		return "LBrace"
	case token.RBrace:
		return "RBrace"
	case token.LBrack:
		return "LBrack"
	case token.RBrack:
		return "RBrack"
	case token.DoubleColon:
		return "DoubleColon"
	case token.Semicolon:
		return "Semicolon"
	case token.Colon:
		return "Colon"
	case token.Comma:
		return "Comma"
	case token.Dot:
		return "Dot"
	case token.Vararg:
		return "Vararg"
	default:
		return fmt.Sprintf("Kind(%d)", kind)
	}
}

func debugExportNodeKind(kind ast.NodeKind) string {
	switch kind {
	case ast.KindInvalid:
		return "Invalid"
	case ast.KindFile:
		return "File"
	case ast.KindBlock:
		return "Block"
	case ast.KindLocalAssign:
		return "LocalAssign"
	case ast.KindAssign:
		return "Assign"
	case ast.KindIdent:
		return "Ident"
	case ast.KindNumber:
		return "Number"
	case ast.KindString:
		return "String"
	case ast.KindHashedString:
		return "HashedString"
	case ast.KindBinaryExpr:
		return "BinaryExpr"
	case ast.KindUnaryExpr:
		return "UnaryExpr"
	case ast.KindParenExpr:
		return "ParenExpr"
	case ast.KindNil:
		return "Nil"
	case ast.KindTrue:
		return "True"
	case ast.KindFalse:
		return "False"
	case ast.KindVararg:
		return "Vararg"
	case ast.KindFunctionExpr:
		return "FunctionExpr"
	case ast.KindTableExpr:
		return "TableExpr"
	case ast.KindIndexExpr:
		return "IndexExpr"
	case ast.KindMemberExpr:
		return "MemberExpr"
	case ast.KindCallExpr:
		return "CallExpr"
	case ast.KindRecordField:
		return "RecordField"
	case ast.KindIndexField:
		return "IndexField"
	case ast.KindMethodCall:
		return "MethodCall"
	case ast.KindMethodName:
		return "MethodName"
	case ast.KindExprList:
		return "ExprList"
	case ast.KindNameList:
		return "NameList"
	case ast.KindBreak:
		return "Break"
	case ast.KindReturn:
		return "Return"
	case ast.KindLabel:
		return "Label"
	case ast.KindGoto:
		return "Goto"
	case ast.KindDo:
		return "Do"
	case ast.KindWhile:
		return "While"
	case ast.KindRepeat:
		return "Repeat"
	case ast.KindIf:
		return "If"
	case ast.KindElseIf:
		return "ElseIf"
	case ast.KindElse:
		return "Else"
	case ast.KindForNum:
		return "ForNum"
	case ast.KindForIn:
		return "ForIn"
	case ast.KindLocalFunction:
		return "LocalFunction"
	case ast.KindFunctionStmt:
		return "FunctionStmt"
	default:
		return fmt.Sprintf("Kind(%d)", kind)
	}
}
