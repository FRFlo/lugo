package lsp

import "encoding/json"

// Request represents a JSON-RPC request message from the client.
type Request struct {
	RPC    string          `json:"jsonrpc"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     int             `json:"id"`
}

// Response represents a JSON-RPC response message from the server.
type Response struct {
	RPC    string `json:"jsonrpc"`
	Result any    `json:"result"`
	Error  any    `json:"error,omitempty"`
	ID     int    `json:"id"`
}

// Notification represents a JSON-RPC notification message.
type Notification struct {
	RPC    string          `json:"jsonrpc"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// OutgoingRequest represents a JSON-RPC request message sent from the server.
type OutgoingRequest struct {
	RPC    string `json:"jsonrpc"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
	ID     int    `json:"id"`
}

// OutgoingNotification represents a JSON-RPC notification message sent from the server.
type OutgoingNotification struct {
	RPC    string `json:"jsonrpc"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// ResponseError represents a JSON-RPC error object.
type ResponseError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Position represents a cursor position in a text document (0-indexed).
type Position struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

// Range represents a selection of text between two positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a range inside a specific document.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextEdit represents a text modification applied to a range.
type TextEdit struct {
	NewText string `json:"newText"`
	Range   Range  `json:"range"`
}

// WorkspaceEdit represents a collection of changes to multiple documents in the workspace.
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes"`
}

// Command represents a command that can be executed by the client or server.
type Command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

// MarkupContent represents a content value that can be rendered as plain text or markdown.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// WorkspaceFolder represents a root directory in the client's workspace.
type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// InitializeParams represents the parameters for the initialize request.
type InitializeParams struct {
	RootURI               string                `json:"rootUri"`
	WorkspaceFolders      []WorkspaceFolder     `json:"workspaceFolders,omitempty"`
	InitializationOptions InitializationOptions `json:"initializationOptions"`
	Capabilities          *ClientCapabilities   `json:"capabilities,omitempty"`
}

// InitializeResult represents the result of the initialize request.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

// ServerInfo represents identifying information about the language server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ClientCapabilities represents the capabilities provided by the client.
type ClientCapabilities struct {
	General      *GeneralClientCapabilities      `json:"general,omitempty"`
	TextDocument *TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	Workspace    *WorkspaceClientCapabilities    `json:"workspace,omitempty"`
}

// GeneralClientCapabilities represents general client capabilities.
type GeneralClientCapabilities struct {
	PositionEncodings []string `json:"positionEncodings,omitempty"`
}

// TextDocumentClientCapabilities represents text document specific client capabilities.
type TextDocumentClientCapabilities struct {
	Completion *CompletionClientCapabilities `json:"completion,omitempty"`
}

// CompletionClientCapabilities represents completion specific client capabilities.
type CompletionClientCapabilities struct {
	CompletionItem *CompletionItemClientCapabilities `json:"completionItem,omitempty"`
}

// CompletionItemClientCapabilities represents completion item specific client capabilities.
type CompletionItemClientCapabilities struct {
	SnippetSupport bool `json:"snippetSupport,omitempty"`
}

// WorkspaceClientCapabilities represents workspace specific client capabilities.
type WorkspaceClientCapabilities struct {
	WorkspaceFolders bool `json:"workspaceFolders,omitempty"`
}

// ExecuteCommandOptions represents options for the execute command provider.
type ExecuteCommandOptions struct {
	Commands []string `json:"commands"`
}

// CIConfig represents the configuration for continuous integration runs.
type CIConfig struct {
	WorkspaceFolders []string              `json:"workspaceFolders"`
	Settings         InitializationOptions `json:"settings"`
}

// InitializationOptions represents the custom configuration passed by the client during initialization.
type InitializationOptions struct {
	LibraryPaths  []string          `json:"libraryPaths,omitempty"`
	IgnoreGlobs   []string          `json:"ignoreGlobs,omitempty"`
	KnownGlobals  []string          `json:"knownGlobals,omitempty"`
	BannedSymbols map[string]string `json:"bannedSymbols,omitempty"`
	MaxFileSizeMB int               `json:"maxFileSizeMB"`

	ParserMaxErrors int `json:"parserMaxErrors"`

	DiagUndefinedGlobals     bool `json:"diagUndefinedGlobals"`
	DiagImplicitGlobals      bool `json:"diagImplicitGlobals"`
	DiagUnusedLocal          bool `json:"diagUnusedLocal"`
	DiagUnusedFunction       bool `json:"diagUnusedFunction"`
	DiagUnusedParameter      bool `json:"diagUnusedParameter"`
	DiagUnusedLoopVar        bool `json:"diagUnusedLoopVar"`
	DiagShadowing            bool `json:"diagShadowing"`
	DiagUnreachableCode      bool `json:"diagUnreachableCode"`
	DiagAmbiguousReturns     bool `json:"diagAmbiguousReturns"`
	DiagDeprecated           bool `json:"diagDeprecated"`
	DiagDuplicateField       bool `json:"diagDuplicateField"`
	DiagUnbalancedAssignment bool `json:"diagUnbalancedAssignment"`
	DiagDuplicateLocal       bool `json:"diagDuplicateLocal"`
	DiagSelfAssignment       bool `json:"diagSelfAssignment"`
	DiagEmptyBlock           bool `json:"diagEmptyBlock"`
	DiagFormatString         bool `json:"diagFormatString"`
	DiagTypeCheck            bool `json:"diagTypeCheck"`
	DiagRedundantParameter   bool `json:"diagRedundantParameter"`
	DiagRedundantValue       bool `json:"diagRedundantValue"`
	DiagRedundantReturn      bool `json:"diagRedundantReturn"`
	DiagLoopVarMutation      bool `json:"diagLoopVarMutation"`
	DiagIncorrectVararg      bool `json:"diagIncorrectVararg"`
	DiagShadowingLoopVar     bool `json:"diagShadowingLoopVar"`
	DiagConstantCondition    bool `json:"diagConstantCondition"`
	DiagUnreachableElse      bool `json:"diagUnreachableElse"`
	DiagUsedIgnoredVar       bool `json:"diagUsedIgnoredVar"`

	InlayParamHints    bool `json:"inlayParamHints"`
	InlaySuppressMatch bool `json:"inlaySuppressMatch"`
	InlayImplicitSelf  bool `json:"inlayImplicitSelf"`

	FeatureDocHighlight   bool `json:"featureDocHighlight"`
	FeatureHoverEval      bool `json:"featureHoverEval"`
	FeatureCodeLens       bool `json:"featureCodeLens"`
	FeatureFormatting     bool `json:"featureFormatting"`
	FormatOpinionated     bool `json:"formatOpinionated"`
	SuggestFunctionParams bool `json:"suggestFunctionParams"`
	FeatureFormatAlerts   bool `json:"featureFormatAlerts"`

	// DiagFiveMUnaccountedFile enables diagnostics for files not referenced in fxmanifest/resource files.
	DiagFiveMUnaccountedFile bool `json:"diagFiveMUnaccountedFile"`
	// DiagFiveMUnknownExport enables diagnostics for unknown export lookups.
	DiagFiveMUnknownExport bool `json:"diagFiveMUnknownExport"`
	// DiagFiveMUnknownResource enables diagnostics for exports addressing unknown resources.
	DiagFiveMUnknownResource      bool `json:"diagFiveMUnknownResource"`
	DiagFiveMEventDirection       bool `json:"diagFiveMEventDirection"`
	DiagFiveMUnregisteredNetEvent bool `json:"diagFiveMUnregisteredNetEvent"`
	DiagFiveMUnknownEvent         bool `json:"diagFiveMUnknownEvent"`
}

// ServerCapabilities represents the capabilities provided by the language server.
// Work done progress support is delivered through $/progress notifications.
type ServerCapabilities struct {
	CodeLensProvider       *CodeLensOptions       `json:"codeLensProvider,omitempty"`
	SignatureHelpProvider  *SignatureHelpOptions  `json:"signatureHelpProvider,omitempty"`
	CompletionProvider     *CompletionOptions     `json:"completionProvider,omitempty"`
	SemanticTokensProvider *SemanticTokensOptions `json:"semanticTokensProvider,omitempty"`
	ExecuteCommandProvider *ExecuteCommandOptions `json:"executeCommandProvider,omitempty"`
	RenameProvider         any                    `json:"renameProvider"`
	CodeActionProvider     any                    `json:"codeActionProvider"`
	// TextDocumentSync defines how text documents are synced with the server.
	TextDocumentSync                int      `json:"textDocumentSync"`
	DefinitionProvider              bool     `json:"definitionProvider"`
	HoverProvider                   bool     `json:"hoverProvider"`
	ReferencesProvider              bool     `json:"referencesProvider"`
	DocumentSymbolProvider          bool     `json:"documentSymbolProvider"`
	WorkspaceSymbolProvider         bool     `json:"workspaceSymbolProvider"`
	InlayHintProvider               any     `json:"inlayHintProvider"`
	FoldingRangeProvider            bool     `json:"foldingRangeProvider"`
	SelectionRangeProvider          bool     `json:"selectionRangeProvider,omitempty"`
	LinkedEditingRangeProvider      bool     `json:"linkedEditingRangeProvider"`
	CallHierarchyProvider           bool     `json:"callHierarchyProvider"`
	DocumentHighlightProvider       bool     `json:"documentHighlightProvider,omitempty"`
	DocumentFormattingProvider      bool     `json:"documentFormattingProvider,omitempty"`
	DocumentRangeFormattingProvider bool     `json:"documentRangeFormattingProvider,omitempty"`
	TypeDefinitionProvider          bool     `json:"typeDefinitionProvider,omitempty"`
	ImplementationProvider          bool     `json:"implementationProvider,omitempty"`
	DocumentLinkProvider            any      `json:"documentLinkProvider,omitempty"`
	PositionEncoding                string   `json:"positionEncoding,omitempty"`
	OffsetEncoding                  []string `json:"offsetEncoding,omitempty"`
	Workspace                       *WorkspaceServerCapabilities `json:"workspace,omitempty"`
}

// WorkspaceServerCapabilities represents workspace-specific server capabilities.
type WorkspaceServerCapabilities struct {
	FileOperations *WorkspaceFileOperationsServerCapabilities `json:"fileOperations,omitempty"`
}

// WorkspaceFileOperationsServerCapabilities represents workspace file operation capabilities.
type WorkspaceFileOperationsServerCapabilities struct {
	WillRename *FileOperationRegistrationOptions `json:"willRename,omitempty"`
}

// TextDocumentItem represents a text document that was opened on the client.
type TextDocumentItem struct {
	URI     string `json:"uri"`
	Text    string `json:"text"`
	Version int    `json:"version"`
}

// TextDocumentIdentifier represents a reference to a text document.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier represents a reference to a specific version of a text document.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentPositionParams represents parameters for requests that provide a document and a position.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// DidOpenTextDocumentParams represents parameters for the didOpen notification.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidCloseTextDocumentParams represents parameters for the didClose notification.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidChangeConfigurationParams represents parameters for the didChangeConfiguration notification.
type DidChangeConfigurationParams struct {
	Settings InitializationOptions `json:"settings"`
}

// DidChangeTextDocumentParams represents parameters for the didChange notification.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// TextDocumentContentChangeEvent represents a change to a text document.
type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

// DidChangeWatchedFilesParams represents parameters for the didChangeWatchedFiles notification.
type DidChangeWatchedFilesParams struct {
	Changes []FileEvent `json:"changes"`
}

// FileEvent represents a file change event.
type FileEvent struct {
	URI  string `json:"uri"`
	Type int    `json:"type"`
}

// ExecuteCommandParams represents parameters for the executeCommand request.
type ExecuteCommandParams struct {
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

// ApplyWorkspaceEditParams represents parameters for the applyEdit request.
type ApplyWorkspaceEditParams struct {
	Label string        `json:"label,omitempty"`
	Edit  WorkspaceEdit `json:"edit"`
}

// DiagnosticSeverity represents the severity level of a diagnostic.
type DiagnosticSeverity int

const (
	// SeverityError represents an error diagnostic.
	SeverityError DiagnosticSeverity = 1
	// SeverityWarning represents a warning diagnostic.
	SeverityWarning DiagnosticSeverity = 2
	// SeverityInformation represents an informational diagnostic.
	SeverityInformation DiagnosticSeverity = 3
	// SeverityHint represents a hint diagnostic.
	SeverityHint DiagnosticSeverity = 4
)

// DiagnosticTag represents additional metadata for a diagnostic.
type DiagnosticTag int

const (
	// Unnecessary represents unnecessary or unused code.
	Unnecessary DiagnosticTag = 1
	// Deprecated represents deprecated code.
	Deprecated DiagnosticTag = 2
)

// Diagnostic represents a diagnostic message, such as a compiler error or warning.
type Diagnostic struct {
	Message            string                         `json:"message"`
	Code               string                         `json:"code,omitempty"`
	Tags               []DiagnosticTag                `json:"tags,omitempty"`
	RelatedInformation []DiagnosticRelatedInformation `json:"relatedInformation,omitempty"`
	Data               any                            `json:"data,omitempty"`
	Range              Range                          `json:"range"`
	Severity           DiagnosticSeverity             `json:"severity,omitempty"`
}

// DiagnosticRelatedInformation represents related information for a diagnostic.
type DiagnosticRelatedInformation struct {
	Message  string   `json:"message"`
	Location Location `json:"location"`
}

// PublishDiagnosticsParams represents parameters for the publishDiagnostics notification.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Hover represents the result of a hover request.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// CompletionOptions represents options for the completion provider.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
	ResolveProvider   bool     `json:"resolveProvider,omitempty"`
}

// InlayHintOptions represents options for the inlayHint provider.
type InlayHintOptions struct {
	ResolveProvider bool `json:"resolveProvider,omitempty"`
}

// CompletionParams represents parameters for the completion request.
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// CompletionItemKind represents the kind of a completion item.
type CompletionItemKind int

const (
	// FunctionCompletion represents a function completion.
	FunctionCompletion CompletionItemKind = 3
	// FieldCompletion represents a field completion.
	FieldCompletion CompletionItemKind = 5
	// VariableCompletion represents a variable completion.
	VariableCompletion CompletionItemKind = 6
	// KeywordCompletion represents a keyword completion.
	KeywordCompletion CompletionItemKind = 14
)

// CompletionItemTag represents additional metadata for a completion item.
type CompletionItemTag int

const (
	// CompletionItemTagDeprecated represents a deprecated completion item.
	CompletionItemTagDeprecated CompletionItemTag = 1
)

// CompletionList represents a collection of completion items.
type CompletionList struct {
	Items        []CompletionItem `json:"items"`
	IsIncomplete bool             `json:"isIncomplete"`
}

// InsertTextFormat represents the format of the text to insert.
type InsertTextFormat int

const (
	// PlainTextTextFormat represents plain text format.
	PlainTextTextFormat InsertTextFormat = 1
	// SnippetTextFormat represents snippet format.
	SnippetTextFormat InsertTextFormat = 2
)

// CompletionItem represents an individual item in a completion list.
type CompletionItem struct {
	Label            string              `json:"label"`
	Detail           string              `json:"detail,omitempty"`
	SortText         string              `json:"sortText,omitempty"`
	Documentation    *MarkupContent      `json:"documentation,omitempty"`
	Tags             []CompletionItemTag `json:"tags,omitempty"`
	Kind             CompletionItemKind  `json:"kind"`
	InsertText       string              `json:"insertText,omitempty"`
	InsertTextFormat InsertTextFormat    `json:"insertTextFormat,omitempty"`
	Data             any                 `json:"data,omitempty"`
}

// SignatureHelpOptions represents options for the signature help provider.
type SignatureHelpOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// SignatureHelpParams represents parameters for the signature help request.
type SignatureHelpParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// SignatureHelp represents the signature help information.
type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

// SignatureInformation represents information about a single signature.
type SignatureInformation struct {
	Label         string                 `json:"label"`
	Documentation *MarkupContent         `json:"documentation,omitempty"`
	Parameters    []ParameterInformation `json:"parameters,omitempty"`
}

// ParameterInformation represents information about a single parameter of a signature.
type ParameterInformation struct {
	Label         string         `json:"label"`
	Documentation *MarkupContent `json:"documentation,omitempty"`
}

// InlayHintParams represents parameters for the inlay hint request.
type InlayHintParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
}

