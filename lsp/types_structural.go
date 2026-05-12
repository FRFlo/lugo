package lsp

// PrimitiveType reuses the existing BasicType bitmask during migration so the
// old TypeSet and the new structural Type can coexist without duplicate names.
type PrimitiveType = BasicType

// Type represents either primitive Lua values as a bitmask, a structural table
// or function shape, or both while the migration from TypeSet is in progress.
type Type struct {
	Primitive  PrimitiveType
	Structural *StructuralType
}

// StructuralType describes table-like and function-like Lua values.
type StructuralType struct {
	Fields    map[string]Type
	Metatable *StructuralType
	Readonly  bool
	Const     bool
	Function  *FunctionType
}

// FunctionType describes callable structural values, including method self.
type FunctionType struct {
	Params   []Type
	Variadic bool
	Returns  []Type
	SelfType *StructuralType
}

// Equal reports whether two types have the same primitive bits and structural
// shape. Interned structural values short-circuit through pointer equality.
func (typ Type) Equal(other Type) bool {
	if typ.Primitive != other.Primitive {
		return false
	}

	return structuralTypeEqual(typ.Structural, other.Structural, make(map[structuralPair]bool))
}

// Union combines primitive bits and merges structural fields.
func (typ Type) Union(other Type) Type {
	return Type{
		Primitive:  typ.Primitive | other.Primitive,
		Structural: unionStructuralTypes(typ.Structural, other.Structural),
	}
}

// Intersect keeps primitive bits and structural fields common to both types.
func (typ Type) Intersect(other Type) Type {
	result := Type{
		Primitive:  typ.Primitive & other.Primitive,
		Structural: intersectStructuralTypes(typ.Structural, other.Structural),
	}
	if result.Primitive == TypeUnknown && result.Structural == nil && typ.Structural != nil && other.Structural != nil {
		result.Primitive = TypeNil
	}

	return result
}

type structuralPair struct {
	left  *StructuralType
	right *StructuralType
}

