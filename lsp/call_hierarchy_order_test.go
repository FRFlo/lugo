package lsp

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

func TestCallHierarchyResultOrderIsStable(t *testing.T) {
	want := []CallHierarchyItem{
		{URI: "file:///a.lua", Name: "a", SelectionRange: Range{Start: Position{Line: 1}}},
		{URI: "file:///a.lua", Name: "b", SelectionRange: Range{Start: Position{Line: 1}}},
		{URI: "file:///b.lua", Name: "a", SelectionRange: Range{Start: Position{Line: 0}}},
	}
	var baseline []CallHierarchyItem
	for run := range 100 {
		got := append([]CallHierarchyItem(nil), want...)
		rng := rand.New(rand.NewSource(int64(run)))
		rng.Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		sort.Slice(got, func(i, j int) bool { return callHierarchyItemLess(got[i], got[j]) })
		if run == 0 {
			baseline = got
			continue
		}
		for i := range got {
			if !reflect.DeepEqual(got[i], baseline[i]) {
				t.Fatalf("run %d order differs at %d: got %+v, want %+v", run, i, got, baseline)
			}
		}
	}
}

func TestCallHierarchyRangesAreOrdered(t *testing.T) {
	ranges := []Range{
		{Start: Position{Line: 2, Character: 3}},
		{Start: Position{Line: 1, Character: 4}},
		{Start: Position{Line: 1, Character: 2}},
	}
	for range 5 {
		sort.Slice(ranges, func(i, j int) bool { return callHierarchyRangeLess(ranges[i], ranges[j]) })
	}
	if ranges[0].Start != (Position{Line: 1, Character: 2}) || ranges[1].Start != (Position{Line: 1, Character: 4}) || ranges[2].Start.Line != 2 {
		t.Fatalf("ranges are not in source order: %+v", ranges)
	}
}
