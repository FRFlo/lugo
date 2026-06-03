package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/coalaura/plain"

	"github.com/coalaura/lugo/ast"
	"github.com/coalaura/lugo/parser"
)

const (
	MaxWorkspaceResults    = 100
	DefaultMaxFileSize     = 4 * 1024 * 1024
	DefaultMaxParserErrors = 50
)

type Server struct {
	// Identity
	Version               string
	RootURI               string
	lowerRootPath         string
	WorkspaceFolders      []string
	lowerWorkspaceFolders []string

	// Config
	LibraryPaths           []string
	lowerLibraryPaths      []string
	IgnoreGlobs            []string
	compiledIgnores        []IgnorePattern
	BannedSymbols          map[string]string
	MaxParseErrors         int
	MaxFileSize            int64
	positionEncoding       string
	snippetSupport         bool
	workspaceFolderSupport bool

	// Transport & Logging
	Reader *bufio.Reader
	Writer io.Writer
	Log    *plain.Plain

	// Workspace State
	Documents          map[string]*Document
	OpenFiles          map[string]bool
	activeURIs         map[string]bool
	visitedDirs        map[string]bool
	FiveMResourceGraph *FiveMResourceGraph
	uriCache           map[string]string
	symlinkCache       map[string]string

	// Global Index
	GlobalIndex       *GlobalIndex
	TableAliases      map[uint64]uint64            // class hash → table receiver hash (from @type annotations)
	TableAliasSources map[uint64]map[string]uint64 // class hash → source URI → table receiver hash
	KnownGlobals      map[string]bool
	KnownGlobalGlobs  []string

	// Feature Toggles
	IsIndexing               bool
	DiagUndefinedGlobals     bool
	DiagImplicitGlobals      bool
	DiagUnusedLocal          bool
	DiagUnusedFunction       bool
	DiagUnusedParameter      bool
	DiagUnusedLoopVar        bool
	DiagShadowing            bool
	DiagUnreachableCode      bool
	DiagAmbiguousReturns     bool
	DiagDeprecated           bool
	DiagDuplicateField       bool
	DiagUnbalancedAssignment bool
	DiagDuplicateLocal       bool
	DiagSelfAssignment       bool
	DiagEmptyBlock           bool
	DiagFormatString         bool
	DiagTypeCheck            bool
	DiagRedundantParameter   bool
	DiagRedundantValue       bool
	DiagRedundantReturn      bool
	DiagLoopVarMutation      bool
	DiagIncorrectVararg      bool
	DiagShadowingLoopVar     bool
	DiagConstantCondition    bool
	DiagUnreachableElse      bool
	DiagUsedIgnoredVar       bool

	InlayParamHints    bool
	InlaySuppressMatch bool
	InlayImplicitSelf  bool

	FeatureDocHighlight   bool
	FeatureHoverEval      bool
	FeatureCodeLens       bool
	FeatureFormatting     bool
	FormatOpinionated     bool
	SuggestFunctionParams bool
	FeatureFormatAlerts   bool

	// FiveM-specific Fields
	DiagFiveMUnaccountedFile      bool
	DiagFiveMUnknownExport        bool
	DiagFiveMUnknownResource      bool
	DiagFiveMEventDirection       bool
	DiagFiveMUnregisteredNetEvent bool
	DiagFiveMUnknownEvent         bool

	// CI Fields
	IsCI              bool
	CIDiagnosticCount int
	CIErrorCount      int

	// Shared Buffers
	sharedParser     *parser.Parser
	diagBuf          []Diagnostic
	semTokensBuf     []SemanticToken
	semDataBuf       []uint32
	actualReadsBuf   []int
	depCache         map[ast.NodeID]DepInfo
	seenKeysBuf      map[uint64]ast.NodeID
	unusedDefsBuf    []bool
	deadStoresBuf    map[ast.NodeID]*DeadStoreInfo
	suggestCache     map[string]string
	visibilityCache  map[*Document]bool
	sharedCommentBuf []byte
	sharedDepBuf     []byte
}

