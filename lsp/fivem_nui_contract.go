package lsp

import (
	"bytes"
	"context"
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
	// Keep the template spelling out of the general callback regexp. Almost all
	// assets lack template interpolation, and this conditional scan avoids
	// adding another branch to the regexp's hot path.
	nuiCallbackJSTemplateRE = regexp.MustCompile(`(?i)\$\{\s*GetParentResourceName\(\)\s*\}/(?:[^/"'` + "`" + `]*?/)?([A-Za-z_][A-Za-z0-9_:-]*)['"` + "`" + `]`)
	nuiCallbackJSCombinedRE = regexp.MustCompile(`(?i)(?:GetParentResourceName\(\)|\$\{\s*GetParentResourceName\(\)\s*\}|['"]https?://[^'"]+)/(?:[^/"'` + "`" + `]*?/)?([A-Za-z_][A-Za-z0-9_:-]*)['"` + "`" + `]`)
	nuiMessageJSRE          = regexp.MustCompile(`(?i)\b(?:event|e)\.data\.(?:action|type)\s*={1,3}\s*['"]([^'"]+)['"]`)
	nuiCallbackJSAnchoredRE = regexp.MustCompile(`^(?:` + nuiCallbackJSCombinedRE.String() + `)`)
	nuiMessageJSAnchoredRE  = regexp.MustCompile(`^(?:` + nuiMessageJSRE.String() + `)`)
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
	var traversalFailures, readFailures int
	_ = filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			traversalFailures = boundedNUIFailureCount(traversalFailures)
			return nil
		}
		if entry.IsDir() || strings.Contains(filepath.ToSlash(path), "/node_modules/") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".html" && ext != ".js" && ext != ".ts" && ext != ".jsx" && ext != ".tsx" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			readFailures = boundedNUIFailureCount(readFailures)
			return nil
		}
		uri := s.pathToURI(path)
		if opened := s.Documents[uri]; opened != nil && len(opened.Source()) != 0 {
			src = opened.Source()
		}
		out = append(out, nuiContractFile{uri: uri, src: src, names: nuiJSNames(src, ext == ".html")})
		return nil
	})
	if traversalFailures > 0 || readFailures > 0 {
		RecordTelemetry(WithTraceContext(context.Background(), s.trace), "lugo.fivem_nui_scan.failure", map[string]any{
			"traversal_failures": traversalFailures,
			"read_failures":      readFailures,
		})
	}
	return out
}

func boundedNUIFailureCount(count int) int {
	if count < 1000 {
		return count + 1
	}
	return count
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
		src := doc.Source()
		if bytes.Contains(src, []byte("RegisterNUICallback")) {
			for _, m := range nuiCallbackLuaRE.FindAllSubmatchIndex(src, -1) {
				resource.callbacks[string(src[m[2]:m[3]])] = true
			}
		}
		if bytes.Contains(src, []byte("SendNUIMessage")) {
			for _, m := range nuiMessageRE.FindAllSubmatchIndex(src, -1) {
				resource.messages[string(src[m[4]:m[5]])] = true
			}
		}
		resources[root] = resource
	}
	return resources
}

func nuiJSNames(src []byte, html bool) []nuiContractName {
	resourceNamePresent := nuiContainsFolded(src, "getparentresourcename")
	longSPresent := bytes.Contains(src, []byte("ſ"))
	// The historical split/template selection is subtle when both expressions
	// match. Retain it for actual template callback matches, while allowing
	// unrelated interpolation syntax through the candidate scan.
	if bytes.Contains(src, []byte("${")) && (resourceNamePresent || longSPresent) && nuiCallbackJSTemplateRE.Match(src) {
		return nuiJSNamesRegex(src)
	}
	var out []nuiContractName
	var budget int64 = int64(len(src)) * 64
	for pos := 0; pos < len(src); {
		next := bytes.IndexAny(src[pos:], "'\"Gg$")
		if next < 0 {
			break
		}
		pos += next
		possible := false
		if pos+1 < len(src) {
			switch src[pos] {
			case '\'', '"':
				possible = nuiHasFoldedPrefix(src[pos+1:], "http")
			case 'g', 'G':
				// Stop before the s that may be Unicode long s in the regexp fold.
				possible = nuiHasFoldedPrefix(src[pos:], "getparentre")
			case '$':
				possible = src[pos+1] == '{'
			}
		}
		if possible {
			end := len(src)
			if quote := bytes.IndexAny(src[pos+1:], "'\""); quote >= 0 {
				end = pos + 1 + quote + 1
			}
			cost := int64(end - pos)
			if cost > budget {
				return nuiJSNamesRegex(src)
			}
			budget -= cost
			if match := nuiCallbackJSAnchoredRE.FindSubmatchIndex(src[pos:end]); match != nil {
				out = append(out, nuiContractName{name: string(src[pos+match[2] : pos+match[3]]), offset: pos + match[2]})
				pos += match[1]
				continue
			}
		}
		pos++
	}
	for pos := 0; pos < len(src); {
		next := bytes.IndexAny(src[pos:], "eE")
		if next < 0 {
			break
		}
		pos += next
		word := false
		if pos > 0 {
			c := src[pos-1]
			word = c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
		}
		if !word && (nuiHasFoldedPrefix(src[pos:], "e.data.") || nuiHasFoldedPrefix(src[pos:], "event.data.")) {
			cost := int64(len(src) - pos)
			if cost > budget {
				return nuiJSNamesRegex(src)
			}
			budget -= cost
			if match := nuiMessageJSAnchoredRE.FindSubmatchIndex(src[pos:]); match != nil {
				out = append(out, nuiContractName{name: string(src[pos+match[2] : pos+match[3]]), offset: pos + match[2]})
				pos += match[1]
				continue
			}
		}
		pos++
	}
	return out
}

