package lsp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FRFlo/lugo/ast"
)

// fiveMServerConfig is a conservative snapshot of literal startup commands.
// A nonliteral command makes Complete false: runtime state may differ.
type fiveMServerConfig struct {
	Commands []fiveMServerCommand
	Files    []string
	Problems []fiveMServerConfigProblem
	Complete bool
}

type fiveMServerCommand struct {
	Name, Argument, Path string
	Line                 int
}

type fiveMServerConfigProblem struct {
	Path, Message string
	Line          int
}

func readFiveMServerConfig(path string) fiveMServerConfig {
	result := fiveMServerConfig{Complete: true}
	path, err := filepath.Abs(path)
	if err != nil {
		result.Complete = false
		result.Problems = append(result.Problems, fiveMServerConfigProblem{Path: path, Message: err.Error()})
		return result
	}
	root := filepath.Dir(path)
	active := make(map[string]bool)
	var read func(string, string, int)
	read = func(file, parent string, parentLine int) {
		file = filepath.Clean(file)
		if active[file] {
			result.Complete = false
			result.Problems = append(result.Problems, fiveMServerConfigProblem{Path: parent, Line: parentLine, Message: "configuration include cycle: " + file})
			return
		}
		if len(active) >= 64 {
			result.Complete = false
			result.Problems = append(result.Problems, fiveMServerConfigProblem{Path: parent, Line: parentLine, Message: "configuration include depth exceeded"})
			return
		}
		f, err := os.Open(file)
		if err != nil {
			result.Complete = false
			result.Problems = append(result.Problems, fiveMServerConfigProblem{Path: parent, Line: parentLine, Message: fmt.Sprintf("cannot read %s: %v", file, err)})
			return
		}
		defer f.Close()
		result.Files = append(result.Files, file)
		active[file] = true
		defer delete(active, file)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		line := 0
		for scanner.Scan() {
			line++
			commands, ok := splitFiveMConfigLine(scanner.Text())
			if !ok {
				result.Complete = false
				continue
			}
			for _, words := range commands {
				if len(words) == 0 {
					continue
				}
				name := strings.ToLower(words[0])
				if name != "exec" && name != "ensure" && name != "start" && name != "stop" && name != "restart" {
					continue
				}
				if len(words) != 2 || strings.ContainsAny(words[1], "$%*") {
					result.Complete = false
					continue
				}
				command := fiveMServerCommand{Name: name, Argument: words[1], Path: file, Line: line}
				if name == "exec" {
					if strings.HasPrefix(words[1], "@") {
						result.Complete = false
						continue
					}
					child := words[1]
					if !filepath.IsAbs(child) {
						child = filepath.Join(root, child)
					}
					read(child, file, line)
				} else {
					result.Commands = append(result.Commands, command)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			result.Complete = false
			result.Problems = append(result.Problems, fiveMServerConfigProblem{Path: file, Line: line, Message: err.Error()})
		}
	}
	read(path, path, 0)
	return result
}

// splitFiveMConfigLine handles quoted words, semicolons and comments without
// interpreting command substitutions or other dynamic server console input.
func splitFiveMConfigLine(line string) ([][]string, bool) {
	var commands [][]string
	var words []string
	var word strings.Builder
	var quote rune
	started := false
	flushWord := func() {
		if started {
			words = append(words, word.String())
			word.Reset()
			started = false
		}
	}
	flushCommand := func() {
		flushWord()
		if len(words) != 0 {
			commands = append(commands, words)
			words = nil
		}
	}
	for i, c := range line {
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			started = true
		case '#':
			flushCommand()
			return commands, true
		case '/':
			if i+1 < len(line) && line[i+1] == '/' && !started {
				flushCommand()
				return commands, true
			}
			word.WriteRune(c)
			started = true
		case ';':
			flushCommand()
		case ' ', '\t', '\r':
			flushWord()
		default:
			word.WriteRune(c)
			started = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	flushCommand()
	return commands, true
}

// fiveMServerConfigPaths resolves one configuration per workspace. Explicit
// paths take precedence; otherwise the nearest ancestor server.cfg is used.
func (s *Server) fiveMServerConfigPaths() []string {
	seen := make(map[string]bool)
	var paths []string
	for _, uri := range s.WorkspaceFolders {
		root := s.uriToPath(uri)
		if root == "" {
			continue
		}
		var path string
		if s.FiveMServerConfig != "" {
			path = s.FiveMServerConfig
			if !filepath.IsAbs(path) {
				path = filepath.Join(root, path)
			}
		} else {
			for dir := filepath.Clean(root); ; dir = filepath.Dir(dir) {
				candidate := filepath.Join(dir, "server.cfg")
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					path = candidate
					break
				}
				if filepath.Dir(dir) == dir {
					break
				}
			}
		}
		if path != "" {
			path = filepath.Clean(path)
			if !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	return paths
}

// publishFiveMServerConfigDiagnostics keeps config files outside the Lua
// parser and clears previously published diagnostics when their context changes.
func (s *Server) publishFiveMServerConfigDiagnostics() {
	current := make(map[string][]Diagnostic)
	if s.DiagFiveMServerConfig {
		for _, path := range s.fiveMServerConfigPaths() {
			config := readFiveMServerConfig(path)
			for _, problem := range config.Problems {
				uri := s.pathToURI(problem.Path)
				line := problem.Line - 1
				if line < 0 {
					line = 0
				}
				pos := Position{Line: uint32(line)}
				current[uri] = append(current[uri], Diagnostic{Range: Range{Start: pos, End: pos}, Severity: SeverityWarning, Code: "fivem-server-config", Message: problem.Message})
			}
		}
	}
	if s.IsCI {
		for uri, diags := range current {
			s.printCIDiagnostics(uri, diags)
		}
		return
	}
	for uri := range s.fiveMConfigPublished {
		if _, ok := current[uri]; !ok {
			WriteMessage(s.Writer, OutgoingNotification{RPC: "2.0", Method: "textDocument/publishDiagnostics", Params: PublishDiagnosticsParams{URI: uri, Diagnostics: []Diagnostic{}}})
			delete(s.fiveMConfigPublished, uri)
		}
	}
	for uri, diags := range current {
		if s.fiveMConfigPublished == nil {
			s.fiveMConfigPublished = make(map[string]struct{})
		}
		s.fiveMConfigPublished[uri] = struct{}{}
		WriteMessage(s.Writer, OutgoingNotification{RPC: "2.0", Method: "textDocument/publishDiagnostics", Params: PublishDiagnosticsParams{URI: uri, Diagnostics: diags}})
	}
}

type fiveMConfigEventHandler struct {
	profile FiveMExecutionProfile
}

func (s *Server) collectFiveMConfigEventHandlers() map[string][]fiveMConfigEventHandler {
	handlers := make(map[string][]fiveMConfigEventHandler)
	for _, target := range s.Documents {
		if target == nil || target.Tree == nil {
			continue
		}
		profile := s.getDocumentFiveMProfile(target)
		for _, event := range target.FiveMEvents {
			if (event.Kind == FiveMEventAddHandler || event.Kind == FiveMEventRegisterNet) && event.HandlerID != ast.InvalidNode {
				handlers[event.Name] = append(handlers[event.Name], fiveMConfigEventHandler{profile: profile})
			}
		}
	}
	return handlers
}

// fiveMEventLoadOrderDiagnostics reports only literal, top-level local
// triggers whose known handlers are all in resources started later. An
// unlisted resource, dependency or dynamic startup keeps the result unknown.
func (s *Server) fiveMEventLoadOrderDiagnostics(doc *Document) []Diagnostic {
	if doc == nil || doc.Tree == nil || len(doc.FiveMEvents) == 0 {
		return nil
	}
	profile := s.getDocumentFiveMProfile(doc)
	if !profile.HasResource() {
		return nil
	}
	hasLocalTrigger := false
	for _, event := range doc.FiveMEvents {
		if event.Kind == FiveMEventTriggerLocal {
			hasLocalTrigger = true
			break
		}
	}
	if !hasLocalTrigger {
		return nil
	}
	root := s.uriToPath(profile.ResourceRoot)
	var chosen string
	var chosenWorkspace string
	if s.FiveMServerConfig != "" {
		for _, uri := range s.WorkspaceFolders {
			workspace := s.uriToPath(uri)
			rel, err := filepath.Rel(workspace, root)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(workspace) > len(chosenWorkspace) {
				chosenWorkspace = workspace
			}
		}
		if chosenWorkspace != "" {
			chosen = s.FiveMServerConfig
			if !filepath.IsAbs(chosen) {
				chosen = filepath.Join(chosenWorkspace, chosen)
			}
			chosen = filepath.Clean(chosen)
		}
	}
	facts := s.workspaceDiagnosticFacts()
	var paths []string
	if facts != nil {
		for path := range facts.serverConfigs {
			paths = append(paths, path)
		}
	} else {
		paths = s.fiveMServerConfigPaths()
	}
	if chosen == "" && s.FiveMServerConfig == "" {
		for _, path := range paths {
			rel, err := filepath.Rel(filepath.Dir(path), root)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(filepath.Dir(path)) > len(filepath.Dir(chosen)) {
				chosen = path
			}
		}
	}
	if chosen == "" {
		return nil
	}
	var config fiveMServerConfig
	if facts != nil {
		config = facts.serverConfigs[chosen]
	} else {
		config = readFiveMServerConfig(chosen)
	}
	if !config.Complete {
		return nil
	}
	positions := make(map[string]int)
	for i, command := range config.Commands {
		if command.Name == "stop" || command.Name == "restart" {
			return nil
		}
		if strings.HasPrefix(command.Argument, "[") && strings.HasSuffix(command.Argument, "]") {
			if s.FiveMResourceGraph == nil {
				return nil
			}
			for _, node := range s.FiveMResourceGraph.ByRoot {
				if node == nil || node.Name == "" {
					continue
				}
				for _, segment := range strings.Split(filepath.ToSlash(s.uriToPath(node.RootURI)), "/") {
					if segment == command.Argument {
						if _, exists := positions[node.Name]; !exists {
							positions[node.Name] = i
						}
						break
					}
				}
			}
		} else if _, exists := positions[command.Argument]; !exists {
			positions[command.Argument] = i
		}
	}
	consumerPos, ok := positions[profile.ResourceName]
	if !ok {
		return nil
	}
	// A dependency may start the provider implicitly, including transitively;
	// without modeling that startup graph the configured order is insufficient.
	if resource := s.resolveFiveMResourceByRoot(profile.ResourceRoot); resource != nil && len(resource.Dependencies) != 0 {
		return nil
	}
	var diagnostics []Diagnostic
	var handlers map[string][]fiveMConfigEventHandler
	if facts != nil {
		handlers = facts.serverHandlers
	} else {
		handlers = s.collectFiveMConfigEventHandlers()
	}
	for _, trigger := range doc.FiveMEvents {
		if trigger.Kind != FiveMEventTriggerLocal || trigger.NodeID == ast.InvalidNode || int(trigger.NodeID) >= len(doc.Tree.Nodes) {
			continue
		}
		callID := doc.Tree.Nodes[trigger.NodeID].Parent
		if callID == ast.InvalidNode || int(callID) >= len(doc.Tree.Nodes) {
			continue
		}
		parent := doc.Tree.Nodes[callID].Parent
		if parent == ast.InvalidNode || int(parent) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[parent].Kind != ast.KindBlock {
			continue
		}
		blockParent := doc.Tree.Nodes[parent].Parent
		if blockParent == ast.InvalidNode || int(blockParent) >= len(doc.Tree.Nodes) || doc.Tree.Nodes[blockParent].Kind != ast.KindFile {
			continue
		}
		found, uncertain := false, false
		for _, handler := range handlers[trigger.Name] {
			targetProfile := handler.profile
			if !fiveMCallbackEnvironmentsOverlap(profile.Env(), targetProfile.Env()) {
				continue
			}
			found = true
			pos, exists := positions[targetProfile.ResourceName]
			if !exists || pos <= consumerPos {
				uncertain = true
			}
		}
		if found && !uncertain {
			diagnostics = append(diagnostics, Diagnostic{Range: getNodeRange(doc.Tree, trigger.NodeID), Severity: SeverityWarning, Code: "fivem-event-load-order", Message: "Event may run before its handlers are registered: their resources start later in the server configuration."})
		}
	}
	return diagnostics
}
