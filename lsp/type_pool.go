package lsp

import (
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
)

type TypeHash uint64

// TypePool interns structural types so identical shapes share one pointer.
type TypePool struct {
	mu    sync.RWMutex
	types map[TypeHash][]*StructuralType
}

// NewTypePool creates a thread-safe pool for structural type interning.
func NewTypePool() *TypePool {
	return &TypePool{types: make(map[TypeHash][]*StructuralType)}
}

// Intern returns the canonical pointer for a structurally identical type.
func (pool *TypePool) Intern(st StructuralType) *StructuralType {
	if pool == nil {
		pool = NewTypePool()
	}

	canonical := canonicalizeStructuralType(&st, make(map[*StructuralType]*StructuralType))
	collapseRootEquivalentCycles(canonical)
	hash := TypeHashOf(canonical)

	pool.mu.RLock()
	for _, existing := range pool.types[hash] {
		if (Type{Structural: existing}).Equal(Type{Structural: canonical}) {
			pool.mu.RUnlock()
			return existing
		}
	}
	pool.mu.RUnlock()

	pool.mu.Lock()
	defer pool.mu.Unlock()
	for _, existing := range pool.types[hash] {
		if (Type{Structural: existing}).Equal(Type{Structural: canonical}) {
			return existing
		}
	}
	pool.types[hash] = append(pool.types[hash], canonical)

	return canonical
}

// TypeHashOf returns a stable hash for a structural type shape.
func TypeHashOf(st *StructuralType) TypeHash {
	hasher := fnv.New64a()
	writeStructuralHash(hasher, st, make(map[*StructuralType]int))

	return TypeHash(hasher.Sum64())
}

func canonicalizeStructuralType(st *StructuralType, seen map[*StructuralType]*StructuralType) *StructuralType {
	if st == nil {
		return nil
	}
	if clone, ok := seen[st]; ok {
		return clone
	}

	clone := &StructuralType{
		Readonly: st.Readonly,
		Const:    st.Const,
	}
	seen[st] = clone

	if len(st.Fields) > 0 {
		clone.Fields = make(map[string]Type, len(st.Fields))
		for name, fieldType := range st.Fields {
			clone.Fields[name] = canonicalizeType(fieldType, seen)
		}
	}
	clone.Metatable = canonicalizeStructuralType(st.Metatable, seen)
	clone.Function = canonicalizeFunctionType(st.Function, seen)

	return clone
}

func canonicalizeFunctionType(fn *FunctionType, seen map[*StructuralType]*StructuralType) *FunctionType {
	if fn == nil {
		return nil
	}

	clone := &FunctionType{
		Variadic: fn.Variadic,
		SelfType: canonicalizeStructuralType(fn.SelfType, seen),
	}
	if len(fn.Params) > 0 {
		clone.Params = make([]Type, len(fn.Params))
		for i := range fn.Params {
			clone.Params[i] = canonicalizeType(fn.Params[i], seen)
		}
	}
	if len(fn.Returns) > 0 {
		clone.Returns = make([]Type, len(fn.Returns))
		for i := range fn.Returns {
			clone.Returns[i] = canonicalizeType(fn.Returns[i], seen)
		}
	}

	return clone
}

func canonicalizeType(typ Type, seen map[*StructuralType]*StructuralType) Type {
	return Type{Primitive: typ.Primitive, Structural: canonicalizeStructuralType(typ.Structural, seen)}
}

func collapseRootEquivalentCycles(root *StructuralType) {
	if root == nil {
		return
	}
	collapseRootEquivalentCyclesIn(root, root, make(map[*StructuralType]bool))
}

