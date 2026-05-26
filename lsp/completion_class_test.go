package lsp

import (
	"slices"
	"testing"
)

func TestClassMethodCompletion(t *testing.T) {
	h := newFiveMFixtureHarness(t, "class_method_completion")
	h.requireSingleDefinitionAt("account_save_call", "account_save_definition")

	items := h.completion("completion_class_method")
	labels := labelsFromItems(items.Items)

	for _, label := range []string{"save", "deposit", "withdraw"} {
		if !slices.Contains(labels, label) {
			t.Fatalf("completion labels = %#v, want label %q", labels, label)
		}
	}
}
