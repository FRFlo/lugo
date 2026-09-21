package lsp

// FiveMContractSnapshot is a read-only view of literal FiveM runtime
// surfaces. It intentionally does not participate in diagnostics or resolver
// state.
type FiveMContractSnapshot struct {
	Symbols   []FiveMContractSymbol
	Links     []FiveMContractLink
	Manifests []FiveMContractManifest
}

// FiveMContractManifest is a literal manifest declaration retained alongside
// contracts because manifests configure the runtime surface without being a
// Lua symbol themselves.
type FiveMContractManifest struct {
	Name     string
	Value    string
	Location FiveMContractLocation
}

// FiveMContractKind identifies the runtime surface represented by a contract.
type FiveMContractKind string

const (
	FiveMContractEvent  FiveMContractKind = "event"
	FiveMContractExport FiveMContractKind = "export"
	FiveMContractNUI    FiveMContractKind = "nui"
	FiveMContractConvar FiveMContractKind = "convar"
)

// FiveMContractDirection describes the direction in which a contract crosses
// a language/runtime boundary.
type FiveMContractDirection string

const (
	FiveMContractLuaToJS   FiveMContractDirection = "lua-to-js"
	FiveMContractJSToLua   FiveMContractDirection = "js-to-lua"
	FiveMContractLuaToHost FiveMContractDirection = "lua-to-host"
	FiveMContractHostToLua FiveMContractDirection = "host-to-lua"
	FiveMContractBidirect  FiveMContractDirection = "bidirectional"
)

// FiveMContractConfidence is deliberately qualitative: adapters should not
// imply more precision than their syntax permits.
type FiveMContractConfidence uint8

const (
	FiveMContractConfidenceUnknown FiveMContractConfidence = iota
	FiveMContractConfidenceLow
	FiveMContractConfidenceMedium
	FiveMContractConfidenceHigh
)

// FiveMContractLocation keeps a contract tied to the source that declared it.
// URI and Range are sufficient for both parsed Lua and scanned web assets.
type FiveMContractLocation struct {
	URI   string
	Range Range
}

// FiveMContractSymbol is one side of a cross-language contract.
type FiveMContractSymbol struct {
	Name      string
	Kind      FiveMContractKind
	Location  FiveMContractLocation
	Profile   FiveMExecutionProfileKind
	Direction FiveMContractDirection
}

// FiveMContractLink connects declarations/references without changing any
// existing diagnostics or index behavior.
type FiveMContractLink struct {
	From       FiveMContractSymbol
	To         FiveMContractSymbol
	Confidence FiveMContractConfidence
}

func fiveMContractLocation(uri string, src []byte, start, end int) FiveMContractLocation {
	return FiveMContractLocation{URI: uri, Range: sourceRange(src, start, end)}
}