func collapseRootEquivalentCyclesIn(root, current *StructuralType, seen map[*StructuralType]bool) {
	if current == nil || seen[current] {
		return
	}
	seen[current] = true

	if current.Metatable != nil {
		if current.Metatable != root && (Type{Structural: current.Metatable}).Equal(Type{Structural: root}) {
			current.Metatable = root
		} else {
			collapseRootEquivalentCyclesIn(root, current.Metatable, seen)
		}
	}
	for name, fieldType := range current.Fields {
		if fieldType.Structural == nil {
			continue
		}
		if fieldType.Structural != root && (Type{Structural: fieldType.Structural}).Equal(Type{Structural: root}) {
			fieldType.Structural = root
			current.Fields[name] = fieldType
			continue
		}
		collapseRootEquivalentCyclesIn(root, fieldType.Structural, seen)
	}
	if current.Function != nil {
		collapseRootEquivalentFunctionCycles(root, current.Function, seen)
	}
}

func collapseRootEquivalentFunctionCycles(root *StructuralType, fn *FunctionType, seen map[*StructuralType]bool) {
	if fn.SelfType != nil {
		if fn.SelfType != root && (Type{Structural: fn.SelfType}).Equal(Type{Structural: root}) {
			fn.SelfType = root
		} else {
			collapseRootEquivalentCyclesIn(root, fn.SelfType, seen)
		}
	}
	for i := range fn.Params {
		if fn.Params[i].Structural == nil {
			continue
		}
		if fn.Params[i].Structural != root && (Type{Structural: fn.Params[i].Structural}).Equal(Type{Structural: root}) {
			fn.Params[i].Structural = root
			continue
		}
		collapseRootEquivalentCyclesIn(root, fn.Params[i].Structural, seen)
	}
	for i := range fn.Returns {
		if fn.Returns[i].Structural == nil {
			continue
		}
		if fn.Returns[i].Structural != root && (Type{Structural: fn.Returns[i].Structural}).Equal(Type{Structural: root}) {
			fn.Returns[i].Structural = root
			continue
		}
		collapseRootEquivalentCyclesIn(root, fn.Returns[i].Structural, seen)
	}
}

type hashWriter interface {
	Write([]byte) (int, error)
}

func writeStructuralHash(w hashWriter, st *StructuralType, seen map[*StructuralType]int) {
	if st == nil {
		writeHashString(w, "<nil-struct>")
		return
	}
	if id, ok := seen[st]; ok {
		writeHashString(w, "<cycle:"+strconv.Itoa(id)+">")
		return
	}
	seen[st] = len(seen)

	writeHashString(w, "struct{")
	writeHashString(w, strconv.FormatBool(st.Readonly))
	writeHashString(w, strconv.FormatBool(st.Const))

	if st.Function != nil {
		writeFunctionHash(w, st.Function, seen)
	} else {
		writeHashString(w, "<nil-func>")
	}

	writeStructuralHash(w, st.Metatable, seen)

	keys := make([]string, 0, len(st.Fields))
	for key := range st.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeHashString(w, key)
		writeTypeHash(w, st.Fields[key], seen)
	}
	writeHashString(w, "}")
}

func writeFunctionHash(w hashWriter, fn *FunctionType, seen map[*StructuralType]int) {
	writeHashString(w, "func(")
	writeHashString(w, strconv.FormatBool(fn.Variadic))
	writeStructuralHash(w, fn.SelfType, seen)
	for _, param := range fn.Params {
		writeTypeHash(w, param, seen)
	}
	writeHashString(w, ")(")
	for _, ret := range fn.Returns {
		writeTypeHash(w, ret, seen)
	}
	writeHashString(w, ")")
}

func writeTypeHash(w hashWriter, typ Type, seen map[*StructuralType]int) {
	writeHashString(w, "type:")
	writeHashString(w, strconv.FormatUint(uint64(typ.Primitive), 10))
	writeStructuralHash(w, typ.Structural, seen)
}

func writeHashString(w hashWriter, value string) {
	_, _ = w.Write([]byte(strconv.Itoa(len(value))))
	_, _ = w.Write([]byte(":"))
	_, _ = w.Write([]byte(value))
	_, _ = w.Write([]byte(";"))
}