// evictClosedDocumentCaches drops memory-heavy caches for documents that are closed
// or not currently opened. This keeps AST + Resolver in memory for cross-document
// features while freeing large in-memory caches tied to the source bytes.
//
// Rules:
//   - Do not touch Resolver or Tree (AST remains required for cross-document features)
//   - Preserve library document source/caches so std/runtime hover and signature metadata
//     remain available after unrelated workspace files are closed/reopened.
//   - For each non-library document not currently open (OpenFiles[uri] is false/absent):
//   - clear TypeCache, Inferring, LuaDocCache, ActualReads, MutatedLocals
//   - nil the Tree.Source pointer (Tree owns Source now)
func evictClosedDocumentCaches(s *Server) {
	if s == nil {
		return
	}
	for uri, doc := range s.Documents {
		if doc == nil {
			continue
		}
		if doc.IsLibrary {
			continue
		}
		// Skip currently open documents
		if s.OpenFiles != nil {
			if open, ok := s.OpenFiles[uri]; ok && open {
				continue
			}
		}

		// Evict large caches. Preserve AST (doc.Tree) and Resolver for cross-doc features.
		doc.TypeCache = nil
		doc.Inferring = nil
		doc.LuaDocCache = nil
		doc.ActualReads = nil
		doc.MutatedLocals = nil
		// Tree owns Source; free the underlying source buffer
		if doc.Tree != nil {
			doc.Tree.Source = nil
		}
		// Do not modify doc.Resolver or doc.Tree themselves
	}
}

func NewServer(version string) *Server {
	return &Server{
		Version: version,
		Reader:  bufio.NewReader(os.Stdin),
		Writer:  os.Stdout,

		// Workspace State
		Documents:          make(map[string]*Document),
		OpenFiles:          make(map[string]bool),
		IsIndexing:         true,
		FiveMResourceGraph: NewFiveMResourceGraph(),
		uriCache:           make(map[string]string, 1024),

		// Global Index
		GlobalIndex:       NewGlobalIndex(),
		TableAliases:      make(map[uint64]uint64),
		TableAliasSources: make(map[uint64]map[string]uint64),

		// Shared Buffers
		sharedParser:     parser.New(nil, ast.NewTree(nil), 50),
		diagBuf:          make([]Diagnostic, 0, 1024),
		semTokensBuf:     make([]SemanticToken, 0, 4096),
		semDataBuf:       make([]uint32, 0, 4096*5),
		actualReadsBuf:   make([]int, 0, 4096),
		sharedCommentBuf: make([]byte, 0, 1024),
		sharedDepBuf:     make([]byte, 0, 128),

		// Configuration Defaults
		MaxParseErrors:   DefaultMaxParserErrors,
		MaxFileSize:      DefaultMaxFileSize,
		positionEncoding: "utf-16",
		snippetSupport:   true,
	}
}

func (s *Server) Start() error {
	s.Log = plain.New(
		plain.WithTarget(os.Stderr),
		plain.WithDate(plain.RFC3339Local),
	)

	s.Log.Printf("Lugo LSP %s Started\n", s.Version)

	for {
		msg, err := ReadMessage(s.Reader)
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "closed") {
				s.Log.Println("Input stream closed, stopping server.")

				break
			}

			s.Log.Errorf("Error reading message: %v\n", err)

			continue
		}

		var req Request

		err = json.Unmarshal(msg, &req)
		if err != nil {
			s.Log.Errorf("Failed to unmarshal request: %v\n", err)

			continue
		}

		s.handleMessage(req)
	}

	return nil
}