// InlayHintKind represents the kind of an inlay hint.
type InlayHintKind int

const (
	// TypeHint represents a type inlay hint.
	TypeHint InlayHintKind = 1
	// ParameterHint represents a parameter inlay hint.
	ParameterHint InlayHintKind = 2
)

// InlayHint represents an individual inlay hint.
type InlayHint struct {
	Label        string        `json:"label"`
	Tooltip      string        `json:"tooltip,omitempty"`
	Position     Position      `json:"position"`
	Kind         InlayHintKind `json:"kind,omitempty"`
	PaddingLeft  bool          `json:"paddingLeft,omitempty"`
	PaddingRight bool          `json:"paddingRight,omitempty"`
	Data         any           `json:"data,omitempty"`
}

// CodeActionParams represents parameters for the codeAction request.
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Context      CodeActionContext      `json:"context"`
	Range        Range                  `json:"range"`
}

// CodeActionContext represents additional information for a codeAction request.
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
}

// CodeAction represents a code action that can be performed by the server.
type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
	Command     *Command       `json:"command,omitempty"`
	Data        any            `json:"data,omitempty"`
	IsPreferred bool           `json:"isPreferred,omitempty"`
}

// CodeLensOptions represents options for the codeLens provider.
type CodeLensOptions struct {
	ResolveProvider bool `json:"resolveProvider,omitempty"`
}

