package lsp

import (
	"sync"
	"testing"
)

func TestTypeInterning(t *testing.T) {
	t.Run("identical_tables_share_pointer", func(t *testing.T) {
		pool := NewTypePool()
		left := pool.Intern(StructuralType{Fields: map[string]Type{
			"id":   {Primitive: TypeNumber},
			"name": {Primitive: TypeString},
		}})
		right := pool.Intern(StructuralType{Fields: map[string]Type{
			"name": {Primitive: TypeString},
			"id":   {Primitive: TypeNumber},
		}})

		if left != right {
			t.Fatalf("identical tables interned to different pointers: %p != %p", left, right)
		}
	})

	t.Run("different_tables_keep_distinct_pointers", func(t *testing.T) {
		pool := NewTypePool()
		left := pool.Intern(StructuralType{Fields: map[string]Type{"id": {Primitive: TypeNumber}}})
		right := pool.Intern(StructuralType{Fields: map[string]Type{"id": {Primitive: TypeString}}})

		if left == right {
			t.Fatalf("different tables interned to same pointer: %p", left)
		}
	})

	t.Run("recursive_tables_do_not_loop", func(t *testing.T) {
		pool := NewTypePool()
		self := &StructuralType{Fields: map[string]Type{}}
		self.Metatable = self
		self.Fields["self"] = Type{Structural: self}

		got := pool.Intern(*self)
		if got == nil {
			t.Fatalf("recursive structural type interned to nil")
		}
		if got.Metatable != got {
			t.Fatalf("canonical recursive metatable should point at canonical type")
		}
		if got.Fields["self"].Structural != got {
			t.Fatalf("canonical recursive field should point at canonical type")
		}
	})

	t.Run("concurrent_access_is_canonical", func(t *testing.T) {
		pool := NewTypePool()
		const workers = 16
		results := make([]*StructuralType, workers)
		var wg sync.WaitGroup

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i] = pool.Intern(StructuralType{Fields: map[string]Type{"id": {Primitive: TypeNumber}}})
			}(i)
		}

		wg.Wait()
		for i := 1; i < workers; i++ {
			if results[i] != results[0] {
				t.Fatalf("result %d = %p, want %p", i, results[i], results[0])
			}
		}
	})
}
