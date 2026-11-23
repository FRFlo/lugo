package lsp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMConvarQuickFix returns explicit, evidence-based alternatives for
// convar diagnostics. It never supplies a default value or moves a write.
func (s *Server) fiveMConvarQuickFix(doc *Document, uri string, diag Diagnostic) []CodeAction {
	if s == nil || doc == nil || !strings.HasPrefix(diag.Code, "fivem-convar-") {
		return nil
	}
	facts := s.collectFiveMConvars()
	switch diag.Code {
	case "fivem-convar-unknown":
		name := convarNameAtDiagnostic(doc, diag)
		if name == "" {
			return nil
		}
		var names []string
		for _, entries := range facts.known {
			for _, entry := range entries {
				if levenshteinFast(strings.ToLower(name), strings.ToLower(entry.name), 2) <= 2 && !containsConvarString(names, entry.name) {
					names = append(names, entry.name)
				}
			}
		}
		sort.Strings(names)
		var actions []CodeAction
		for _, candidate := range names {
			actions = append(actions, convarRenameAction(uri, diag, candidate))
		}
		if action, ok := s.addConvarManifestAction(doc, diag, name); ok {
			actions = append(actions, action)
		}
		return actions
	case "fivem-convar-type-conflict":
		return s.convarReadAlternatives(doc, uri, diag, facts, true)
	case "fivem-convar-default-conflict":
		return s.convarReadAlternatives(doc, uri, diag, facts, false)
	case "fivem-convar-conflict":
		return s.convarDeclarationAlternatives(doc, uri, diag, facts)
	default:
		return nil
	}
}

func convarNameAtDiagnostic(doc *Document, diag Diagnostic) string {
	if doc == nil || doc.Tree == nil {
		return ""
	}
	id := doc.Tree.NodeAt(doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character))
	for id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes) {
		if name, typ, ok := convarLiteral(doc, id); ok && typ == "string" && name != "" {
			return name
		}
		id = doc.Tree.Nodes[id].Parent
	}
	return ""
}

func (s *Server) addConvarManifestAction(doc *Document, diag Diagnostic, name string) (CodeAction, bool) {
	res := doc.FiveMManifestResource
	if res == nil {
		profile := s.getDocumentFiveMProfile(doc)
		res = s.resolveFiveMResourceByRoot(profile.ResourceRoot)
	}
	if res == nil || res.ManifestURI == "" || res.ManifestSource != FiveMManifestSourceFXManifest {
		return CodeAction{}, false
	}
	manifest := s.Documents[res.ManifestURI]
	if manifest == nil || !manifest.IsFiveMManifest || manifest.Tree == nil {
		return CodeAction{}, false
	}
	source := manifest.Source()
	if source == nil {
		return CodeAction{}, false
	}
	line, col := manifest.Tree.Position(uint32(len(source)))
	position := Position{Line: line, Character: col}
	prefix := ""
	if len(source) > 0 && source[len(source)-1] != '\n' {
		prefix = "\n"
	}
	edit := TextEdit{Range: Range{Start: position, End: position}, NewText: prefix + fmt.Sprintf("convar %q\n", name)}
	return CodeAction{Title: fmt.Sprintf("Declare convar '%s' in fxmanifest.lua", name), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{res.ManifestURI: {edit}}}}, true
}

