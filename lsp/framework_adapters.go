package lsp

import (
	"reflect"
	"strings"
)

// FrameworkAdapterConfig selects an optional framework adapter pack. Adapter packs
// only contribute editor metadata; they do not alter Lua resolution semantics.
type FrameworkAdapterConfig struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type FrameworkAdapterSymbol struct {
	Kind          string // event, export, callback, player, or job
	Name          string
	Detail        string
	Documentation string
}

type FrameworkAdapter struct {
	Name            string
	Version         string
	KnownGlobals    []string
	Symbols         []FrameworkAdapterSymbol
	DiagnosticCodes []string
}

var frameworkAdapterPacks = []FrameworkAdapter{
	{Name: "esx", Version: "1", KnownGlobals: []string{"ESX"}, Symbols: []FrameworkAdapterSymbol{
		{Kind: "event", Name: "esx:playerLoaded", Detail: "ESX event"},
		{Kind: "event", Name: "esx:getSharedObject", Detail: "ESX event"},
		{Kind: "export", Name: "es_extended:getSharedObject", Detail: "ESX export"},
		{Kind: "callback", Name: "ESX.TriggerServerCallback", Detail: "ESX callback"},
		{Kind: "callback", Name: "ESX.RegisterServerCallback", Detail: "ESX callback"},
		{Kind: "player", Name: "ESX.GetPlayerData", Detail: "ESX player API"},
		{Kind: "job", Name: "xPlayer.getJob", Detail: "ESX job API"},
	}},
	{Name: "qbcore", Version: "1", KnownGlobals: []string{"QBCore"}, Symbols: []FrameworkAdapterSymbol{
		{Kind: "event", Name: "QBCore:Client:OnPlayerLoaded", Detail: "QBCore event"},
		{Kind: "event", Name: "QBCore:Server:OnPlayerLoaded", Detail: "QBCore event"},
		{Kind: "export", Name: "qb-core:GetCoreObject", Detail: "QBCore export"},
		{Kind: "callback", Name: "QBCore.Functions.TriggerCallback", Detail: "QBCore callback"},
		{Kind: "callback", Name: "QBCore.Functions.CreateCallback", Detail: "QBCore callback"},
		{Kind: "player", Name: "QBCore.Functions.GetPlayerData", Detail: "QBCore player API"},
		{Kind: "job", Name: "PlayerData.job", Detail: "QBCore job API"},
	}},
	{Name: "ox", Version: "1", KnownGlobals: []string{"lib"}, Symbols: []FrameworkAdapterSymbol{
		{Kind: "event", Name: "ox:playerLoaded", Detail: "ox event"},
		{Kind: "export", Name: "ox_inventory:Items", Detail: "ox export"},
		{Kind: "callback", Name: "lib.callback.register", Detail: "ox callback"},
		{Kind: "callback", Name: "lib.callback.await", Detail: "ox callback"},
		{Kind: "player", Name: "ESX.GetPlayerData", Detail: "ox-compatible player API"},
		{Kind: "job", Name: "playerState.job", Detail: "ox job API"},
	}},
}

// FrameworkAdapterPacks returns the declarative built-in packs.
func FrameworkAdapterPacks() []FrameworkAdapter { return frameworkAdapterPacks }

// SelectFrameworkAdapters resolves configured names and versions. Unknown or
// incompatible entries are ignored so adding a typo cannot change diagnostics.
func SelectFrameworkAdapters(config []FrameworkAdapterConfig) []FrameworkAdapter {
	return selectFrameworkAdapters(config)
}

func selectFrameworkAdapters(config []FrameworkAdapterConfig) []FrameworkAdapter {
	var out []FrameworkAdapter
	for _, want := range config {
		if want.Enabled != nil && !*want.Enabled {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(want.Name))
		for _, pack := range frameworkAdapterPacks {
			if pack.Name == name && adapterVersionMatches(want.Version, pack.Version) {
				out = append(out, pack)
				break
			}
		}
	}
	return out
}

func adapterVersionMatches(requested, available string) bool {
	if requested == "" || requested == available {
		return true
	}
	// Packs advertise a major line (for example "1"); accept a pinned
	// semver in that line without pretending to model framework behavior.
	return strings.HasPrefix(requested, available+".")
}

func frameworkAdaptersEqual(a, b []FrameworkAdapter) bool { return reflect.DeepEqual(a, b) }

func adapterCompletions(adapters []FrameworkAdapter) []CompletionItem {
	var items []CompletionItem
	for _, adapter := range adapters {
		for _, symbol := range adapter.Symbols {
			item := CompletionItem{Label: symbol.Name, Kind: FunctionCompletion, Detail: symbol.Detail}
			if symbol.Documentation != "" {
				item.Documentation = &MarkupContent{Kind: "markdown", Value: symbol.Documentation}
			}
			items = append(items, item)
		}
	}
	return items
}