// CodeLensParams represents parameters for the codeLens request.
type CodeLensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// CodeLens represents an individual code lens.
type CodeLens struct {
	Command *Command `json:"command,omitempty"`
	Data    any      `json:"data,omitempty"`
	Range   Range    `json:"range"`
}

// RenameParams represents parameters for the rename request.
type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	NewName      string                 `json:"newName"`
	Position     Position               `json:"position"`
}

// PrepareRenameParams represents parameters for the prepareRename request.
type PrepareRenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// PrepareRenameResult represents the result of the prepareRename request.
type PrepareRenameResult struct {
	Placeholder string `json:"placeholder"`
	Range       Range  `json:"range"`
}

// SymbolKind represents the kind of a symbol.
type SymbolKind int

const (
	// SymbolKindFile represents a file symbol.
	SymbolKindFile SymbolKind = 1
	// SymbolKindClass represents a class symbol.
	SymbolKindClass SymbolKind = 5 // class for tables
	// SymbolKindMethod represents a method symbol.
	SymbolKindMethod SymbolKind = 6
	// SymbolKindField represents a field symbol.
	SymbolKindField SymbolKind = 8
	// SymbolKindFunction represents a function symbol.
	SymbolKindFunction SymbolKind = 12
	// SymbolKindVariable represents a variable symbol.
	SymbolKindVariable SymbolKind = 13
	// SymbolKindEvent represents an event symbol.
	SymbolKindEvent SymbolKind = 24
)

