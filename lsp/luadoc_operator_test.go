package lsp

import "testing"

func TestParseLuaDoc_Operators(t *testing.T) {
	doc := parseLuaDoc([]byte("@class Vector\n@operator add(Vector): Vector\n@operator mul(number): Vector\n@operator unm: Vector\n@operator call: number\n@operator index(string): any | nil"), false)

	want := []LuaDocOperator{
		{Operation: "add", InputType: "Vector", ResultType: "Vector"},
		{Operation: "mul", InputType: "number", ResultType: "Vector"},
		{Operation: "unm", ResultType: "Vector"},
		{Operation: "call", ResultType: "number"},
		{Operation: "index", InputType: "string", ResultType: "any | nil"},
	}
	if len(doc.Operators) != len(want) {
		t.Fatalf("got %d operators, want %d: %+v", len(doc.Operators), len(want), doc.Operators)
	}
	for i := range want {
		if doc.Operators[i] != want[i] {
			t.Errorf("operator %d = %+v, want %+v", i, doc.Operators[i], want[i])
		}
	}
}

func TestParseLuaDoc_InvalidOperatorsIgnored(t *testing.T) {
	doc := parseLuaDoc([]byte("@operator add(Vector) Vector\n@operator nonsense: number\n@operator sub(): number\n@operator call:"), false)
	if len(doc.Operators) != 0 {
		t.Fatalf("invalid operators should be ignored, got %+v", doc.Operators)
	}
}