func (s *Server) applyInitializationOptions(opts InitializationOptions) (needsReindex bool, needsRepublish bool) {
	SetTelemetryEnabled(opts.TelemetryEnabled)

	effectiveLibraryPaths := s.buildConfiguredLibraryPaths(opts.LibraryPaths)
	if s.setLibraryPaths(effectiveLibraryPaths) {
		needsReindex = true
	}

	if s.setIgnoreGlobs(opts.IgnoreGlobs) {
		needsReindex = true
	}

	if s.setKnownGlobals(opts.KnownGlobals) {
		needsReindex = true
	}

	if !maps.Equal(s.BannedSymbols, opts.BannedSymbols) {
		s.BannedSymbols = opts.BannedSymbols

		needsRepublish = true
	}

	setCfg(&s.MaxParseErrors, opts.ParserMaxErrors, &needsRepublish)

	maxSize := int64(opts.MaxFileSizeMB) * 1024 * 1024

	if maxSize <= 0 {
		maxSize = DefaultMaxFileSize
	}

	setCfg(&s.MaxFileSize, maxSize, &needsReindex)

	setCfg(&s.DiagUndefinedGlobals, opts.DiagUndefinedGlobals, &needsRepublish)
	setCfg(&s.DiagImplicitGlobals, opts.DiagImplicitGlobals, &needsRepublish)
	setCfg(&s.DiagUnusedLocal, opts.DiagUnusedLocal, &needsRepublish)
	setCfg(&s.DiagUnusedFunction, opts.DiagUnusedFunction, &needsRepublish)
	setCfg(&s.DiagUnusedParameter, opts.DiagUnusedParameter, &needsRepublish)
	setCfg(&s.DiagUnusedLoopVar, opts.DiagUnusedLoopVar, &needsRepublish)
	setCfg(&s.DiagShadowing, opts.DiagShadowing, &needsRepublish)
	setCfg(&s.DiagUnreachableCode, opts.DiagUnreachableCode, &needsRepublish)
	setCfg(&s.DiagAmbiguousReturns, opts.DiagAmbiguousReturns, &needsRepublish)
	setCfg(&s.DiagDeprecated, opts.DiagDeprecated, &needsRepublish)
	setCfg(&s.DiagDuplicateField, opts.DiagDuplicateField, &needsRepublish)
	setCfg(&s.DiagUnbalancedAssignment, opts.DiagUnbalancedAssignment, &needsRepublish)
	setCfg(&s.DiagDuplicateLocal, opts.DiagDuplicateLocal, &needsRepublish)
	setCfg(&s.DiagSelfAssignment, opts.DiagSelfAssignment, &needsRepublish)
	setCfg(&s.DiagEmptyBlock, opts.DiagEmptyBlock, &needsRepublish)
	setCfg(&s.DiagFormatString, opts.DiagFormatString, &needsRepublish)
	setCfg(&s.DiagTypeCheck, opts.DiagTypeCheck, &needsRepublish)
	setCfg(&s.DiagRedundantParameter, opts.DiagRedundantParameter, &needsRepublish)
	setCfg(&s.DiagRedundantValue, opts.DiagRedundantValue, &needsRepublish)
	setCfg(&s.DiagRedundantReturn, opts.DiagRedundantReturn, &needsRepublish)
	setCfg(&s.DiagLoopVarMutation, opts.DiagLoopVarMutation, &needsRepublish)
	setCfg(&s.DiagIncorrectVararg, opts.DiagIncorrectVararg, &needsRepublish)
	setCfg(&s.DiagShadowingLoopVar, opts.DiagShadowingLoopVar, &needsRepublish)
	setCfg(&s.DiagConstantCondition, opts.DiagConstantCondition, &needsRepublish)
	setCfg(&s.DiagUnreachableElse, opts.DiagUnreachableElse, &needsRepublish)
	setCfg(&s.DiagUsedIgnoredVar, opts.DiagUsedIgnoredVar, &needsRepublish)

	setCfg(&s.InlayParamHints, opts.InlayParamHints, nil)
	setCfg(&s.InlaySuppressMatch, opts.InlaySuppressMatch, nil)
	setCfg(&s.InlayImplicitSelf, opts.InlayImplicitSelf, nil)

	setCfg(&s.FeatureDocHighlight, opts.FeatureDocHighlight, nil)
	setCfg(&s.FeatureHoverEval, opts.FeatureHoverEval, nil)
	setCfg(&s.FeatureCodeLens, opts.FeatureCodeLens, nil)
	setCfg(&s.FeatureFormatting, opts.FeatureFormatting, nil)
	setCfg(&s.FormatOpinionated, opts.FormatOpinionated, nil)
	setCfg(&s.SuggestFunctionParams, opts.SuggestFunctionParams, nil)
	setCfg(&s.FeatureFormatAlerts, opts.FeatureFormatAlerts, nil)

	setCfg(&s.DiagFiveMUnaccountedFile, opts.DiagFiveMUnaccountedFile, &needsRepublish)
	setCfg(&s.DiagFiveMUnknownExport, opts.DiagFiveMUnknownExport, &needsRepublish)
	setCfg(&s.DiagFiveMUnknownResource, opts.DiagFiveMUnknownResource, &needsRepublish)
	setCfg(&s.DiagFiveMEventDirection, opts.DiagFiveMEventDirection, &needsRepublish)
	setCfg(&s.DiagFiveMUnregisteredNetEvent, opts.DiagFiveMUnregisteredNetEvent, &needsRepublish)
	setCfg(&s.DiagFiveMUnknownEvent, opts.DiagFiveMUnknownEvent, &needsRepublish)

	return needsReindex, needsRepublish
}

