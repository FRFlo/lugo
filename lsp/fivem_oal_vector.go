package lsp

import (
	"fmt"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// buildFiveMOALVectorDiagnostics reports vector arguments that OAL passes as
// one value where a native signature requires the vector's numeric components.
func (s *Server) buildFiveMOALVectorDiagnostics(doc *Document) []Diagnostic {
	if s == nil || doc == nil || doc.Tree == nil {
		return nil
	}

	profile := s.getDocumentFiveMProfile(doc)
	if profile.Kind != FiveMProfileClient {
		return nil
	}
	resource := s.resolveFiveMResourceByRoot(profile.ResourceRoot)
	if resource == nil || !resource.NativeSelection(profile).UseExperimentalOAL {
		return nil
	}

	var diagnostics []Diagnostic
	for callID := ast.NodeID(1); int(callID) < len(doc.Tree.Nodes); callID++ {
		call := doc.Tree.Nodes[callID]
		if call.Kind != ast.KindCallExpr {
			continue
		}
		functionID := call.Left
		if functionID == ast.InvalidNode || int(functionID) >= len(doc.Tree.Nodes) {
			continue
		}
		if function := doc.Tree.Nodes[functionID]; function.Kind == ast.KindMemberExpr {
			functionID = function.Right
		}

		ctx := s.resolveSymbolNode(doc.URI, doc, functionID)
		if ctx == nil {
			continue
		}

		defs := ctx.GlobalDefs
		if len(defs) == 0 && ctx.TargetDoc != nil && ctx.TargetDefID != ast.InvalidNode {
			defs = []GlobalSymbol{{URI: ctx.TargetDoc.URI, NodeID: ctx.TargetDefID}}
		}
		var nativeParams []LuaDocParam
		for _, def := range defs {
			target := s.Documents[def.URI]
			if _, ok := fiveMNativeMetadataFor(target, def.NodeID); !ok {
				continue
			}
			luaDoc := target.GetLuaDoc(def.NodeID)
			if luaDoc != nil && len(luaDoc.Params) > 0 {
				nativeParams = luaDoc.Params
				break
			}
		}
		if len(nativeParams) == 0 {
			continue
		}

		for argIndex := uint32(0); argIndex < call.Count; argIndex++ {
			argID := doc.Tree.ExtraList[call.Extra+argIndex]
			dimension := fiveMVectorDimension(doc.InferType(argID))
			if dimension == 0 || argIndex+uint32(dimension) > uint32(len(nativeParams)) {
				continue
			}

			componentsExpected := true
			for component := 0; component < dimension; component++ {
				if !fiveMNativeNumericParam(nativeParams[int(argIndex)+component].Type) {
					componentsExpected = false
					break
				}
			}
			if !componentsExpected {
				continue
			}

			diagnostics = append(diagnostics, Diagnostic{
				Range:    getNodeRange(doc.Tree, argID),
				Severity: SeverityWarning,
				Code:     "fivem-oal-vector-argument",
				Message:  fmt.Sprintf("This native expects separate numeric vector components, but use_experimental_fxv2_oal passes the vector as one argument. Pass its %s components explicitly.", fiveMVectorComponentNames(dimension)),
			})
		}
	}

	return diagnostics
}

func fiveMVectorComponentNames(dimension int) string {
	components := []string{".x", ".y", ".z", ".w"}
	if dimension > len(components) {
		dimension = len(components)
	}
	return strings.Join(components[:dimension], ", ")
}

func fiveMVectorDimension(t TypeSet) int {
	switch strings.ToLower(strings.TrimSpace(t.CustomName)) {
	case "vector2":
		return 2
	case "vector3":
		return 3
	case "vector4", "quat":
		return 4
	default:
		return 0
	}
}

func fiveMNativeNumericParam(paramType string) bool {
	if strings.TrimSpace(paramType) == "" {
		return false
	}
	for _, part := range strings.Split(strings.ToLower(paramType), "|") {
		part = strings.TrimSpace(strings.TrimSuffix(part, "?"))
		switch part {
		case "number", "integer", "float", "double":
		default:
			return false
		}
	}
	return true
}
