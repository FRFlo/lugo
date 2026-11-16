package lsp

import "strings"

func substituteLuaDocGenericType(typeName string, bindings map[string]string) string {
	return substituteGenericLuaDocType(typeName, bindings)
}

func luaDocGenericBindingsForHierarchy(hierarchy []LuaDocClassHierarchyEntry, rootArgs []string) []map[string]string {
	if len(hierarchy) == 0 {
		return nil
	}
	hasGenericMetadata := false
	for _, class := range hierarchy {
		if class.Entry == nil || class.Entry.LuaDoc == nil || class.Entry.LuaDoc.GenericClass == nil {
			continue
		}
		genericClass := class.Entry.LuaDoc.GenericClass
		if len(genericClass.Params) > 0 || luaDocClassHasGenericParent(genericClass.Parents) {
			hasGenericMetadata = true
			break
		}
	}
	if !hasGenericMetadata {
		return nil
	}
	bindings := make([]map[string]string, len(hierarchy))
	classParams := func(index int) []LuaDocGeneric {
		if index < 0 || index >= len(hierarchy) || hierarchy[index].Entry == nil || hierarchy[index].Entry.LuaDoc == nil || hierarchy[index].Entry.LuaDoc.GenericClass == nil {
			return nil
		}
		return hierarchy[index].Entry.LuaDoc.GenericClass.Params
	}
	bind := func(index int, args []string) map[string]string {
		params := classParams(index)
		result := make(map[string]string, len(params))
		for i, parameter := range params {
			if i < len(args) && args[i] != "" {
				result[parameter.Name] = args[i]
			}
		}
		return result
	}
	bindings[0] = bind(0, rootArgs)
	for childIndex := range hierarchy {
		childBindings := bindings[childIndex]
		if childBindings == nil {
			continue
		}
		entry := hierarchy[childIndex].Entry
		if entry == nil {
			continue
		}
		parentTypes := symbolParents(entry)
		if entry.LuaDoc != nil && entry.LuaDoc.GenericClass != nil {
			parentTypes = entry.LuaDoc.GenericClass.Parents
		}
		parentIndices := hierarchy[childIndex].ParentIndices
		if hierarchy[childIndex].FirstParent >= 0 {
			parentIndices = append([]int{hierarchy[childIndex].FirstParent}, parentIndices...)
		}
		for _, parentIndex := range parentIndices {
			if parentIndex < 0 || parentIndex >= len(hierarchy) || bindings[parentIndex] != nil {
				continue
			}
			parentName := hierarchy[parentIndex].Name
			for _, parentType := range parentTypes {
				name, args, isGeneric := splitLuaDocGenericType(parentType)
				if !isGeneric {
					name = strings.TrimSpace(parentType)
				}
				if name != parentName {
					continue
				}
				for i := range args {
					args[i] = substituteLuaDocGenericType(args[i], childBindings)
				}
				bindings[parentIndex] = bind(parentIndex, args)
				break
			}
		}
	}
	return bindings
}
