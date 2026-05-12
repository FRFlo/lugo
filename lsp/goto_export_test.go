package lsp

import "testing"

func TestGotoExportTarget(t *testing.T) {
	idx := NewGlobalIndex()
	idx.EnsureResource("file:///consumer")
	idx.EnsureResource("file:///provider")
	idx.mu.Lock()
	idx.Resources["file:///provider"].Dependents = []ResourceURI{"file:///consumer"}
	idx.mu.Unlock()

	idx.AddSymbol("file:///provider", GlobalIndexScopeServer, "Ping", &SymbolEntry{Name: "Ping", Export: &ExportData{Name: "Ping", Resource: "file:///provider"}})

	got := GotoExportTarget("Ping", "file:///consumer", idx, GlobalIndexScopeServer)
	if got == nil || got.Export == nil || got.Export.Resource != "file:///provider" {
		t.Fatalf("GotoExportTarget = %+v, want provider export", got)
	}
}

func TestGotoLocalTarget(t *testing.T) {
	idx := NewGlobalIndex()
	idx.AddSymbol("file:///resource", GlobalIndexScopeClient, "LocalFn", &SymbolEntry{Name: "LocalFn", Type: Type{Structural: &StructuralType{Function: &FunctionType{}}}})

	got := GotoLocalTarget("LocalFn", "file:///resource", idx)
	if got == nil || got.Name != "LocalFn" {
		t.Fatalf("GotoLocalTarget = %+v, want local symbol", got)
	}
}