func (s *Server) setIgnoreGlobs(globs []string) bool {
	if slices.Equal(s.IgnoreGlobs, globs) {
		return false
	}

	s.IgnoreGlobs = slices.Clone(globs)
	s.compileIgnorePatterns()

	return true
}

func (s *Server) buildConfiguredLibraryPaths(paths []string) []string {
	configured := slices.Clone(paths)

	return configured
}

func (s *Server) setKnownGlobals(globals []string) bool {
	var (
		newKnownGlobals     map[string]bool
		newKnownGlobalGlobs []string
	)

	if len(globals) > 0 {
		newKnownGlobals = make(map[string]bool, len(globals))
		newKnownGlobalGlobs = make([]string, 0, len(globals))

		for _, g := range globals {
			if strings.ContainsAny(g, "*?") {
				newKnownGlobalGlobs = append(newKnownGlobalGlobs, g)
			} else {
				newKnownGlobals[g] = true
			}
		}
	}

	if maps.Equal(s.KnownGlobals, newKnownGlobals) && slices.Equal(s.KnownGlobalGlobs, newKnownGlobalGlobs) {
		return false
	}

	s.KnownGlobals = newKnownGlobals
	s.KnownGlobalGlobs = newKnownGlobalGlobs

	return true
}

func (s *Server) setLibraryPaths(paths []string) bool {
	if slices.Equal(s.LibraryPaths, paths) {
		return false
	}

	s.LibraryPaths = slices.Clone(paths)
	s.lowerLibraryPaths = s.lowerLibraryPaths[:0]

	for i, lib := range s.LibraryPaths {
		if realPath, err := filepath.EvalSymlinks(lib); err == nil {
			s.LibraryPaths[i] = realPath

			lib = realPath
		}

		s.lowerLibraryPaths = append(s.lowerLibraryPaths, strings.ToLower(filepath.Clean(filepath.FromSlash(lib))))
	}

	return true
}

