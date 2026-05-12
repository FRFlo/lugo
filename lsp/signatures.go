package lsp

import (
	"fmt"
	"strings"
)

func SignatureFromEntry(entry *SymbolEntry) *SignatureInformation {
	fn := signatureFunctionType(entry)
	if fn == nil {
		return nil
	}

	name := string(entry.Name)
	if name == "" {
		name = "function"
	}

	params := make([]ParameterInformation, 0, len(fn.Params)+b2i(fn.Variadic))
	labels := make([]string, 0, len(fn.Params)+b2i(fn.Variadic))
	for i := range fn.Params {
		label := fmt.Sprintf("param%d", i+1)
		params = append(params, ParameterInformation{Label: label})
		labels = append(labels, label)
	}
	if fn.Variadic {
		params = append(params, ParameterInformation{Label: "..."})
		labels = append(labels, "...")
	}

	return &SignatureInformation{
		Label:      fmt.Sprintf("%s(%s)", name, joinComma(labels)),
		Parameters: params,
	}
}

func SignatureHelpForEntry(entry *SymbolEntry) *SignatureHelp {
	sig := SignatureFromEntry(entry)
	if sig == nil {
		return nil
	}

	return &SignatureHelp{
		Signatures:      []SignatureInformation{*sig},
		ActiveSignature: 0,
		ActiveParameter: 0,
	}
}

func SignatureHelpForMethodCall(name SymbolName, objType *Type, index *GlobalIndex, resource ResourceURI) *SignatureHelp {
	if name == "" {
		return nil
	}

	if fn := signatureFunctionFromObject(name, objType); fn != nil {
		return signatureHelpFromFunction(name, fn)
	}

	if objType == nil && index != nil {
		if entry := index.LookupByScope(resource, GlobalIndexScopeShared, name); entry != nil {
			return SignatureHelpForEntry(entry)
		}
	}

	return nil
}

func signatureHelpFromFunction(name SymbolName, fn *FunctionType) *SignatureHelp {
	if fn == nil {
		return nil
	}
	entry := &SymbolEntry{Name: name, Type: Type{Primitive: TypeFunction, Structural: &StructuralType{Function: fn}}}
	return SignatureHelpForEntry(entry)
}

func signatureFunctionType(entry *SymbolEntry) *FunctionType {
	if entry == nil || entry.Type.Structural == nil {
		return nil
	}
	return entry.Type.Structural.Function
}

func signatureFunctionFromObject(name SymbolName, objType *Type) *FunctionType {
	if objType == nil || objType.Structural == nil {
		return nil
	}
	field, ok := objType.Structural.Fields[string(name)]
	if !ok || field.Structural == nil {
		return nil
	}
	return field.Structural.Function
}

func joinComma(parts []string) string {
	return strings.Join(parts, ", ")
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}
