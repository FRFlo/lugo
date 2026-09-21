package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// NUI contract checks deliberately only consider literal names.  NUI code is
// commonly generated, so guessing at dynamic names would create noisy errors.
type nuiContractName struct {
	name   string
	offset int
}

type nuiContractFile struct {
	uri   string
	src   []byte
	names []nuiContractName
}

type nuiDiagnosticState struct {
	mu        sync.Mutex
	published map[string]struct{}
}

type nuiResourceFacts struct {
	profile   FiveMExecutionProfile
	files     []nuiContractFile
	callbacks map[string]bool
	messages  map[string]bool
}

// NUI assets are not Documents, so retain the URIs that received standalone
// diagnostics in order to explicitly clear them after they become clean or
// are removed from disk.
var nuiDiagnosticStates sync.Map // map[*Server]*nuiDiagnosticState

func (s *Server) nuiDiagnosticState() *nuiDiagnosticState {
	if state, ok := nuiDiagnosticStates.Load(s); ok {
		return state.(*nuiDiagnosticState)
	}
	state := &nuiDiagnosticState{published: make(map[string]struct{})}
	actual, _ := nuiDiagnosticStates.LoadOrStore(s, state)
	return actual.(*nuiDiagnosticState)
}

var (
	nuiCallbackLuaRE = regexp.MustCompile(`(?m)\bRegisterNUICallback\s*\(\s*['"]([^'"]+)['"]`)
	nuiMessageRE     = regexp.MustCompile(`(?m)\bSendNUIMessage\s*\(\s*\{[^}]*?\b(action|type)\s*=\s*['"]([^'"]+)['"]`)
	// These patterns cover the conventional fetch/$.post NUI callback forms.
	nuiCallbackJSRE = regexp.MustCompile(`(?i)(?:GetParentResourceName\(\)|['"]https?://[^'"]+)/(?:[^/"'` + "`" + `]*?/)?([A-Za-z_][A-Za-z0-9_:-]*)['"` + "`" + `]`)
	nuiMessageJSRE  = regexp.MustCompile(`(?i)\b(?:event|e)\.data\.(?:action|type)\s*={1,3}\s*['"]([^'"]+)['"]`)
)

func (s *Server) nuiResourceFiles(doc *Document) []nuiContractFile {
	if s == nil || doc == nil {
		return nil
	}
	root := s.getDocResourceRoot(doc)
	if root == "" {
		return nil
	}
	rootPath := s.uriToPath(root)
	if rootPath == "" {
		rootPath = root
	}
	var out []nuiContractFile
	_ = filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || strings.Contains(filepath.ToSlash(path), "/node_modules/") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".html" && ext != ".js" && ext != ".ts" && ext != ".jsx" && ext != ".tsx" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		uri := s.pathToURI(path)
		if opened := s.Documents[uri]; opened != nil && len(opened.Source()) != 0 {
			src = opened.Source()
		}
		out = append(out, nuiContractFile{uri: uri, src: src, names: nuiJSNames(src, ext == ".html")})
		return nil
	})
	return out
}

func (s *Server) collectFiveMNUIResourceFacts() map[string]nuiResourceFacts {
	resources := make(map[string]nuiResourceFacts)
	// Only Lua documents identify a resource for NUI asset discovery, matching
	// the standalone NUI publication behavior.
	for _, doc := range sortedFiveMDocuments(s.Documents) {
		if !strings.EqualFold(filepath.Ext(doc.Path), ".lua") {
			continue
		}
		root := s.getDocResourceRoot(doc)
		if root == "" {
			continue
		}
		if _, ok := resources[root]; ok {
			continue
		}
		resources[root] = nuiResourceFacts{
			profile:   s.getDocumentFiveMProfile(doc),
			files:     s.nuiResourceFiles(doc),
			callbacks: make(map[string]bool),
			messages:  make(map[string]bool),
		}
	}
	// Open NUI assets can be Documents too, so include every document in the
	// resource when collecting Lua-side contract references.
	for _, doc := range sortedFiveMDocuments(s.Documents) {
		root := s.getDocResourceRoot(doc)
		resource, ok := resources[root]
		if !ok {
			continue
		}
		for _, m := range nuiCallbackLuaRE.FindAllSubmatchIndex(doc.Source(), -1) {
			resource.callbacks[string(doc.Source()[m[2]:m[3]])] = true
		}
		for _, m := range nuiMessageRE.FindAllSubmatchIndex(doc.Source(), -1) {
			resource.messages[string(doc.Source()[m[4]:m[5]])] = true
		}
		resources[root] = resource
	}
	return resources
}