func (s *Server) handleMessage(req Request) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()

			s.Log.Errorf("CRITICAL PANIC in method %s: %v\n%s\n", req.Method, r, string(stack))

			CapturePanic(r, "handleMessage:"+req.Method)
			FlushTelemetry()

			// Attempt to notify the client before we die
			if req.ID != 0 {
				WriteMessage(s.Writer, Response{
					RPC: "2.0",
					ID:  req.ID,
					Error: ResponseError{
						Code:    -32603, // InternalError
						Message: fmt.Sprintf("Lugo LSP crashed critically: %v", r),
					},
				})
			}

			// Fail-fast
			os.Exit(1)
		}
	}()

	s.Log.Debugf("Received method: %s\n", req.Method)

	switch req.Method {
	// Lifecycle
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		s.handleInitialized(req)
	case "shutdown":
		s.handleShutdown(req)
	case "exit":
		s.handleExit()

	// Workspace
	case "workspace/didChangeConfiguration":
		s.handleDidChangeConfiguration(req)
	case "workspace/didChangeWatchedFiles":
		s.handleDidChangeWatchedFiles(req)
	case "workspace/didChangeWorkspaceFolders":
		s.handleDidChangeWorkspaceFolders(req)
	case "workspace/willRenameFiles":
		s.handleWillRenameFiles(req)
	case "$/cancelRequest":
		// Cancel is a no-op for single-threaded servers;
		// the client may still send it per LSP spec.
	case "textDocument/didOpen":
		s.handleDidOpen(req)
	case "textDocument/didChange":
		s.handleDidChange(req)
	case "textDocument/didClose":
		s.handleDidClose(req)
	case "lugo/reindex":
		s.handleReindex(req)
	case "lugo/debugExport":
		s.handleDebugExport(req)

	// Symbols & Navigation
	case "textDocument/definition":
		s.handleDefinition(req)
	case "textDocument/typeDefinition":
		s.handleTypeDefinition(req)
	case "textDocument/implementation":
		s.handleImplementation(req)
	case "textDocument/references":
		s.handleReferences(req)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(req)
	case "workspace/symbol":
		s.handleWorkspaceSymbol(req)

	// Refactoring & Code Actions
	case "textDocument/codeAction":
		s.handleCodeAction(req)
	case "codeAction/resolve":
		s.handleCodeActionResolve(req)
	case "workspace/executeCommand":
		s.handleExecuteCommand(req)
	case "textDocument/prepareRename":
		s.handlePrepareRename(req)
	case "textDocument/rename":
		s.handleRename(req)
	case "textDocument/linkedEditingRange":
		s.handleLinkedEditingRange(req)

	// Editor Features
	case "textDocument/hover":
		s.handleHover(req)
	case "textDocument/completion":
		s.handleCompletion(req)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(req)
	case "textDocument/inlayHint":
		s.handleInlayHint(req)
	case "textDocument/documentHighlight":
		s.handleDocumentHighlight(req)
	case "textDocument/semanticTokens/full":
		s.handleSemanticTokensFull(req)
	case "textDocument/formatting":
		s.handleFormatting(req)
	case "textDocument/rangeFormatting":
		s.handleRangeFormatting(req)
	case "textDocument/foldingRange":
		s.handleFoldingRange(req)
	case "textDocument/selectionRange":
		s.handleSelectionRange(req)
	case "textDocument/codeLens":
		s.handleCodeLens(req)
	case "codeLens/resolve":
		s.handleCodeLensResolve(req)
	case "completionItem/resolve":
		s.handleCompletionResolve(req)
	case "inlayHint/resolve":
		s.handleInlayHintResolve(req)
	case "textDocument/semanticTokens/range":
		s.handleSemanticTokensRange(req)
	case "textDocument/documentLink":
		s.handleDocumentLink(req)
	case "documentLink/resolve":
		s.handleDocumentLinkResolve(req)
	case "textDocument/prepareCallHierarchy":
		s.handlePrepareCallHierarchy(req)
	case "callHierarchy/incomingCalls":
		s.handleCallHierarchyIncomingCalls(req)
	case "callHierarchy/outgoingCalls":
		s.handleCallHierarchyOutgoingCalls(req)
	}
}

