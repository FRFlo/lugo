package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignatureHelpInsideStringArgumentsAndNestedCalls(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("main.lua", `string = {}
function string.format(formatstring, ...) end
function tonumber(value, base) end
local result = tonumber(string.format("%." .. (places or 0) .. "f", tonumber("12")))
-- string.format("comment", 1)
`)
	h.reindex()

	for _, marker := range []struct {
		name       string
		text       string
		wantPrefix string
	}{
		{name: "format_spec", text: `"%."`, wantPrefix: "string.format("},
		{name: "format_value", text: `"f"`, wantPrefix: "string.format("},
		{name: "nested_tonumber", text: `"12"`, wantPrefix: "tonumber("},
	} {
		var signature *SignatureHelp
		uri := h.server.pathToURI(filepath.Join(h.root, "main.lua"))
		offset := strings.Index(string(h.server.Documents[uri].Source()), marker.text) + 1
		lineNo, col := h.server.Documents[uri].Tree.Position(uint32(offset))
		params, _ := json.Marshal(SignatureHelpParams{TextDocument: TextDocumentIdentifier{URI: uri}, Position: Position{Line: lineNo, Character: col}})
		h.resetRPC()
		h.server.handleSignatureHelp(Request{RPC: "2.0", ID: 1, Params: params})
		var envelope struct {
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(h.lastResponse(1), &envelope); err != nil {
			t.Fatal(err)
		}
		if string(envelope.Result) != "null" {
			var got SignatureHelp
			if err := json.Unmarshal(envelope.Result, &got); err != nil {
				t.Fatal(err)
			}
			signature = &got
		}
		if signature == nil || len(signature.Signatures) == 0 {
			t.Fatalf("signature at %s = %+v, want enclosing call signature", marker.name, signature)
		}
		foundFormat := false
		for _, got := range signature.Signatures {
			if strings.HasPrefix(got.Label, marker.wantPrefix) {
				foundFormat = true
			}
		}
		if !foundFormat {
			t.Fatalf("signature at %s = %+v, want %s", marker.name, signature, marker.wantPrefix)
		}
	}

	uri := h.server.pathToURI(filepath.Join(h.root, "main.lua"))
	line := strings.Split(string(h.server.Documents[uri].Source()), "\n")[4]
	offset := strings.Index(line, "string.format") + len("string.format") + 1
	lineNo, col := h.server.Documents[uri].Tree.Position(uint32(offset))
	params, _ := json.Marshal(SignatureHelpParams{TextDocument: TextDocumentIdentifier{URI: uri}, Position: Position{Line: lineNo, Character: col}})
	h.resetRPC()
	h.server.handleSignatureHelp(Request{RPC: "2.0", ID: 1, Params: params})
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(h.lastResponse(1), &envelope); err != nil {
		t.Fatal(err)
	}
	if string(envelope.Result) != "null" {
		t.Fatalf("signature inside comment = %s, want null", envelope.Result)
	}
}

func TestSignatureFromEntry(t *testing.T) {
	t.Run("FunctionType", func(t *testing.T) {
		entry := &SymbolEntry{Name: "sum", Type: Type{Structural: &StructuralType{Function: &FunctionType{Params: []Type{{Primitive: TypeNumber}, {Primitive: TypeString}}}}}}

		got := SignatureFromEntry(entry)
		if got == nil {
			t.Fatal("SignatureFromEntry returned nil")
		}
		if got.Label != "sum(param1, param2)" {
			t.Fatalf("label = %q, want sum(param1, param2)", got.Label)
		}
		if len(got.Parameters) != 2 || got.Parameters[0].Label != "param1" || got.Parameters[1].Label != "param2" {
			t.Fatalf("parameters = %+v, want param1/param2", got.Parameters)
		}
	})

	t.Run("NonFunctionReturnsNil", func(t *testing.T) {
		if got := SignatureFromEntry(&SymbolEntry{Name: "value", Type: Type{Primitive: TypeString}}); got != nil {
			t.Fatalf("SignatureFromEntry returned %+v, want nil", got)
		}
	})

	t.Run("MethodCall", func(t *testing.T) {
		objType := &Type{Structural: &StructuralType{Fields: map[string]Type{
			"run": {Structural: &StructuralType{Function: &FunctionType{Params: []Type{{Primitive: TypeString}}}}},
		}}}

		got := SignatureHelpForMethodCall("run", objType, nil, "")
		if got == nil {
			t.Fatal("SignatureHelpForMethodCall returned nil")
		}
		if len(got.Signatures) != 1 || got.Signatures[0].Label != "run(param1)" {
			t.Fatalf("signature help = %+v, want run(param1)", got)
		}
	})
}