// SymbolTag represents additional metadata for a symbol.
type SymbolTag int

const (
	// SymbolTagDeprecated represents a deprecated symbol.
	SymbolTagDeprecated SymbolTag = 1
)

// SymbolInformation represents information about a symbol.
type SymbolInformation struct {
	Name          string     `json:"name"`
	ContainerName string     `json:"containerName,omitempty"`
	Location      Location   `json:"location"`
	Kind          SymbolKind `json:"kind"`
}

// DocumentSymbol represents a symbol in a document.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Children       []DocumentSymbol `json:"children,omitempty"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Kind           SymbolKind       `json:"kind"`
}

// DocumentSymbolParams represents parameters for the documentSymbol request.
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// WorkspaceSymbolParams represents parameters for the workspaceSymbol request.
type WorkspaceSymbolParams struct {
	Query string `json:"query"`
}

// ReferenceParams represents parameters for the references request.
type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Context      ReferenceContext       `json:"context"`
	Position     Position               `json:"position"`
}

// ReferenceContext represents additional information for a references request.
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// DocumentHighlightParams represents parameters for the documentHighlight request.
type DocumentHighlightParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// DocumentHighlightKind represents the kind of a document highlight.
type DocumentHighlightKind int

const (
	// TextHighlight represents a simple text highlight.
	TextHighlight DocumentHighlightKind = 1
	// ReadHighlight represents a read access highlight.
	ReadHighlight DocumentHighlightKind = 2
	// WriteHighlight represents a write access highlight.
	WriteHighlight DocumentHighlightKind = 3
)