func (s *Server) handleInitialize(req Request) {
	var params InitializeParams

	err := json.Unmarshal(req.Params, &params)
	if err == nil {
		s.applyClientCapabilities(params.Capabilities)

		if len(params.WorkspaceFolders) > 0 {
			for _, folder := range params.WorkspaceFolders {
				uri := s.normalizeURI(folder.URI)

				s.WorkspaceFolders = append(s.WorkspaceFolders, uri)
				s.lowerWorkspaceFolders = append(s.lowerWorkspaceFolders, strings.ToLower(s.uriToPath(uri)))
			}

			s.RootURI = s.WorkspaceFolders[0]
			s.lowerRootPath = s.lowerWorkspaceFolders[0]
		} else if params.RootURI != "" {
			s.RootURI = s.normalizeURI(params.RootURI)
			s.lowerRootPath = strings.ToLower(s.uriToPath(s.RootURI))

			s.WorkspaceFolders = []string{s.RootURI}
			s.lowerWorkspaceFolders = []string{s.lowerRootPath}
		}

		s.applyInitializationOptions(params.InitializationOptions)
		if !s.snippetSupport {
			s.SuggestFunctionParams = false
		}
	}

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync:   1,
			DefinitionProvider: true,
			HoverProvider:      true,
			RenameProvider: map[string]bool{
				"prepareProvider": true,
			},
			ReferencesProvider:              true,
			DocumentSymbolProvider:          true,
			WorkspaceSymbolProvider:         true,
			InlayHintProvider: &InlayHintOptions{
				ResolveProvider: true,
			},
			FoldingRangeProvider:            true,
			SelectionRangeProvider:          true,
			CallHierarchyProvider:           true,
			DocumentHighlightProvider:       true,
			DocumentFormattingProvider:      true,
			DocumentRangeFormattingProvider: true,
			TypeDefinitionProvider:          true,
			ImplementationProvider:          true,
			PositionEncoding:                s.positionEncoding,
			OffsetEncoding:                  []string{s.positionEncoding},
			CodeActionProvider: map[string]any{
				"codeActionKinds": []string{"quickfix", "refactor.rewrite"},
				"resolveProvider": true,
			},
			CodeLensProvider: &CodeLensOptions{
				ResolveProvider: true,
			},
			SignatureHelpProvider: &SignatureHelpOptions{
				TriggerCharacters: []string{"(", ","},
			},
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: []string{".", ":"},
				ResolveProvider:   true,
			},
			SemanticTokensProvider: &SemanticTokensOptions{
				Legend: SemanticTokensLegend{
					TokenTypes:     []string{"variable", "property", "parameter", "function", "method", "class", "number", "string", "keyword", "regexp"},
					TokenModifiers: []string{"declaration", "readonly", "deprecated", "defaultLibrary"},
				},
				Full:  true,
				Range: true,
			},
			DocumentLinkProvider: &DocumentLinkOptions{
				ResolveProvider: true,
			},
			ExecuteCommandProvider: &ExecuteCommandOptions{
				Commands: []string{"lugo.applySafeFixes"},
			},
			Workspace: &WorkspaceServerCapabilities{
				FileOperations: &WorkspaceFileOperationsServerCapabilities{
					WillRename: &FileOperationRegistrationOptions{
						Filters: []FileOperationFilter{
							{
								Scheme: "file",
								Pattern: FileOperationPattern{
									Glob: "**/*.lua",
								},
							},
						},
					},
				},
			},
		},
		ServerInfo: &ServerInfo{
			Name:    "lugo",
			Version: s.Version,
		},
	}

	err = WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: result})
	if err != nil {
		s.Log.Errorf("WriteMessage error: %v\n", err)
	}
}

func (s *Server) applyClientCapabilities(caps *ClientCapabilities) {
	s.positionEncoding = "utf-16"
	s.snippetSupport = true
	s.workspaceFolderSupport = false

	if caps == nil {
		return
	}

	if caps.General != nil {
		for _, enc := range caps.General.PositionEncodings {
			if enc == "utf-8" {
				s.positionEncoding = "utf-8"
				break
			}

			if enc == "utf-16" {
				s.positionEncoding = "utf-16"
			}
		}
	}

	s.snippetSupport = false
	if caps.TextDocument != nil && caps.TextDocument.Completion != nil && caps.TextDocument.Completion.CompletionItem != nil {
		s.snippetSupport = caps.TextDocument.Completion.CompletionItem.SnippetSupport
	}

	if caps.Workspace != nil {
		s.workspaceFolderSupport = caps.Workspace.WorkspaceFolders
	}
}

func (s *Server) handleInitialized(req Request) {
	_ = req

	s.IsIndexing = false
	go s.refreshWorkspace()
	s.sendShowMessage(3, "Lugo LSP "+s.Version+" ready.")
}

