package lsp

import "testing"

func TestTypeEqual(t *testing.T) {
	t.Run("primitive_equality", func(t *testing.T) {
		left := Type{Primitive: TypeNumber | TypeString}
		right := Type{Primitive: TypeNumber | TypeString}

		if !left.Equal(right) {
			t.Fatalf("%v should equal %v", left, right)
		}

		if left.Equal(Type{Primitive: TypeNumber}) {
			t.Fatalf("union primitive should not equal narrower primitive")
		}
	})

	t.Run("structural_equality", func(t *testing.T) {
		left := Type{Structural: &StructuralType{Fields: map[string]Type{
			"id":   {Primitive: TypeNumber},
			"name": {Primitive: TypeString},
		}}}
		right := Type{Structural: &StructuralType{Fields: map[string]Type{
			"name": {Primitive: TypeString},
			"id":   {Primitive: TypeNumber},
		}}}

		if !left.Equal(right) {
			t.Fatalf("structurally identical table types should compare equal")
		}

		right.Structural.Fields["name"] = Type{Primitive: TypeNumber}
		if left.Equal(right) {
			t.Fatalf("different field types should not compare equal")
		}
	})

	t.Run("interned_structural_pointer_equality", func(t *testing.T) {
		pool := NewTypePool()
		left := pool.Intern(StructuralType{Fields: map[string]Type{"ok": {Primitive: TypeBoolean}}})
		right := pool.Intern(StructuralType{Fields: map[string]Type{"ok": {Primitive: TypeBoolean}}})

		if !(Type{Structural: left}).Equal(Type{Structural: right}) {
			t.Fatalf("interned structural types should compare equal")
		}
		if left != right {
			t.Fatalf("interned structural types should share canonical pointer")
		}
	})
}

func TestTypeUnion(t *testing.T) {
	t.Run("primitive_union_and_intersection", func(t *testing.T) {
		left := Type{Primitive: TypeNumber | TypeString}
		right := Type{Primitive: TypeString | TypeBoolean}

		if got, want := left.Union(right), (Type{Primitive: TypeNumber | TypeString | TypeBoolean}); !got.Equal(want) {
			t.Fatalf("union = %v, want %v", got, want)
		}

		if got, want := left.Intersect(right), (Type{Primitive: TypeString}); !got.Equal(want) {
			t.Fatalf("intersection = %v, want %v", got, want)
		}
	})

	t.Run("structural_union_merges_fields", func(t *testing.T) {
		left := Type{Structural: &StructuralType{Fields: map[string]Type{"x": {Primitive: TypeNumber}}}}
		right := Type{Structural: &StructuralType{Fields: map[string]Type{"y": {Primitive: TypeString}}}}

		got := left.Union(right)
		if got.Structural == nil {
			t.Fatalf("union should produce structural type")
		}
		if !got.Structural.Fields["x"].Equal(Type{Primitive: TypeNumber}) {
			t.Fatalf("union missing x:number field: %#v", got.Structural.Fields)
		}
		if !got.Structural.Fields["y"].Equal(Type{Primitive: TypeString}) {
			t.Fatalf("union missing y:string field: %#v", got.Structural.Fields)
		}
	})

	t.Run("structural_intersection_keeps_compatible_common_fields", func(t *testing.T) {
		left := Type{Structural: &StructuralType{Fields: map[string]Type{
			"shared": {Primitive: TypeNumber | TypeString},
			"left":   {Primitive: TypeBoolean},
		}}}
		right := Type{Structural: &StructuralType{Fields: map[string]Type{
			"shared": {Primitive: TypeString | TypeBoolean},
			"right":  {Primitive: TypeNil},
		}}}

		got := left.Intersect(right)
		if got.Structural == nil {
			t.Fatalf("intersection should produce structural type")
		}
		if len(got.Structural.Fields) != 1 {
			t.Fatalf("intersection fields = %#v, want only shared", got.Structural.Fields)
		}
		if !got.Structural.Fields["shared"].Equal(Type{Primitive: TypeString}) {
			t.Fatalf("shared field intersection = %v, want string", got.Structural.Fields["shared"])
		}
	})

	t.Run("structural_intersection_empty_is_nil", func(t *testing.T) {
		left := Type{Structural: &StructuralType{Fields: map[string]Type{"x": {Primitive: TypeNumber}}}}
		right := Type{Structural: &StructuralType{Fields: map[string]Type{"y": {Primitive: TypeString}}}}

		if got := left.Intersect(right); !got.Equal(Type{Primitive: TypeNil}) {
			t.Fatalf("empty structural intersection = %v, want nil", got)
		}
	})
}

func TestStructuralType(t *testing.T) {
	t.Run("function_shape", func(t *testing.T) {
		self := &StructuralType{Fields: map[string]Type{"id": {Primitive: TypeNumber}}, Readonly: true}
		fn := &StructuralType{Function: &FunctionType{
			Params:   []Type{{Primitive: TypeString}},
			Variadic: true,
			Returns:  []Type{{Primitive: TypeBoolean}},
			SelfType: self,
		}}

		if fn.Function == nil || !fn.Function.Variadic {
			t.Fatalf("function type did not preserve function metadata")
		}
		if fn.Function.SelfType != self {
			t.Fatalf("function type did not preserve self type")
		}
		if !(Type{Structural: fn}).Equal(Type{Structural: &StructuralType{Function: &FunctionType{
			Params:   []Type{{Primitive: TypeString}},
			Variadic: true,
			Returns:  []Type{{Primitive: TypeBoolean}},
			SelfType: &StructuralType{Fields: map[string]Type{"id": {Primitive: TypeNumber}}, Readonly: true},
		}}}) {
			t.Fatalf("equivalent function structural types should compare equal")
		}
	})

	t.Run("metatable_and_markers", func(t *testing.T) {
		meta := &StructuralType{Fields: map[string]Type{"__index": {Primitive: TypeTable}}}
		st := &StructuralType{Fields: map[string]Type{"value": {Primitive: TypeAny}}, Metatable: meta, Readonly: true, Const: true}

		if st.Metatable != meta || !st.Readonly || !st.Const {
			t.Fatalf("structural markers not preserved: %#v", st)
		}
	})
}
