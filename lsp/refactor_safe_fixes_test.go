package lsp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSourceFixAllWithoutClientDiagnostics(t *testing.T) {
	s, root := newFiveMProfileTestServer(t)
	uri := s.pathToURI(root + "/main.lua")
	doc := addFiveMTestDocument(t, s, root+"/main.lua", `local unusedValue = 42
local unusedCall = doSideEffect()
`)
	var output bytes.Buffer
	s.Writer = &output

	request := func(only []string, diagnostics []Diagnostic) []CodeAction {
		t.Helper()
		params, err := json.Marshal(CodeActionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Context:      CodeActionContext{Only: only, Diagnostics: diagnostics},
		})
		if err != nil {
			t.Fatal(err)
		}
		output.Reset()
		s.handleCodeAction(Request{RPC: "2.0", ID: 1, Params: params})
		var envelope struct {
			Result json.RawMessage `json:"result"`
		}
		response := output.Bytes()
		separator := bytes.Index(response, []byte("\r\n\r\n"))
		if separator < 0 {
			t.Fatalf("missing response header: %q", response)
		}
		if err := json.Unmarshal(response[separator+4:], &envelope); err != nil {
			t.Fatal(err)
		}
		var actions []CodeAction
		if err := json.Unmarshal(envelope.Result, &actions); err != nil {
			t.Fatal(err)
		}
		return actions
	}

	fixes := s.getSafeFixesForDocument(doc)
	if len(fixes) == 0 || len(fixes[0].Coverage) == 0 {
		t.Fatal("fixture must contain safe fixes")
	}
	diag := Diagnostic{Code: "unused-local", Data: float64(fixes[0].Coverage[0])}
	for _, tc := range []struct {
		name  string
		only  []string
		diags []Diagnostic
		want  bool
	}{
		{name: "explicit fixAll without diagnostics", only: []string{"source.fixAll"}, want: true},
		{name: "source parent filter", only: []string{"source"}, want: true},
		{name: "diagnostic driven fixAll", only: []string{"source.fixAll"}, diags: []Diagnostic{diag}, want: true},
		{name: "ordinary request", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			found := false
			for _, action := range request(tc.only, tc.diags) {
				if action.Kind == "source.fixAll" {
					found = true
					if len(action.Edit.Changes[uri]) == 0 {
						t.Fatal("source.fixAll has no edits")
					}
				}
			}
			if found != tc.want {
				t.Fatalf("source.fixAll present = %v, want %v", found, tc.want)
			}
		})
	}
}

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
