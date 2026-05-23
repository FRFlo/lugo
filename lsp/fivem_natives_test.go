package lsp

import "testing"

func TestNativeIntegration(t *testing.T) {
	t.Run("StructuralCatalog", func(t *testing.T) {
		idx := NewGlobalIndex()
		res := &FiveMResource{Name: "runtime", RootURI: "runtime", Games: []string{"gta5"}, FXVersion: "cerulean", IsCfxV2: true, UseExperimentalOAL: true, ClientGlobs: []string{"client.lua"}, ServerGlobs: []string{"server.lua"}}
		scope := idx.RegisterFiveMResource(res)
		if scope == nil {
			t.Fatal("RegisterFiveMResource returned nil")
		}
		registerTestNativeCatalog(t, idx, scope, res)
		playerPed := idx.LookupFiveMNative("runtime", GlobalIndexScopeClient, "PlayerPedId")
		if playerPed == nil {
			t.Fatal("PlayerPedId native missing from client-visible catalog")
		}
		assertFunctionShape(t, playerPed.Type, 0, []BasicType{TypeNumber})
		if playerPed.FiveM == nil || playerPed.FiveM.Bundle != "natives_universal.lua" || !playerPed.FiveM.UseExperimentalOAL {
			t.Fatalf("PlayerPedId FiveM metadata = %+v, want universal/OAL native metadata", playerPed.FiveM)
		}
		if playerPed.LuaDoc == nil || playerPed.LuaDoc.Description == "" || len(playerPed.LuaDoc.Returns) != 1 {
			t.Fatalf("PlayerPedId LuaDoc = %+v, want description and return metadata", playerPed.LuaDoc)
		}
		passengers := idx.LookupFiveMNative("runtime", GlobalIndexScopeClient, "GetVehicleMaxNumberOfPassengers")
		if passengers == nil {
			t.Fatal("GetVehicleMaxNumberOfPassengers native missing")
		}
		assertFunctionShape(t, passengers.Type, 1, []BasicType{TypeNumber})
		if passengers.Type.Structural.Function.Params[0].Primitive != TypeNumber {
			t.Fatalf("vehicle param type = %v, want number/integer", passengers.Type.Structural.Function.Params[0].Primitive)
		}
		shared := idx.LookupFiveMNative("runtime", GlobalIndexScopeClient, "GetInvokingResource")
		if shared == nil || scope.Shared["GetInvokingResource"] == nil {
			t.Fatalf("GetInvokingResource = %+v shared table = %+v, want shared native", shared, scope.Shared["GetInvokingResource"])
		}
	})

	t.Run("NoManifestAndUnsupportedGameUsesGTA", func(t *testing.T) {
		idx := NewGlobalIndex()
		if res := idx.RegisterFiveMResource(&FiveMResource{}); res != nil {
			t.Fatalf("empty no-manifest resource registered: %+v", res)
		}
		redMRes := &FiveMResource{Name: "redm", RootURI: "redm", Games: []string{"rdr3"}, FXVersion: "cerulean", IsCfxV2: true, ClientGlobs: []string{"client.lua"}}
		redMScope := idx.RegisterFiveMResource(redMRes)
		registerTestNativeCatalog(t, idx, redMScope, redMRes)
		native := idx.LookupFiveMNative("redm", GlobalIndexScopeClient, "PlayerPedId")
		if native == nil {
			t.Fatal("FiveM GTA native PlayerPedId missing for unsupported game fallback")
		}
		if native.FiveM == nil || native.FiveM.Bundle != "natives_universal.lua" || native.FiveM.Family != FiveMNativeFamilyGTA5 {
			t.Fatalf("fallback metadata = %+v, want natives_universal.lua family GTA5", native.FiveM)
		}
	})
}

func registerTestNativeCatalog(t testing.TB, idx *GlobalIndex, scope *ResourceScope, res *FiveMResource) {
	t.Helper()
	catalog := NewFiveMNativeCatalog(newTestFiveMNativeBundleLoader(t))
	idx.mu.Lock()
	idx.clearFiveMGeneratedSymbolsLocked(scope)
	idx.registerFiveMNativeCatalogLocked(scope, res, catalog)
	idx.rebuildHashIndexLocked()
	idx.mu.Unlock()
}