func (s *Server) handleDidChangeWorkspaceFolders(req Request) {
	var params DidChangeWorkspaceFoldersParams

	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.Log.Errorf("Failed to unmarshal workspace/didChangeWorkspaceFolders params: %v\n", err)

		return
	}

	if len(params.Event.Removed) > 0 {
		removed := make(map[string]bool, len(params.Event.Removed))
		for _, folder := range params.Event.Removed {
			removed[s.normalizeURI(folder.URI)] = true
		}

		workspaceFolders := s.WorkspaceFolders[:0]
		lowerWorkspaceFolders := s.lowerWorkspaceFolders[:0]
		for i, uri := range s.WorkspaceFolders {
			if removed[uri] {
				continue
			}

			workspaceFolders = append(workspaceFolders, uri)
			lowerWorkspaceFolders = append(lowerWorkspaceFolders, s.lowerWorkspaceFolders[i])
		}

		s.WorkspaceFolders = workspaceFolders
		s.lowerWorkspaceFolders = lowerWorkspaceFolders
	}

	for _, folder := range params.Event.Added {
		uri := s.normalizeURI(folder.URI)
		if uri == "" {
			continue
		}

		if slices.Contains(s.WorkspaceFolders, uri) {
			continue
		}

		s.WorkspaceFolders = append(s.WorkspaceFolders, uri)
		s.lowerWorkspaceFolders = append(s.lowerWorkspaceFolders, strings.ToLower(s.uriToPath(uri)))
	}

	if len(s.WorkspaceFolders) > 0 {
		s.RootURI = s.WorkspaceFolders[0]
		s.lowerRootPath = s.lowerWorkspaceFolders[0]
	} else {
		s.RootURI = ""
		s.lowerRootPath = ""
	}

	s.refreshWorkspace()
}

func (s *Server) handleWillRenameFiles(req Request) {
	var params WillRenameFilesParams

	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.Log.Errorf("Failed to unmarshal workspace/willRenameFiles params: %v\n", err)

		return
	}

	edit := s.computeRequirePathUpdates(params.Files)

	WriteMessage(s.Writer, Response{
		RPC:    "2.0",
		ID:     req.ID,
		Result: edit,
	})
}