func (s *Server) convarReadAlternatives(doc *Document, uri string, diag Diagnostic, facts fiveMConvarFacts, types bool) []CodeAction {
	name := convarNameAtDiagnostic(doc, diag)
	if name == "" {
		return nil
	}
	id := doc.Tree.NodeAt(doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character))
	var call ast.NodeID
	for id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes) {
		if doc.Tree.Nodes[id].Kind == ast.KindCallExpr {
			call = id
			break
		}
		id = doc.Tree.Nodes[id].Parent
	}
	if call == ast.InvalidNode {
		return nil
	}
	args := make([]string, 0, 4)
	for _, entry := range facts.known[strings.ToLower(name)] {
		if entry.typeName == "" {
			continue
		}
		if types {
			if entry.def == "" {
				continue
			}
			apis := map[string][]string{"string": {"GetConvar"}, "boolean": {"GetConvarBool"}, "number": {"GetConvarFloat", "GetConvarInt"}}[entry.typeName]
			for _, api := range apis {
				if !containsConvarString(args, api) {
					args = append(args, api)
				}
			}
		} else if entry.def != "" && !containsConvarString(args, entry.def) {
			args = append(args, entry.def)
		}
	}
	sort.Strings(args)
	var out []CodeAction
	if types {
		n := doc.Tree.Nodes[call]
		callee := n.Left
		if callee == 0 || int(callee) >= len(doc.Tree.Nodes) {
			return nil
		}
		defaultArg := callArgument(doc, doc.Tree.Nodes[call], 1)
		if defaultArg == ast.InvalidNode {
			return nil
		}
		for _, api := range args {
			for _, entry := range facts.known[strings.ToLower(name)] {
				if entry.def == "" {
					continue
				}
				apiForType := map[string][]string{"string": {"GetConvar"}, "boolean": {"GetConvarBool"}, "number": {"GetConvarFloat", "GetConvarInt"}}[entry.typeName]
				if !containsConvarString(apiForType, api) {
					continue
				}
				defaultText := entry.def
				if entry.typeName == "string" {
					defaultText = fmt.Sprintf("%q", defaultText)
				}
				edits := []TextEdit{{Range: getNodeRange(doc.Tree, callee), NewText: api}, {Range: getNodeRange(doc.Tree, defaultArg), NewText: defaultText}}
				out = append(out, CodeAction{Title: fmt.Sprintf("Read convar with %s() and its declared default", api), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: edits}}})
				break
			}
		}
	} else {
		arg := callArgument(doc, doc.Tree.Nodes[call], 1)
		if arg == ast.InvalidNode {
			return nil
		}
		for _, value := range args {
			out = append(out, CodeAction{Title: fmt.Sprintf("Use existing default %q", value), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: getNodeRange(doc.Tree, arg), NewText: fmt.Sprintf("%q", value)}}}}})
		}
	}
	return out
}

func (s *Server) convarDeclarationAlternatives(doc *Document, uri string, diag Diagnostic, facts fiveMConvarFacts) []CodeAction {
	name := convarNameAtDiagnostic(doc, diag)
	if name == "" {
		return nil
	}
	id := doc.Tree.NodeAt(doc.Tree.Offset(diag.Range.Start.Line, diag.Range.Start.Character))
	for id != ast.InvalidNode && int(id) < len(doc.Tree.Nodes) && getNodeRange(doc.Tree, id) != diag.Range {
		id = doc.Tree.Nodes[id].Parent
	}
	if id == ast.InvalidNode {
		return nil
	}
	var call ast.NodeID = ast.InvalidNode
	for parent := id; parent != ast.InvalidNode && int(parent) < len(doc.Tree.Nodes); parent = doc.Tree.Nodes[parent].Parent {
		if doc.Tree.Nodes[parent].Kind == ast.KindCallExpr {
			call = parent
			break
		}
	}
	if call == ast.InvalidNode {
		return nil // Manifest declarations and indirect writes need different edits.
	}
	currentType := ""
	if _, currentType, _ = convarLiteral(doc, callArgument(doc, doc.Tree.Nodes[call], 1)); currentType == "" {
		return nil
	}
	var values []string
	for _, entry := range facts.known[strings.ToLower(name)] {
		if entry.doc == doc && entry.node == id || entry.typeName != currentType || entry.def == "" {
			continue
		}
		if !containsConvarString(values, entry.def) {
			values = append(values, entry.def)
		}
	}
	sort.Strings(values)
	arg := callArgument(doc, doc.Tree.Nodes[call], 1)
	if arg == ast.InvalidNode {
		return nil
	}
	var actions []CodeAction
	for _, value := range values {
		replacement := value
		if currentType == "string" {
			replacement = fmt.Sprintf("%q", value)
		}
		actions = append(actions, CodeAction{Title: fmt.Sprintf("Use existing convar value %q", value), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: getNodeRange(doc.Tree, arg), NewText: replacement}}}}})
	}
	return actions
}

func convarRenameAction(uri string, diag Diagnostic, replacement string) CodeAction {
	return CodeAction{Title: fmt.Sprintf("Use existing convar '%s'", replacement), Kind: "quickfix", Diagnostics: []Diagnostic{diag}, Edit: &WorkspaceEdit{Changes: map[string][]TextEdit{uri: {{Range: diag.Range, NewText: fmt.Sprintf("%q", replacement)}}}}}
}

func containsConvarString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}