// DocumentHighlight represents an individual document highlight.
type DocumentHighlight struct {
	Range Range                 `json:"range"`
	Kind  DocumentHighlightKind `json:"kind,omitempty"`
}

// SemanticTokensOptions represents options for the semantic tokens provider.
type SemanticTokensOptions struct {
	Legend SemanticTokensLegend `json:"legend"`
	Full   bool                 `json:"full"`
	Range  any                  `json:"range,omitempty"`
}

// SemanticTokensLegend represents the legend for semantic tokens.
type SemanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

// SemanticTokensParams represents parameters for the semantic tokens request.
type SemanticTokensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// SemanticTokensRangeParams represents parameters for the semanticTokens/range request.
type SemanticTokensRangeParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
}

// SemanticTokens represents the semantic tokens information.
type SemanticTokens struct {
	Data []uint32 `json:"data"`
}

// CallHierarchyPrepareParams represents parameters for the callHierarchy/prepare request.
type CallHierarchyPrepareParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// CallHierarchyItem represents an item in a call hierarchy.
type CallHierarchyItem struct {
	Name           string      `json:"name"`
	Detail         string      `json:"detail,omitempty"`
	URI            string      `json:"uri"`
	Data           any         `json:"data,omitempty"`
	Tags           []SymbolTag `json:"tags,omitempty"`
	Range          Range       `json:"range"`
	SelectionRange Range       `json:"selectionRange"`
	Kind           SymbolKind  `json:"kind"`
}

