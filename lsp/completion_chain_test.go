package lsp

import (
	"slices"
	"testing"
)

func TestCompletionResourceCacheChainCompleteReturnsAllFields(t *testing.T) {
	cache := NewCompletionResourceCache()
	items := cache.ChainComplete(&Type{Structural: &StructuralType{Fields: map[string]Type{
		"alpha": {Primitive: TypeString},
		"beta":  {Structural: &StructuralType{Function: &FunctionType{}}},
		"gamma": {Primitive: TypeNumber},
	}}}, "")

	wantLabels := []string{"alpha", "beta", "gamma"}
	if gotLabels := labelsFromItems(items); !slices.Equal(gotLabels, wantLabels) {
		t.Fatalf("labels = %#v, want %#v", gotLabels, wantLabels)
	}
	if items[1].Kind != FunctionCompletion {
		t.Fatalf("beta kind = %v, want function", items[1].Kind)
	}
}

func TestCompletionResourceCacheChainCompleteFiltersByPrefix(t *testing.T) {
	cache := NewCompletionResourceCache()
	items := cache.ChainComplete(&Type{Structural: &StructuralType{Fields: map[string]Type{
		"alpha": {Primitive: TypeString},
		"beta":  {Primitive: TypeBoolean},
		"bravo": {Primitive: TypeNumber},
	}}}, "b")

	wantLabels := []string{"beta", "bravo"}
	if gotLabels := labelsFromItems(items); !slices.Equal(gotLabels, wantLabels) {
		t.Fatalf("labels = %#v, want %#v", gotLabels, wantLabels)
	}
}

func TestCompletionResourceCacheChainCompleteNilType(t *testing.T) {
	cache := NewCompletionResourceCache()
	if items := cache.ChainComplete(nil, ""); items != nil {
		t.Fatalf("ChainComplete(nil) = %#v, want nil", items)
	}
	if items := cache.ChainComplete(&Type{}, ""); items != nil {
		t.Fatalf("ChainComplete(empty type) = %#v, want nil", items)
	}
}

func TestCompletionResourceCacheChainCompleteFromEntry(t *testing.T) {
	cache := NewCompletionResourceCache()
	entry := &SymbolEntry{Type: Type{Structural: &StructuralType{Fields: map[string]Type{
		"delta": {Primitive: TypeString},
		"alpha": {Primitive: TypeNumber},
	}}}}

	items := cache.ChainCompleteFromEntry(entry, "")
	wantLabels := []string{"alpha", "delta"}
	if gotLabels := labelsFromItems(items); !slices.Equal(gotLabels, wantLabels) {
		t.Fatalf("labels = %#v, want %#v", gotLabels, wantLabels)
	}
}