func structuralTypeEqual(left, right *StructuralType, seen map[structuralPair]bool) bool {
	if left == right {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	if left.Readonly != right.Readonly || left.Const != right.Const {
		return false
	}

	pair := structuralPair{left: left, right: right}
	if seen[pair] {
		return true
	}
	seen[pair] = true

	if !functionTypeEqual(left.Function, right.Function, seen) {
		return false
	}
	if !structuralTypeEqual(left.Metatable, right.Metatable, seen) {
		return false
	}
	if len(left.Fields) != len(right.Fields) {
		return false
	}

	for name, leftType := range left.Fields {
		rightType, ok := right.Fields[name]
		if !ok || !typeEqualSeen(leftType, rightType, seen) {
			return false
		}
	}

	return true
}

func functionTypeEqual(left, right *FunctionType, seen map[structuralPair]bool) bool {
	if left == right {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	if left.Variadic != right.Variadic || len(left.Params) != len(right.Params) || len(left.Returns) != len(right.Returns) {
		return false
	}
	if !structuralTypeEqual(left.SelfType, right.SelfType, seen) {
		return false
	}

	for i := range left.Params {
		if !typeEqualSeen(left.Params[i], right.Params[i], seen) {
			return false
		}
	}
	for i := range left.Returns {
		if !typeEqualSeen(left.Returns[i], right.Returns[i], seen) {
			return false
		}
	}

	return true
}

func typeEqualSeen(left, right Type, seen map[structuralPair]bool) bool {
	return left.Primitive == right.Primitive && structuralTypeEqual(left.Structural, right.Structural, seen)
}

func unionStructuralTypes(left, right *StructuralType) *StructuralType {
	if left == nil {
		return cloneStructuralType(right, make(map[*StructuralType]*StructuralType))
	}
	if right == nil {
		return cloneStructuralType(left, make(map[*StructuralType]*StructuralType))
	}

	result := &StructuralType{
		Fields:    make(map[string]Type, len(left.Fields)+len(right.Fields)),
		Readonly:  left.Readonly && right.Readonly,
		Const:     left.Const && right.Const,
		Metatable: unionStructuralTypes(left.Metatable, right.Metatable),
		Function:  unionFunctionTypes(left.Function, right.Function),
	}

	for name, fieldType := range left.Fields {
		result.Fields[name] = fieldType
	}
	for name, fieldType := range right.Fields {
		if existing, ok := result.Fields[name]; ok {
			result.Fields[name] = existing.Union(fieldType)
		} else {
			result.Fields[name] = fieldType
		}
	}

	return result
}

func intersectStructuralTypes(left, right *StructuralType) *StructuralType {
	if left == nil || right == nil {
		return nil
	}

	result := &StructuralType{
		Fields:   make(map[string]Type),
		Readonly: left.Readonly && right.Readonly,
		Const:    left.Const && right.Const,
	}

	if meta := intersectStructuralTypes(left.Metatable, right.Metatable); meta != nil {
		result.Metatable = meta
	}
	if fn := intersectFunctionTypes(left.Function, right.Function); fn != nil {
		result.Function = fn
	}

	for name, leftType := range left.Fields {
		if rightType, ok := right.Fields[name]; ok {
			field := leftType.Intersect(rightType)
			if field.Primitive != TypeUnknown || field.Structural != nil {
				result.Fields[name] = field
			}
		}
	}

	if len(result.Fields) == 0 && result.Metatable == nil && result.Function == nil {
		return nil
	}

	return result
}

func unionFunctionTypes(left, right *FunctionType) *FunctionType {
	if left == nil {
		return cloneFunctionType(right, make(map[*StructuralType]*StructuralType))
	}
	if right == nil {
		return cloneFunctionType(left, make(map[*StructuralType]*StructuralType))
	}
	if len(left.Params) != len(right.Params) || len(left.Returns) != len(right.Returns) {
		return nil
	}

	result := &FunctionType{
		Params:   make([]Type, len(left.Params)),
		Variadic: left.Variadic || right.Variadic,
		Returns:  make([]Type, len(left.Returns)),
		SelfType: unionStructuralTypes(left.SelfType, right.SelfType),
	}
	for i := range left.Params {
		result.Params[i] = left.Params[i].Union(right.Params[i])
	}
	for i := range left.Returns {
		result.Returns[i] = left.Returns[i].Union(right.Returns[i])
	}

	return result
}

func intersectFunctionTypes(left, right *FunctionType) *FunctionType {
	if left == nil || right == nil || len(left.Params) != len(right.Params) || len(left.Returns) != len(right.Returns) {
		return nil
	}

	result := &FunctionType{
		Params:   make([]Type, len(left.Params)),
		Variadic: left.Variadic && right.Variadic,
		Returns:  make([]Type, len(left.Returns)),
		SelfType: intersectStructuralTypes(left.SelfType, right.SelfType),
	}
	for i := range left.Params {
		result.Params[i] = left.Params[i].Intersect(right.Params[i])
	}
	for i := range left.Returns {
		result.Returns[i] = left.Returns[i].Intersect(right.Returns[i])
	}

	return result
}

func cloneStructuralType(st *StructuralType, seen map[*StructuralType]*StructuralType) *StructuralType {
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
			clone.Fields[name] = cloneType(fieldType, seen)
		}
	}
	clone.Metatable = cloneStructuralType(st.Metatable, seen)
	clone.Function = cloneFunctionType(st.Function, seen)

	return clone
}

func cloneFunctionType(fn *FunctionType, seen map[*StructuralType]*StructuralType) *FunctionType {
	if fn == nil {
		return nil
	}

	clone := &FunctionType{
		Variadic: fn.Variadic,
		SelfType: cloneStructuralType(fn.SelfType, seen),
	}
	if len(fn.Params) > 0 {
		clone.Params = make([]Type, len(fn.Params))
		for i := range fn.Params {
			clone.Params[i] = cloneType(fn.Params[i], seen)
		}
	}
	if len(fn.Returns) > 0 {
		clone.Returns = make([]Type, len(fn.Returns))
		for i := range fn.Returns {
			clone.Returns[i] = cloneType(fn.Returns[i], seen)
		}
	}

	return clone
}

func cloneType(typ Type, seen map[*StructuralType]*StructuralType) Type {
	return Type{Primitive: typ.Primitive, Structural: cloneStructuralType(typ.Structural, seen)}
}