func TestExportBridge(t *testing.T) {
	idx := NewGlobalIndex()
	idx.RegisterFiveMResource(&FiveMResource{Name: "bank", RootURI: "bank", ClientExports: []string{"pay", "sharedPing"}, ServerExports: []string{"audit", "sharedPing"}})
	idx.RegisterFiveMResource(&FiveMResource{Name: "shop", RootURI: "shop", Dependencies: []string{"bank"}, ClientGlobs: []string{"client.lua"}})
	idx.RegisterFiveMResource(&FiveMResource{Name: "intruder", RootURI: "intruder", ClientGlobs: []string{"client.lua"}})
	pay := idx.LookupFiveMExport("shop", "bank", GlobalIndexScopeClient, "pay")
	if pay == nil || pay.Export == nil || pay.Export.Scope != GlobalIndexScopeClient {
		t.Fatalf("client export pay = %+v, want client export visible through dependency", pay)
	}
	assertFunctionShape(t, pay.Type, 0, []BasicType{TypeAny})
	if got := idx.LookupFiveMExport("shop", "bank", GlobalIndexScopeClient, "audit"); got != nil {
		t.Fatalf("server-only export leaked to client: %+v", got)
	}
	if got := idx.LookupFiveMExport("shop", "bank", GlobalIndexScopeServer, "audit"); got == nil {
		t.Fatal("server export audit missing from server side")
	}
	if got := idx.LookupFiveMExport("shop", "bank", GlobalIndexScopeClient, "sharedPing"); got == nil || got.Export.Scope != GlobalIndexScopeShared {
		t.Fatalf("shared export = %+v, want shared side propagation", got)
	}
	if got := idx.LookupFiveMExport("intruder", "bank", GlobalIndexScopeClient, "pay"); got != nil {
		t.Fatalf("export visible without dependency: %+v", got)
	}
}

func TestScopeFiltering(t *testing.T) {
	idx := NewGlobalIndex()
	catalog := NewFiveMNativeCatalog(func(name string) ([]byte, error) {
		switch name {
		case "natives_universal.lua":
			return []byte(`---@meta
---@return integer
function ClientOnlyNative() end

---@return string
function SharedNative() end
`), nil
		case "natives_server.lua":
			return []byte(`---@meta
---@return boolean
function ServerOnlyNative() end

---@return string
function SharedNative() end
`), nil
		default:
			return []byte(`---@meta
`), nil
		}
	})
	scope := idx.RegisterFiveMResource(&FiveMResource{Name: "scoped", RootURI: "scoped", Games: []string{"gta5"}, FXVersion: "cerulean", IsCfxV2: true})
	idx.mu.Lock()
	idx.clearFiveMGeneratedSymbolsLocked(scope)
	idx.registerFiveMNativeCatalogLocked(scope, &FiveMResource{Name: "scoped", RootURI: "scoped", Games: []string{"gta5"}, FXVersion: "cerulean", IsCfxV2: true}, catalog)
	idx.rebuildHashIndexLocked()
	idx.mu.Unlock()
	if got := idx.LookupFiveMNative("scoped", GlobalIndexScopeClient, "ClientOnlyNative"); got == nil {
		t.Fatal("client native missing from client scope")
	}
	if got := idx.LookupFiveMNative("scoped", GlobalIndexScopeClient, "ServerOnlyNative"); got != nil {
		t.Fatalf("server native leaked to client scope: %+v", got)
	}
	if got := idx.LookupFiveMNative("scoped", GlobalIndexScopeServer, "ClientOnlyNative"); got != nil {
		t.Fatalf("client native leaked to server scope: %+v", got)
	}
	if got := idx.LookupFiveMNative("scoped", GlobalIndexScopeShared, "SharedNative"); got == nil || got.FiveM.Scope != GlobalIndexScopeShared {
		t.Fatalf("shared native = %+v, want shared scope", got)
	}
}

func TestCompiledNativeLuaDocMultipleReturns(t *testing.T) {
	native := compiledFiveMNative{
		Name:        "CompiledMultiReturn",
		Description: "---Compiled multi-return fixture.",
		Params: []compiledFiveMNativeParam{
			{Name: "entity", Type: "number"},
		},
		Returns: []string{"boolean", "integer"},
	}
	luadoc := compiledFiveMNativeLuaDoc(native)
	entry := fiveMNativeCatalogEntry{
		Name:       native.Name,
		Bundle:     "natives_universal.lua",
		LuaDoc:     luaDocDataFromLuaDoc(luadoc),
		ParamNames: []string{"entity"},
	}
	entry.Type = structuralFunctionTypeFromLuaDoc(entry.ParamNames, luadoc.Params, luadoc.Returns)

	if len(entry.LuaDoc.Returns) != 2 {
		t.Fatalf("compiled LuaDoc returns = %+v, want two return annotations", entry.LuaDoc)
	}
	assertFunctionShape(t, entry.Type, 1, []BasicType{TypeBoolean, TypeNumber})
}

func assertFunctionShape(t *testing.T, typ Type, wantParams int, wantReturns []BasicType) {
	t.Helper()
	if typ.Primitive&TypeFunction == 0 || typ.Structural == nil || typ.Structural.Function == nil {
		t.Fatalf("type = %+v, want structural function", typ)
	}
	fn := typ.Structural.Function
	if len(fn.Params) != wantParams {
		t.Fatalf("param count = %d, want %d", len(fn.Params), wantParams)
	}
	if len(fn.Returns) != len(wantReturns) {
		t.Fatalf("return count = %d, want %d", len(fn.Returns), len(wantReturns))
	}
	for i, want := range wantReturns {
		if fn.Returns[i].Primitive&want == 0 {
			t.Fatalf("return %d primitive = %v, want %v", i, fn.Returns[i].Primitive, want)
		}
	}
}