func nuiJSNames(src []byte, html bool) []nuiContractName {
	var out []nuiContractName
	for _, m := range nuiCallbackJSRE.FindAllSubmatchIndex(src, -1) {
		off := m[2]
		out = append(out, nuiContractName{name: string(src[m[2]:m[3]]), offset: off})
	}
	for _, m := range nuiMessageJSRE.FindAllSubmatchIndex(src, -1) {
		out = append(out, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
	}
	return out
}

func (s *Server) buildFiveMNUIContractDiagnostics(doc *Document) []Diagnostic {
	if s == nil || doc == nil || s.getDocumentFiveMProfile(doc).ResourceRoot == "" {
		return nil
	}
	root := s.getDocResourceRoot(doc)
	var files []nuiContractFile
	callbacks, messages := map[string]bool{}, map[string]bool{}
	if facts := s.workspaceDiagnosticFacts(); facts != nil {
		resource, ok := facts.nui[root]
		if !ok {
			return nil
		}
		files, callbacks, messages = resource.files, resource.callbacks, resource.messages
	} else {
		files = s.nuiResourceFiles(doc)
		for _, d := range s.Documents {
			if d == nil || s.getDocResourceRoot(d) != root {
				continue
			}
			for _, m := range nuiCallbackLuaRE.FindAllSubmatchIndex(d.Source(), -1) {
				callbacks[string(d.Source()[m[2]:m[3]])] = true
			}
			for _, m := range nuiMessageRE.FindAllSubmatchIndex(d.Source(), -1) {
				messages[string(d.Source()[m[4]:m[5]])] = true
			}
		}
	}
	if len(files) == 0 {
		return nil
	}
	var out []Diagnostic
	// Diagnostics for the current Lua document are missing contracts.
	if strings.EqualFold(filepath.Ext(doc.Path), ".lua") {
		for _, m := range nuiCallbackLuaRE.FindAllSubmatchIndex(doc.Source(), -1) {
			name := string(doc.Source()[m[2]:m[3]])
			if !hasNUIName(files, name, false) {
				out = append(out, nuiDiag(doc.Source(), m[2], m[3], "fivem-nui-missing-handler", fmt.Sprintf("NUI callback '%s' has no local JavaScript handler.", name)))
			}
		}
		for _, m := range nuiMessageRE.FindAllSubmatchIndex(doc.Source(), -1) {
			name := string(doc.Source()[m[4]:m[5]])
			if !hasNUIName(files, name, true) {
				out = append(out, nuiDiag(doc.Source(), m[4], m[5], "fivem-nui-missing-handler", fmt.Sprintf("NUI message '%s' has no local JavaScript handler.", name)))
			}
		}
	}
	// When an HTML/JS document is explicitly indexed, report handlers never
	// referenced by Lua. (Workspace files are published below as well.)
	for _, f := range files {
		if f.uri != doc.URI {
			continue
		}
		for _, n := range f.names {
			if !callbacks[n.name] && !messages[n.name] {
				out = append(out, nuiDiag(f.src, n.offset, n.offset+len(n.name), "fivem-nui-unused-handler", fmt.Sprintf("NUI handler '%s' is not referenced by Lua.", n.name)))
			}
		}
	}
	return out
}

func hasNUIName(files []nuiContractFile, name string, message bool) bool {
	for _, f := range files {
		for _, n := range f.names {
			if n.name == name {
				return true
			}
		}
	}
	return false
}

func nuiDiag(src []byte, start, end int, code, message string) Diagnostic {
	return Diagnostic{Range: sourceRange(src, start, end), Severity: SeverityWarning, Code: code, Message: message}
}

func sourceRange(src []byte, start, end int) Range {
	line, col := 0, 0
	for i := 0; i < start && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	endLine, endCol := line, col
	for i := start; i < end && i < len(src); i++ {
		if src[i] == '\n' {
			endLine++
			endCol = 0
		} else {
			endCol++
		}
	}
	return Range{Start: Position{Line: uint32(line), Character: uint32(col)}, End: Position{Line: uint32(endLine), Character: uint32(endCol)}}
}
