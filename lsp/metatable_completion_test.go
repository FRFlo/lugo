package lsp

import (
	"slices"
	"strings"
	"testing"
)

func TestMetatableCompletionPrefersOwnFieldsAndIncludesIndexMembers(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { inherited = true, run = function(self, amount) end }
local mt = { __index = base }
local value = setmetatable({ own = true, run = function(self, ownArg) end }, mt)
value:`+"--[[@completion]]")
	h.reindex()

	items := h.completion("completion")
	labels := labelsFromItems(items.Items)
	for _, label := range []string{"own", "inherited", "run"} {
		if !slices.Contains(labels, label) {
			t.Errorf("completion labels = %#v, want %q", labels, label)
		}
	}
	if got := completionItemByLabel(items, "run"); got == nil || got.InsertTextFormat != SnippetTextFormat || !strings.Contains(got.InsertText, "ownArg") || strings.Contains(got.InsertText, "amount") {
		t.Errorf("own run completion = %#v, want own method snippet", got)
	}
}

func TestMetatableCompletionIgnoresIndexFieldsOnIndexTable(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local first = { firstOnly = true }
local second = { secondOnly = true }
first.__index = second
second.__index = first
local value = setmetatable({}, first)
value.`+"--[[@completion]]")
	h.reindex()

	labels := labelsFromItems(h.completion("completion").Items)
	if !slices.Contains(labels, "secondOnly") || slices.Contains(labels, "firstOnly") {
		t.Fatalf("completion labels = %#v, want secondOnly without firstOnly", labels)
	}
}

func TestMetatableCompletionUsesMetatableOfIndexTable(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local ancestor = { inherited = true }
local unrelated = { unrelatedField = true }
local base = setmetatable({ ownBase = true, __index = unrelated }, { __index = ancestor })
local value = setmetatable({}, { __index = base })
value.`+"--[[@completion]]")
	h.reindex()
	labels := labelsFromItems(h.completion("completion").Items)
	for _, label := range []string{"ownBase", "inherited"} {
		if !slices.Contains(labels, label) {
			t.Errorf("completion labels = %#v, want %q", labels, label)
		}
	}
	if slices.Contains(labels, "unrelatedField") {
		t.Errorf("completion followed base.__index as a metatable: %#v", labels)
	}
}

func TestMetatableCompletionDoesNotFollowOrdinaryIndexField(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local unrelated = { unrelatedField = true }
local base = { ownBase = true, __index = unrelated }
local value = setmetatable({}, { __index = base })
value.`+"--[[@completion]]")
	h.reindex()
	labels := labelsFromItems(h.completion("completion").Items)
	if !slices.Contains(labels, "ownBase") || slices.Contains(labels, "unrelatedField") {
		t.Fatalf("completion labels = %#v, want ownBase without unrelatedField", labels)
	}
}

func TestMetatableCompletionRespectsIndexAttachmentOrder(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local root = { inherited = true }
local base = {}
local object = setmetatable({}, { __index = base })
local first = object.i--[[@before_index_attach]]
setmetatable(base, { __index = root })
local second = object.i--[[@after_index_attach]]
`)
	h.reindex()
	before := labelsFromItems(h.completion("before_index_attach").Items)
	after := labelsFromItems(h.completion("after_index_attach").Items)
	if slices.Contains(before, "inherited") || !slices.Contains(after, "inherited") {
		t.Fatalf("completion before = %#v, after = %#v; want inherited only after attachment", before, after)
	}
}

func TestMetatableCompletionIndexReplacementFollowsAccessOrder(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local first = { visibleFirst = true }
local second = { visibleSecond = true }
local mt = { __index = first }
local object = setmetatable({}, mt)
local a = object.v--[[@completion_index_before]]
mt.__index = second
local b = object.v--[[@completion_index_after]]
mt.__index = nil
local c = object.v--[[@completion_index_clear]]
`)
	h.reindex()
	before := labelsFromItems(h.completion("completion_index_before").Items)
	after := labelsFromItems(h.completion("completion_index_after").Items)
	cleared := labelsFromItems(h.completion("completion_index_clear").Items)
	if !slices.Contains(before, "visibleFirst") || slices.Contains(before, "visibleSecond") || !slices.Contains(after, "visibleSecond") || slices.Contains(after, "visibleFirst") || slices.Contains(cleared, "visibleFirst") || slices.Contains(cleared, "visibleSecond") {
		t.Fatalf("completion before=%v after=%v cleared=%v", before, after, cleared)
	}
}

func TestMetatableCompletionIncludesStringIndexFields(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { ["\u{61}nswer"] = 42, ["run"] = function(self, amount) end }
local value = setmetatable({}, { __index = base })
value:--[[@indexed_fields]]
`)
	h.reindex()
	completion := h.completion("indexed_fields")
	for _, label := range []string{"answer", "run"} {
		if completionItemByLabel(completion, label) == nil {
			t.Errorf("completion %q missing: %#v", label, completion.Items)
		}
	}
}

func TestMetatableCompletionIncludesAssignedStringIndexFields(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = {}
base["answer"] = 42
local value = setmetatable({}, { __index = base })
value.--[[@assigned_index_field]]
`)
	h.reindex()
	completion := h.completion("assigned_index_field")
	if completionItemByLabel(completion, "answer") == nil {
		t.Fatalf("assigned string-key field missing: %#v", completion.Items)
	}
}

func TestMetatableCompletionExcludesClearedStringIndexField(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = {}
base["answer"] = 42
local value = setmetatable({}, { __index = base })
local before = value.a--[[@index_field_before_clear]]
base["answer"] = nil
local after = value.a--[[@index_field_after_clear]]
`)
	h.reindex()
	before := h.completion("index_field_before_clear")
	after := h.completion("index_field_after_clear")
	if completionItemByLabel(before, "answer") == nil || completionItemByLabel(after, "answer") != nil {
		t.Fatalf("completion before=%#v after=%#v", before.Items, after.Items)
	}
}

func TestMetatableCompletionExcludesClearedLiteralField(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local base = { answer = 42 }
local value = setmetatable({}, { __index = base })
local before = value.a--[[@literal_before_clear]]
base.answer = nil
local after = value.a--[[@literal_after_clear]]
`)
	h.reindex()
	before := h.completion("literal_before_clear")
	after := h.completion("literal_after_clear")
	if completionItemByLabel(before, "answer") == nil || completionItemByLabel(after, "answer") != nil {
		t.Fatalf("completion before=%#v after=%#v", before.Items, after.Items)
	}
}

func BenchmarkMetatableCompletionIndexedFieldsLargeFile(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	var source strings.Builder
	source.WriteString("local base = {}\nbase[\"answer\"] = 42\n")
	for i := 0; i < 4000; i++ {
		source.WriteString("local unused = 1\n")
	}
	source.WriteString("local object = setmetatable({}, { __index = base })\nobject.a--[[@bench_index_completion]]\n")
	h.writeWorkspaceFile("main.lua", source.String())
	h.reindex()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = h.completion("bench_index_completion")
	}
}

func TestMetatableCompletionNilIntermediateFallsBackToAncestor(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `local root = { run = function(self, amount) end }
local middle = setmetatable({ run = nil }, { __index = root })
local object = setmetatable({}, { __index = middle })
object:--[[@nil_inherited_completion]]
`)
	h.reindex()
	item := completionItemByLabel(h.completion("nil_inherited_completion"), "run")
	if item == nil || !strings.Contains(item.InsertText, "amount") {
		t.Fatalf("completion for inherited method = %#v, want ancestor signature", item)
	}
}
