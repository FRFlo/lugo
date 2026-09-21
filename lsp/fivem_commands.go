package lsp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// FiveM command analysis is deliberately limited to literal arguments. Command
// names assembled at runtime cannot be checked reliably and are ignored.
type fiveMCommandUse struct {
	name       string
	doc        *Document
	node       ast.NodeID
	restricted bool
}

type fiveMKeyMapping struct {
	command string
	device  string
	key     string
	doc     *Document
	node    ast.NodeID
}

func fiveMCall(doc *Document, id ast.NodeID) (string, ast.Node) {
	n := doc.Tree.Nodes[id]
	if n.Kind != ast.KindCallExpr || n.Left == ast.InvalidNode || int(n.Left) >= len(doc.Tree.Nodes) {
		return "", n
	}
	left := doc.Tree.Nodes[n.Left]
	if left.Kind != ast.KindIdent {
		return "", n
	}
	return string(doc.Source()[left.Start:left.End]), n
}

func fiveMLiteralString(doc *Document, id ast.NodeID) (string, bool) {
	if id == ast.InvalidNode || int(id) >= len(doc.Tree.Nodes) {
		return "", false
	}
	n := doc.Tree.Nodes[id]
	if n.Kind != ast.KindString && n.Kind != ast.KindHashedString {
		return "", false
	}
	return unquoteLuaString(string(doc.Source()[n.Start:n.End])), true
}

func fiveMCommandArg(doc *Document, call ast.Node, index uint32) (string, ast.NodeID, bool) {
	id := callArgument(doc, call, index)
	value, ok := fiveMLiteralString(doc, id)
	return value, id, ok && value != ""
}

func sortedFiveMDocuments(documents map[string]*Document) []*Document {
	docs := make([]*Document, 0, len(documents))
	for _, doc := range documents {
		if doc != nil {
			docs = append(docs, doc)
		}
	}
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].URI != docs[j].URI {
			return docs[i].URI < docs[j].URI
		}
		return docs[i].Path < docs[j].Path
	})
	return docs
}

func sortFiveMDiagnostics(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line < b.Range.Start.Line
		}
		if a.Range.Start.Character != b.Range.Start.Character {
			return a.Range.Start.Character < b.Range.Start.Character
		}
		if a.Range.End.Line != b.Range.End.Line {
			return a.Range.End.Line < b.Range.End.Line
		}
		if a.Range.End.Character != b.Range.End.Character {
			return a.Range.End.Character < b.Range.End.Character
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
}

func (s *Server) collectFiveMCommands() ([]fiveMCommandUse, []fiveMKeyMapping, map[string]bool) {
	var commands []fiveMCommandUse
	var mappings []fiveMKeyMapping
	aces := make(map[string]bool)
	for _, doc := range sortedFiveMDocuments(s.Documents) {
		if doc == nil || doc.Tree == nil {
			continue
		}
		for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
			name, call := fiveMCall(doc, id)
			switch name {
			case "RegisterCommand":
				command, node, ok := fiveMCommandArg(doc, call, 0)
				if !ok {
					continue
				}
				restricted := false
				arg := callArgument(doc, call, 2)
				if arg != ast.InvalidNode && int(arg) < len(doc.Tree.Nodes) {
					restricted = doc.Tree.Nodes[arg].Kind == ast.KindTrue
				}
				commands = append(commands, fiveMCommandUse{name: command, doc: doc, node: node, restricted: restricted})
			case "RegisterKeyMapping":
				command, node, ok := fiveMCommandArg(doc, call, 0)
				if !ok {
					continue
				}
				device, _, _ := fiveMCommandArg(doc, call, 2)
				key, _, _ := fiveMCommandArg(doc, call, 3)
				if key != "" {
					mappings = append(mappings, fiveMKeyMapping{command: command, device: device, key: strings.ToLower(key), doc: doc, node: node})
				}
			case "ExecuteCommand":
				command, _, ok := fiveMCommandArg(doc, call, 0)
				if ok && strings.HasPrefix(strings.ToLower(command), "add_ace ") {
					fields := strings.Fields(command)
					if len(fields) >= 3 {
						aceName := fields[1]
						aceName = strings.TrimPrefix(aceName, "command.")
						aces[aceName] = true
					}
				}
			}
		}
	}
	return commands, mappings, aces
}

func (s *Server) buildFiveMCommandDiagnostics(doc *Document) []Diagnostic {
	if s == nil || doc == nil || doc.Tree == nil {
		return nil
	}
	var commands []fiveMCommandUse
	var mappings []fiveMKeyMapping
	var aces map[string]bool
	if facts := s.workspaceDiagnosticFacts(); facts != nil {
		commands, mappings, aces = facts.commands, facts.mappings, facts.aces
	} else {
		commands, mappings, aces = s.collectFiveMCommands()
	}
	var out []Diagnostic
	// Mark ACE declarations after collecting all files, then report restricted
	// commands without an explicit ACE declaration. This is intentionally a
	// warning: ACE configuration is commonly kept outside the resource.
	declared := make(map[string]bool, len(commands))
	for _, command := range commands {
		declared[command.name] = true
		if command.doc == doc && command.restricted && !aces[command.name] {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, command.node), Severity: SeverityWarning, Code: "fivem-command-missing-ace", Message: fmt.Sprintf("Command '%s' is restricted but has no documented ACE permission.", command.name)})
		}
	}
	for _, mapping := range mappings {
		if mapping.doc != doc || mapping.node == ast.InvalidNode || int(mapping.node) >= len(doc.Tree.Nodes) {
			continue
		}
		if !declared[mapping.command] {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, mapping.node), Severity: SeverityWarning, Code: "fivem-command-missing-declaration", Message: fmt.Sprintf("Key mapping references undeclared command '%s'.", mapping.command)})
		}
	}
	seen := make(map[string]fiveMKeyMapping)
	for _, mapping := range mappings {
		key := mapping.device + "\x00" + mapping.key
		if previous, ok := seen[key]; ok && previous.command != mapping.command && mapping.doc == doc {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, mapping.node), Severity: SeverityWarning, Code: "fivem-key-mapping-conflict", Message: fmt.Sprintf("Key mapping '%s' conflicts with command '%s'.", mapping.command, previous.command)})
		} else {
			seen[key] = mapping
		}
	}
	// ExecuteCommand is checked separately so a literal invocation is useful
	// even when there is no key mapping.
	for id := ast.NodeID(1); int(id) < len(doc.Tree.Nodes); id++ {
		name, call := fiveMCall(doc, id)
		if name != "ExecuteCommand" {
			continue
		}
		command, node, ok := fiveMCommandArg(doc, call, 0)
		if !ok || strings.HasPrefix(strings.ToLower(command), "add_ace ") || strings.HasPrefix(strings.ToLower(command), "add_principal ") {
			continue
		}
		invoked := strings.Fields(command)
		if len(invoked) == 0 {
			continue
		}
		if !declared[invoked[0]] {
			out = append(out, Diagnostic{Range: getNodeRange(doc.Tree, node), Severity: SeverityWarning, Code: "fivem-command-missing-declaration", Message: fmt.Sprintf("ExecuteCommand invokes undeclared command '%s'.", invoked[0])})
		}
	}
	sortFiveMDiagnostics(out)
	return out
}