// nuiJSNamesRegex is the full-source fallback for the bounded candidate scan.
func nuiJSNamesRegex(src []byte) []nuiContractName {
	var out []nuiContractName
	var callbacks []nuiContractName
	resourceNamePresent := nuiContainsFolded(src, "getparentresourcename")
	longSPresent := bytes.Contains(src, []byte("ſ"))
	if resourceNamePresent || nuiContainsFolded(src, "http") || longSPresent {
		for _, m := range nuiCallbackJSRE.FindAllSubmatchIndex(src, -1) {
			callbacks = append(callbacks, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
		}
	}
	var templateCallbacks []nuiContractName
	if bytes.Contains(src, []byte("${")) && (resourceNamePresent || longSPresent) {
		for _, m := range nuiCallbackJSTemplateRE.FindAllSubmatchIndex(src, -1) {
			templateCallbacks = append(templateCallbacks, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
		}
	}
	if len(callbacks) == 0 {
		callbacks = templateCallbacks
	} else if len(templateCallbacks) != 0 {
		callbacks = callbacks[:0]
		for _, m := range nuiCallbackJSCombinedRE.FindAllSubmatchIndex(src, -1) {
			callbacks = append(callbacks, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
		}
	}
	out = append(out, callbacks...)
	if nuiContainsFolded(src, "event.data.") || nuiContainsFolded(src, "e.data.") {
		for _, m := range nuiMessageJSRE.FindAllSubmatchIndex(src, -1) {
			out = append(out, nuiContractName{name: string(src[m[2]:m[3]]), offset: m[2]})
		}
	}
	return out
}

func nuiHasFoldedPrefix(src []byte, prefix string) bool {
	if len(src) < len(prefix) {
		return false
	}
	for i := range len(prefix) {
		c := src[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}

func nuiContainsFolded(src []byte, needle string) bool {
	if len(needle) == 0 || len(needle) > len(src) {
		return false
	}
	for i := 0; i <= len(src)-len(needle); i++ {
		for j := range len(needle) {
			b := src[i+j]
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if b != needle[j] {
				break
			}
			if j == len(needle)-1 {
				return true
			}
		}
	}
	return false
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
			src := d.Source()
			if bytes.Contains(src, []byte("RegisterNUICallback")) {
				for _, m := range nuiCallbackLuaRE.FindAllSubmatchIndex(src, -1) {
					callbacks[string(src[m[2]:m[3]])] = true
				}
			}
			if bytes.Contains(src, []byte("SendNUIMessage")) {
				for _, m := range nuiMessageRE.FindAllSubmatchIndex(src, -1) {
					messages[string(src[m[4]:m[5]])] = true
				}
			}
		}
	}
	if len(files) == 0 {
		return nil
	}
	var out []Diagnostic
	// Diagnostics for the current Lua document are missing contracts.
	if strings.EqualFold(filepath.Ext(doc.Path), ".lua") {
		src := doc.Source()
		if bytes.Contains(src, []byte("RegisterNUICallback")) {
			for _, m := range nuiCallbackLuaRE.FindAllSubmatchIndex(src, -1) {
				name := string(src[m[2]:m[3]])
				if !hasNUIName(files, name, false) {
					out = append(out, nuiDiag(src, m[2], m[3], "fivem-nui-missing-handler", fmt.Sprintf("NUI callback '%s' has no local JavaScript handler.", name)))
				}
			}
		}
		if bytes.Contains(src, []byte("SendNUIMessage")) {
			for _, m := range nuiMessageRE.FindAllSubmatchIndex(src, -1) {
				name := string(src[m[4]:m[5]])
				if !hasNUIName(files, name, true) {
					out = append(out, nuiDiag(src, m[4], m[5], "fivem-nui-missing-handler", fmt.Sprintf("NUI message '%s' has no local JavaScript handler.", name)))
				}
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