// CallHierarchyIncomingCallsParams represents parameters for the callHierarchy/incomingCalls request.
type CallHierarchyIncomingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

// CallHierarchyIncomingCall represents an incoming call in a call hierarchy.
type CallHierarchyIncomingCall struct {
	From       CallHierarchyItem `json:"from"`
	FromRanges []Range           `json:"fromRanges"`
}

// CallHierarchyOutgoingCallsParams represents parameters for the callHierarchy/outgoingCalls request.
type CallHierarchyOutgoingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

// CallHierarchyOutgoingCall represents an outgoing call in a call hierarchy.
type CallHierarchyOutgoingCall struct {
	To         CallHierarchyItem `json:"to"`
	FromRanges []Range           `json:"fromRanges"`
}

// DocumentFormattingParams represents parameters for the documentFormatting request.
type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Options      FormattingOptions      `json:"options"`
}

// DocumentRangeFormattingParams represents parameters for the documentRangeFormatting request.
type DocumentRangeFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Options      FormattingOptions      `json:"options"`
}

// FormattingOptions represents options for document formatting.
type FormattingOptions struct {
	TabSize      int  `json:"tabSize"`
	InsertSpaces bool `json:"insertSpaces"`
}

// FoldingRangeParams represents parameters for the foldingRange request.
type FoldingRangeParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// FoldingRange represents an individual folding range.
type FoldingRange struct {
	Kind           string `json:"kind,omitempty"`
	StartLine      uint32 `json:"startLine"`
	StartCharacter uint32 `json:"startCharacter,omitempty"`
	EndLine        uint32 `json:"endLine"`
	EndCharacter   uint32 `json:"endCharacter,omitempty"`
}

