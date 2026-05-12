package lsp

import "testing"

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
