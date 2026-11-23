package lsp

import (
	"strings"
	"testing"

	"github.com/FRFlo/lugo/ast"
	"github.com/FRFlo/lugo/parser"
)

func TestLuaDiagnosticQuickFix(t *testing.T) {
	for _, tc := range []struct {
		name, src, code, marker, want string
		last                          bool
	}{
		{"empty parameters", "local function f() return ... end", "incorrect-vararg", "...", "local function f(...) return ... end", false},
		{"nested function", "local function f(...) local function g(a) return ... end end", "incorrect-vararg", "...", "local function f(...) local function g(a, ...) return ... end end", true},
		{"explicit nil", "local a,b,c = 1, (f())", "unbalanced-assignment", "c", "local a,b,c = 1, (f()), nil", true},
		{"dynamic return refused", "local a,b = f()", "unbalanced-assignment", "b", "", false},
		{"false branch", "if false then local a = 1 else local b = 2 end", "constant-condition", "false", "do\nlocal b = 2\nend", false},
		{"unknown branch refused", "if false then x() elseif f() then y() end", "constant-condition", "false", "", false},
		{"comments refused", "if false then -- retain\n x() end", "constant-condition", "false", "", false},
		{"identical fields", "local x = {a=1, b=2, a=1}", "duplicate-field", "a", "local x = {b=2, a=1}", true},
		{"different fields refused", "local x = {a=1, a=2}", "duplicate-field", "a", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			tree := ast.NewTree(src)
			p := parser.New(src, tree, 50)
			p.Parse()
			if len(p.Errors) != 0 {
				t.Fatalf("fixture parse: %v", p.Errors)
			}
			idx := strings.Index(tc.src, tc.marker)
			if tc.last {
				idx = strings.LastIndex(tc.src, tc.marker)
			}
			if idx < 0 {
				t.Fatal("missing marker")
			}
			diag := Diagnostic{Code: tc.code, Range: getRange(tree, uint32(idx), uint32(idx+len(tc.marker)))}
			action, ok := luaDiagnosticQuickFix(&Document{Tree: tree}, "file:///test.lua", diag)
			if tc.want == "" {
				if ok {
					t.Fatalf("unexpected action: %+v", action)
				}
				return
			}
			if !ok {
				t.Fatal("missing action")
			}
			if action.IsPreferred || action.Kind != "quickfix" {
				t.Fatalf("unexpected action flags: %+v", action)
			}
			edits := action.Edit.Changes["file:///test.lua"]
			if len(edits) != 1 {
				t.Fatalf("edits: %v", edits)
			}
			edit := edits[0]
			start := tree.Offset(edit.Range.Start.Line, edit.Range.Start.Character)
			end := tree.Offset(edit.Range.End.Line, edit.Range.End.Character)
			got := tc.src[:start] + edit.NewText + tc.src[end:]
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			newTree := ast.NewTree([]byte(got))
			next := parser.New([]byte(got), newTree, 50)
			next.Parse()
			if len(next.Errors) != 0 {
				t.Fatalf("result parse: %v", next.Errors)
			}
		})
	}
}