// SelectionRangeParams represents parameters for the selectionRange request.
type SelectionRangeParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Positions    []Position             `json:"positions"`
}

// SelectionRange represents an individual selection range.
type SelectionRange struct {
	Range  Range           `json:"range"`
	Parent *SelectionRange `json:"parent,omitempty"`
}

// LinkedEditingRangeParams represents parameters for the linkedEditingRange request.
type LinkedEditingRangeParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// LinkedEditingRanges represents a collection of linked editing ranges.
type LinkedEditingRanges struct {
	WordPattern string  `json:"wordPattern,omitempty"`
	Ranges      []Range `json:"ranges"`
}

// DebugExportParams represents parameters for the custom debugExport request.
type DebugExportParams struct {
	Categories []string `json:"categories,omitempty"`
}

// DebugExportResult represents the result of the custom debugExport request.
type DebugExportResult struct {
	Content string `json:"content"`
}

// ShowMessageParams represents parameters for the window/showMessage notification.
type ShowMessageParams struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}

// WorkDoneProgressBegin represents the begin notification for work done progress.
type WorkDoneProgressBegin struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Cancellable bool   `json:"cancellable,omitempty"`
	Message     string `json:"message,omitempty"`
	Percentage  int    `json:"percentage,omitempty"`
}

// WorkDoneProgressReport represents the report notification for work done progress.
type WorkDoneProgressReport struct {
	Kind        string `json:"kind"`
	Cancellable bool   `json:"cancellable,omitempty"`
	Message     string `json:"message,omitempty"`
	Percentage  int    `json:"percentage,omitempty"`
}

// WorkDoneProgressEnd represents the end notification for work done progress.
type WorkDoneProgressEnd struct {
	Kind    string `json:"kind"`
	Message string `json:"message,omitempty"`
}

// ProgressParams represents parameters for the $/progress notification.
type ProgressParams struct {
	Token string `json:"token"`
	Value any    `json:"value"`
}

// TypeDefinitionParams represents parameters for the textDocument/typeDefinition request.
type TypeDefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// ImplementationParams represents parameters for the textDocument/implementation request.
type ImplementationParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// DidChangeWorkspaceFoldersParams represents parameters for the workspace/didChangeWorkspaceFolders notification.
type DidChangeWorkspaceFoldersParams struct {
	Event WorkspaceFoldersChangeEvent `json:"event"`
}

// WorkspaceFoldersChangeEvent represents a workspace folders change event.
type WorkspaceFoldersChangeEvent struct {
	Added   []WorkspaceFolder `json:"added"`
	Removed []WorkspaceFolder `json:"removed"`
}

// DocumentLinkOptions represents options for the documentLink provider.
type DocumentLinkOptions struct {
	ResolveProvider bool `json:"resolveProvider,omitempty"`
}

// DocumentLinkParams represents parameters for the textDocument/documentLink request.
type DocumentLinkParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DocumentLink represents a link in a document.
type DocumentLink struct {
	Range  Range   `json:"range"`
	Target string  `json:"target,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
	Data   any     `json:"data,omitempty"`
}

// FileRename represents a single file rename in workspace/willRenameFiles.
type FileRename struct {
	OldURI string `json:"oldUri"`
	NewURI string `json:"newUri"`
}

// WillRenameFilesParams represents parameters for the workspace/willRenameFiles request.
type WillRenameFilesParams struct {
	Files []FileRename `json:"files"`
}

// DidRenameFilesParams represents parameters for the workspace/didRenameFiles notification.
type DidRenameFilesParams struct {
	Files []FileRename `json:"files"`
}

// FileOperationRegistrationOptions represents registration options for file operations.
type FileOperationRegistrationOptions struct {
	Filters []FileOperationFilter `json:"filters"`
}

// FileOperationFilter represents a filter for file operations.
type FileOperationFilter struct {
	Scheme  string                `json:"scheme,omitempty"`
	Pattern FileOperationPattern  `json:"pattern"`
}

// FileOperationPattern represents a file operation pattern.
type FileOperationPattern struct {
	Glob string `json:"glob"`
}
