package lsp

import (
	"strings"
	"testing"
)

func TestUnusedAssignmentFixesPreserveSideEffects(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	doc := addFiveMTestDocument(t, s, root+"/main.lua", `local unusedValue = 42
local unusedCall = doSideEffect()
`)

	fixes := s.getSafeFixesForDocument(doc)
	removedPureAssignment := false
	preservedCall := false
	for _, fix := range fixes {
		switch fix.Title {
		case "Remove unused assignment":
			removedPureAssignment = true
		case "Remove unused assignment (keep function call)":
			for _, edit := range fix.Edits {
				if strings.Contains(edit.NewText, "doSideEffect()") {
					preservedCall = true
				}
			}
		}
	}
	if !removedPureAssignment {
		t.Fatal("expected safe fix for an unused side-effect-free assignment")
	}
	if !preservedCall {
		t.Fatal("expected unused-call fix to preserve evaluation of the call")
	}
}
