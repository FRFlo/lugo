package lsp

import (
	"slices"
	"testing"
)

func TestCompletionResourceCache(t *testing.T) {
	cache := NewCompletionResourceCache()
	scope := &ResourceScope{
		URI: "file:///resource",
		Client: SymbolTable{
			"clientFn": &SymbolEntry{Type: Type{Structural: &StructuralType{Function: &FunctionType{}}}},
		},
		Server: SymbolTable{
			"sharedName": &SymbolEntry{Type: Type{Primitive: TypeString}},
		},
		Shared: SymbolTable{
			"sharedValue": &SymbolEntry{Type: Type{Primitive: TypeBoolean}},
			"sharedName":  &SymbolEntry{Type: Type{Primitive: TypeNumber}},
		},
	}

	items := cache.Build(scope)
	wantLabels := []string{"clientFn", "sharedName", "sharedValue"}
	gotLabels := make([]string, 0, len(items))
	for _, item := range items {
		gotLabels = append(gotLabels, item.Label)
	}
	if !slices.Equal(gotLabels, wantLabels) {
		t.Fatalf("labels = %#v, want %#v", gotLabels, wantLabels)
	}
	if items[0].Kind != FunctionCompletion || items[1].Kind != VariableCompletion || items[2].Kind != VariableCompletion {
		t.Fatalf("kinds = %#v, want function/variable/variable", items)
	}
	if items[0].Detail != string(GlobalIndexScopeClient) || items[1].Detail != string(GlobalIndexScopeServer) || items[2].Detail != string(GlobalIndexScopeShared) {
		t.Fatalf("details = %#v, want scope names", items)
	}

	got := cache.Get(scope.URI)
	if !slices.Equal(gotLabels, labelsFromItems(got)) {
		t.Fatalf("cache get labels = %#v, want %#v", labelsFromItems(got), gotLabels)
	}

	cache.Invalidate(scope.URI)
	if items := cache.Get(scope.URI); len(items) != 0 {
		t.Fatalf("cache still populated after invalidate: %#v", items)
	}
}

func TestCompletionResourceCacheNilInputs(t *testing.T) {
	cache := NewCompletionResourceCache()
	if items := cache.Build(nil); items != nil {
		t.Fatalf("Build(nil) = %#v, want nil", items)
	}
	if items := cache.Get("file:///missing"); items != nil {
		t.Fatalf("Get(missing) = %#v, want nil", items)
	}
}

func labelsFromItems(items []CompletionItem) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}
