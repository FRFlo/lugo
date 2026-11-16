package lsp

import (
	"testing"

	"github.com/FRFlo/lugo/token"
)

func TestMetatableOperatorHelperResolvesLuaNames(t *testing.T) {
	tests := []struct {
		op   token.Kind
		name string
	}{
		{token.Plus, "__add"}, {token.Minus, "__sub"},
		{token.Asterisk, "__mul"}, {token.Modulo, "__mod"},
		{token.Caret, "__pow"}, {token.Slash, "__div"},
		{token.FloorSlash, "__idiv"}, {token.BitAnd, "__band"},
		{token.BitOr, "__bor"}, {token.BitXor, "__bxor"},
		{token.ShiftLeft, "__shl"}, {token.ShiftRight, "__shr"},
		{token.Concat, "__concat"}, {token.Hash, "__len"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := LuaMetatableOperatorName(tc.op); !ok || got != tc.name {
				t.Fatalf("LuaMetatableOperatorName(%v) = %q, %v; want %q, true", tc.op, got, ok, tc.name)
			}
		})
	}
	if _, ok := LuaMetatableOperatorName(token.Eq); ok {
		t.Fatal("equality must not be resolved as an arithmetic-style operator")
	}
}

func TestMetatableUnaryOperatorHelperResolvesLuaNames(t *testing.T) {
	for _, tc := range []struct {
		op   token.Kind
		name string
	}{{token.Minus, "__unm"}, {token.BitXor, "__bnot"}, {token.Hash, "__len"}} {
		if got, ok := LuaMetatableUnaryOperatorName(tc.op); !ok || got != tc.name {
			t.Errorf("LuaMetatableUnaryOperatorName(%v) = %q, %v; want %q, true", tc.op, got, ok, tc.name)
		}
	}
	operand := TypeSet{Basics: TypeTable}
	want := TypeSet{Basics: TypeString}
	got, ok := InferMetatableUnaryOperator(token.Minus, operand, func(receiver TypeSet, name string) (TypeSet, bool) {
		if receiver.Basics != TypeTable || name != "__unm" {
			t.Fatalf("resolver received %#v, %q", receiver, name)
		}
		return want, true
	})
	if !ok || got.Basics != want.Basics {
		t.Fatalf("unary result = %#v, %v; want %#v, true", got, ok, want)
	}
}

func TestMetatableOperatorHelperPrefersLeftThenRight(t *testing.T) {
	left := TypeSet{Basics: TypeTable}
	right := TypeSet{Basics: TypeUserdata}
	want := TypeSet{Basics: TypeString}
	called := ""
	resolve := func(receiver TypeSet, name string) (TypeSet, bool) {
		if name != "__add" {
			t.Fatalf("unexpected metamethod %q", name)
		}
		if receiver.Basics == left.Basics {
			called = "left"
			return want, true
		}
		called = "right"
		return TypeSet{Basics: TypeBoolean}, true
	}

	got, ok := InferMetatableBinaryOperator(token.Plus, left, right, resolve)
	if !ok || got.Basics != want.Basics || called != "left" {
		t.Fatalf("left result = %#v, %v, called %q; want %#v, true, left", got, ok, called, want)
	}

	called = ""
	got, ok = InferMetatableBinaryOperator(token.Plus, TypeSet{}, right, resolve)
	if !ok || got.Basics != TypeBoolean || called != "right" {
		t.Fatalf("right fallback = %#v, %v, called %q; want boolean, true, right", got, ok, called)
	}
}

func TestMetatableCallOperator(t *testing.T) {
	callable := TypeSet{Basics: TypeTable}
	want := TypeSet{Basics: TypeString}
	got, ok := InferMetatableCallOperator(callable, func(receiver TypeSet, name string) (TypeSet, bool) {
		if receiver.Basics != TypeTable || name != "__call" {
			t.Fatalf("resolver received %#v, %q", receiver, name)
		}
		return want, true
	})
	if !ok || got.Basics != want.Basics {
		t.Fatalf("call result = %#v, %v; want %#v, true", got, ok, want)
	}
}
