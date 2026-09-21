package lsp

import (
	"fmt"
	"strings"

	"github.com/coalaura/lugo/ast"
)

// Convar checking intentionally only considers literal names.  Convars are
// global to the server, so declarations are collected across the workspace.
type fiveMConvar struct {
	name     string
	typeName string
	def      string
	decl     bool
	doc      *Document
	node     ast.NodeID
}

type fiveMConvarUse struct {
	name     string
	typeName string
	def      string
	doc      *Document
	node     ast.NodeID
}

func convarLiteral(doc *Document, id ast.NodeID) (string, string, bool) {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return "", "", false
	}
	n := doc.Tree.Nodes[id]
	switch n.Kind {
	case ast.KindString, ast.KindHashedString:
		return unquoteLuaString(string(doc.Source()[n.Start:n.End])), "string", true
	case ast.KindNumber:
		return string(doc.Source()[n.Start:n.End]), "number", true
	case ast.KindTrue, ast.KindFalse:
		return string(doc.Source()[n.Start:n.End]), "boolean", true
	}
	return "", "", false
}

func convarCall(doc *Document, id ast.NodeID) (string, ast.Node, bool) {
	n := doc.Tree.Nodes[id]
	if n.Kind != ast.KindCallExpr || n.Left == ast.InvalidNode || int(n.Left) >= len(doc.Tree.Nodes) {
		return "", n, false
	}
	left := doc.Tree.Nodes[n.Left]
	if left.Kind != ast.KindIdent {
		return "", n, false
	}
	return string(doc.Source()[left.Start:left.End]), n, true
}

func (s *Server) buildFiveMConvarDiagnostics(doc *Document) []Diagnostic {
	if s == nil || doc == nil || doc.Tree == nil {
		return nil
	}
	known := make(map[string][]fiveMConvar)
	var uses []fiveMConvarUse
	var out []Diagnostic
	for _, other := range s.Documents {
		if other == nil || other.Tree == nil {
			continue
		}
		profile := s.getDocumentFiveMProfile(other)
		for id := ast.NodeID(1); int(id) < len(other.Tree.Nodes); id++ {
			name, call, ok := convarCall(other, id)
			if !ok {
				continue
			}
			switch name {
			case "SetConvar", "SetConvarReplicated", "SetConvarServerInfo":
				nameValue, nameType, ok := convarLiteral(other, callArgument(other, call, 0))
				if !ok || nameValue == "" || nameType != "string" {
					continue
				}
				value, typ, _ := convarLiteral(other, callArgument(other, call, 1))
				entry := fiveMConvar{name: nameValue, typeName: typ, def: value, doc: other, node: callArgument(other, call, 0)}
				known[strings.ToLower(nameValue)] = append(known[strings.ToLower(nameValue)], entry)
				if other == doc && profile.Kind != FiveMProfilePlainLua && profile.Kind != FiveMProfileServer {
					out = append(out, Diagnostic{Range: getNodeRange(other.Tree, entry.node), Severity: SeverityWarning, Code: "fivem-convar-scope", Message: fmt.Sprintf("%s may only be used from a server script.", name)})
				}
			case "GetConvar", "GetConvarBool", "GetConvarInt", "GetConvarFloat":
				nameValue, _, ok := convarLiteral(other, callArgument(other, call, 0))
				if !ok || nameValue == "" {
					continue
				}
				def, typ, _ := convarLiteral(other, callArgument(other, call, 1))
				if name == "GetConvarBool" {
					typ = "boolean"
				} else if name == "GetConvarInt" || name == "GetConvarFloat" {
					typ = "number"
				} else {
					typ = "string"
				}
				uses = append(uses, fiveMConvarUse{name: nameValue, typeName: typ, def: def, doc: other, node: callArgument(other, call, 0)})
			case "ExecuteCommand":
				command, _, ok := convarLiteral(other, callArgument(other, call, 0))
				if !ok {
					continue
				}
				fields := strings.Fields(command)
				if len(fields) < 2 {
					continue
				}
				kind := strings.ToLower(fields[0])
				if kind != "set" && kind != "setr" && kind != "sets" {
					continue
				}
				value := ""
				if len(fields) > 2 {
					value = strings.Join(fields[2:], " ")
				}
				known[strings.ToLower(fields[1])] = append(known[strings.ToLower(fields[1])], fiveMConvar{name: fields[1], typeName: "string", def: value, decl: true, doc: other, node: callArgument(other, call, 0)})
			}
		}
		// Manifest convar declarations are literal directive values.
		if other.IsFiveMManifest {
			if res := s.parseFiveMManifest(other); res != nil && res.Manifest != nil {
				for _, entry := range res.Manifest.Entries {
					if entry.EmittedName != "convar" && entry.EmittedName != "convar_category" {
						continue
					}
					if entry.Value == "" {
						continue
					}
					known[strings.ToLower(entry.Value)] = append(known[strings.ToLower(entry.Value)], fiveMConvar{name: entry.Value, typeName: "string", decl: true, doc: other})
				}
			}
		}
	}
	for _, use := range uses {
		if use.doc != doc {
			continue
		}
		entries := known[strings.ToLower(use.name)]
		if len(entries) == 0 {
			out = append(out, Diagnostic{Range: getNodeRange(use.doc.Tree, use.node), Severity: SeverityWarning, Code: "fivem-convar-unknown", Message: fmt.Sprintf("Convar '%s' is read without a known declaration.", use.name)})
			continue
		}
		for _, entry := range entries {
			if entry.typeName != "" && use.typeName != "string" && entry.typeName != use.typeName {
				out = append(out, Diagnostic{Range: getNodeRange(use.doc.Tree, use.node), Severity: SeverityWarning, Code: "fivem-convar-type-conflict", Message: fmt.Sprintf("Convar '%s' is read as %s but declared as %s.", use.name, use.typeName, entry.typeName)})
				break
			}
			if use.def != "" && entry.def != "" && use.def != entry.def {
				out = append(out, Diagnostic{Range: getNodeRange(use.doc.Tree, use.node), Severity: SeverityWarning, Code: "fivem-convar-default-conflict", Message: fmt.Sprintf("Convar '%s' has conflicting defaults (%q and %q).", use.name, use.def, entry.def)})
				break
			}
		}
	}
	// Conflicts between declarations/writes are reported at the later write.
	for name, entries := range known {
		if len(entries) < 2 {
			continue
		}
		first := entries[0]
		for _, entry := range entries[1:] {
			if (first.typeName != "" && entry.typeName != "" && first.typeName != entry.typeName) || (first.def != "" && entry.def != "" && first.def != entry.def) {
				if entry.doc == doc {
					out = append(out, Diagnostic{Range: getNodeRange(entry.doc.Tree, entry.node), Severity: SeverityWarning, Code: "fivem-convar-conflict", Message: fmt.Sprintf("Convar '%s' has conflicting declarations.", name)})
				}
				break
			}
		}
	}
	return out
}