// computeRequirePathUpdates scans all open documents for require()/dofile()/loadfile()
// calls referencing old paths and returns a WorkspaceEdit to update them.
func (s *Server) computeRequirePathUpdates(renames []FileRename) WorkspaceEdit {
	changes := make(map[string][]TextEdit)

	// Build a map from old base name to new relative path for quick lookup.
	type renameMapping struct {
		oldURI string
		newURI string
	}

	mappings := make([]renameMapping, 0, len(renames))
	for _, r := range renames {
		if r.OldURI == r.NewURI {
			continue
		}

		r.OldURI = s.normalizeURI(r.OldURI)
		r.NewURI = s.normalizeURI(r.NewURI)

		mappings = append(mappings, renameMapping{oldURI: r.OldURI, newURI: r.NewURI})
	}

	if len(mappings) == 0 {
		return WorkspaceEdit{Changes: changes}
	}

	// Compute old/new paths to build string replacements.
	type pathMapping struct {
		oldPath string
		newPath string
	}

	pathMappings := make([]pathMapping, 0, len(mappings))
	for _, m := range mappings {
		oldPath := s.uriToPath(m.oldURI)
		newPath := s.uriToPath(m.newURI)
		if oldPath == "" || newPath == "" {
			continue
		}

		pathMappings = append(pathMappings, pathMapping{oldPath: oldPath, newPath: newPath})
	}

	if len(pathMappings) == 0 {
		return WorkspaceEdit{Changes: changes}
	}

	for uri, doc := range s.Documents {
		if doc.Tree == nil {
			continue
		}

		var edits []TextEdit

		for id := ast.NodeID(1); id < ast.NodeID(len(doc.Tree.Nodes)); id++ {
			node := &doc.Tree.Nodes[id]
			if node.Kind != ast.KindCallExpr {
				continue
			}

			funcNode := &doc.Tree.Nodes[node.Left]
			if funcNode.Kind != ast.KindIdent {
				continue
			}

			funcName := doc.Source()[funcNode.Start:funcNode.End]
			if !bytes.Equal(funcName, []byte("require")) &&
				!bytes.Equal(funcName, []byte("dofile")) &&
				!bytes.Equal(funcName, []byte("loadfile")) {
				continue
			}

			if node.Extra == 0 || int(node.Extra) >= len(doc.Tree.ExtraList) {
				continue
			}

			argID := doc.Tree.ExtraList[node.Extra]
			res, ok := doc.evalNode(argID, 0)
			if !ok || res.kind != ast.KindString {
				continue
			}

			currentPath := res.str

			// Resolve the current path relative to document's directory.
			basePath := s.uriToPath(uri)
			documentDir := filepath.Dir(basePath)

			resolvedPath := filepath.Join(documentDir, currentPath)
			if filepath.Ext(resolvedPath) == "" {
				resolvedPath += ".lua"
			}

			// Check if this resolved path matches any renamed file.
			var matchedNewPath string
			for _, pm := range pathMappings {
				if resolvedPath == pm.oldPath {
					// Compute the new relative path from the document's directory.
					rel, err := filepath.Rel(documentDir, pm.newPath)
					if err != nil {
						continue
					}

					// Strip .lua extension if the original didn't have it.
					if filepath.Ext(currentPath) == "" {
						rel = rel[:len(rel)-len(".lua")]
					}

					// Normalize to forward slashes for Lua.
					rel = filepath.ToSlash(rel)

					matchedNewPath = rel

					break
				}
			}

			if matchedNewPath == "" {
				continue
			}

			argNode := doc.Tree.Nodes[argID]
			startLine, startChar := doc.Tree.Position(argNode.Start)
			endLine, endChar := doc.Tree.Position(argNode.End)

			edits = append(edits, TextEdit{
				Range: Range{
					Start: Position{Line: startLine, Character: startChar},
					End:   Position{Line: endLine, Character: endChar},
				},
				NewText: `"` + matchedNewPath + `"`,
			})
		}

		if len(edits) > 0 {
			changes[uri] = edits
		}
	}

	return WorkspaceEdit{Changes: changes}
}

func (s *Server) sendShowMessage(msgType int, message string) {
	WriteMessage(s.Writer, OutgoingNotification{
		RPC:    "2.0",
		Method: "window/showMessage",
		Params: ShowMessageParams{Type: msgType, Message: message},
	})
}

func (s *Server) handleShutdown(req Request) {
	err := WriteMessage(s.Writer, Response{RPC: "2.0", ID: req.ID, Result: nil})
	if err != nil {
		s.Log.Errorf("WriteMessage error (shutdown): %v\n", err)
	}
}

func (s *Server) handleExit() {
	s.Log.Println("Received exit notification, terminating.")

	os.Exit(0)
}

func (s *Server) getRequireModName(doc *Document, callID ast.NodeID) string {
	if callID == ast.InvalidNode || int(callID) >= len(doc.Tree.Nodes) {
		return ""
	}

	node := doc.Tree.Nodes[callID]
	if node.Kind != ast.KindCallExpr {
		return ""
	}

	if int(node.Left) >= len(doc.Tree.Nodes) {
		return ""
	}

	funcNode := doc.Tree.Nodes[node.Left]
	if funcNode.Kind != ast.KindIdent {
		return ""
	}

	funcName := doc.Source()[funcNode.Start:funcNode.End]
	if !bytes.Equal(funcName, []byte("require")) {
		return ""
	}

	if node.Count == 0 || node.Extra >= uint32(len(doc.Tree.ExtraList)) {
		return ""
	}

	argID := doc.Tree.ExtraList[node.Extra]

	res, ok := doc.evalNode(argID, 0)
	if ok && res.kind == ast.KindString {
		return res.str
	}

	return ""
}
