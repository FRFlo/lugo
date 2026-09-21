package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/coalaura/plain"
)

type ciDiagnostic struct {
	URI  string
	Diag Diagnostic
}

type ciState struct {
	policy   CIPolicy
	diags    []ciDiagnostic
	failures int
}

var ciStates sync.Map // map[*Server]*ciState; CI servers are short-lived.

func setCIPolicy(s *Server, policy CIPolicy) {
	ciStates.Store(s, &ciState{policy: policy})
}

func getCIState(s *Server) *ciState {
	if state, ok := ciStates.Load(s); ok {
		return state.(*ciState)
	}
	state := &ciState{}
	ciStates.Store(s, state)
	return state
}

// RunCI executes the server in continuous integration mode using a config file.
// It returns an exit code where 0 indicates success and 1 indicates failure or diagnostics with errors.
func (s *Server) RunCI(configPath string) int {
	s.IsCI = true

	s.Log = plain.New(
		plain.WithTarget(os.Stderr),
		plain.WithDate(plain.RFC3339Local),
	)

	b, err := os.ReadFile(configPath)
	if err != nil {
		s.Log.Errorf("Failed to read CI config: %v\n", err)
		return 1
	}

	var cfg CIConfig
	if err = json.Unmarshal(b, &cfg); err != nil {
		s.Log.Errorf("Failed to parse CI config: %v\n", err)
		return 1
	}
	if cfg.CIPolicy.FailOnSeverity == "" {
		cfg.CIPolicy.FailOnSeverity = "error"
	}
	setCIPolicy(s, cfg.CIPolicy)

	for _, folder := range cfg.WorkspaceFolders {
		absPath, _ := filepath.Abs(folder)
		uri := s.normalizeURI(s.pathToURI(absPath))
		s.WorkspaceFolders = append(s.WorkspaceFolders, uri)
		s.lowerWorkspaceFolders = append(s.lowerWorkspaceFolders, strings.ToLower(s.uriToPath(uri)))
	}
	if len(s.WorkspaceFolders) > 0 {
		s.RootURI = s.WorkspaceFolders[0]
		s.lowerRootPath = s.lowerWorkspaceFolders[0]
	}

	s.applyInitializationOptions(cfg.Settings)
	s.refreshWorkspace()
	s.writeCISARIF(configPath)

	state := getCIState(s)
	s.Log.Printf("CI completed. Found %d diagnostics (%d errors).\n", s.CIDiagnosticCount, s.CIErrorCount)
	if state.failures > 0 {
		return 1
	}
	return 0
}

func (s *Server) printCIDiagnostics(uri string, diags []Diagnostic) {
	state := getCIState(s)
	for _, diag := range diags {
		if !ciCodeAllowed(state.policy, diag.Code) {
			continue
		}
		state.diags = append(state.diags, ciDiagnostic{URI: uri, Diag: diag})
		s.CIDiagnosticCount++
		if diag.Severity == SeverityError {
			s.CIErrorCount++
		}

		if ciSeverityFails(state.policy.FailOnSeverity, diag.Severity) {
			state.failures++
		}
		level := "warning"
		switch diag.Severity {
		case SeverityError:
			level = "error"
		case SeverityHint, SeverityInformation:
			level = "notice"
		}
		path := s.uriToPath(uri)
		line := diag.Range.Start.Line + 1
		col := diag.Range.Start.Character + 1
		fmt.Fprintf(s.Writer, "::%s file=%s,line=%d,col=%d::%s\n", level, ciEscape(path, true), line, col, ciEscape(strings.ReplaceAll(diag.Message, "\n", " "), false))
	}
	if p := state.policy; p.MaxDiagnostics > 0 && s.CIDiagnosticCount > p.MaxDiagnostics {
		state.failures = maxInt(state.failures, 1)
	}
	if p := state.policy; p.MaxErrors > 0 && s.CIErrorCount > p.MaxErrors {
		state.failures = maxInt(state.failures, 1)
	}
	warnings := s.CIDiagnosticCount - s.CIErrorCount
	if p := state.policy; p.MaxWarnings > 0 && warnings > p.MaxWarnings {
		state.failures = maxInt(state.failures, 1)
	}
}

func ciCodeAllowed(policy CIPolicy, code string) bool {
	if len(policy.IncludeCodes) > 0 && !containsString(policy.IncludeCodes, code) {
		return false
	}
	return !containsString(policy.ExcludeCodes, code)
}

func ciSeverityFails(threshold string, severity DiagnosticSeverity) bool {
	threshold = strings.ToLower(threshold)
	if threshold == "" {
		threshold = "error"
	}
	if threshold == "none" || threshold == "off" {
		return false
	}
	min := map[string]DiagnosticSeverity{"error": SeverityError, "warning": SeverityWarning, "information": SeverityInformation, "hint": SeverityHint}[threshold]
	return min != 0 && severity <= min
}

func ciEscape(value string, properties bool) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "\r", "%0D")
	value = strings.ReplaceAll(value, "\n", "%0A")
	if properties {
		value = strings.ReplaceAll(value, ",", "%2C")
		value = strings.ReplaceAll(value, ":", "%3A")
	}
	return value
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Server) writeCISARIF(configPath string) {
	state := getCIState(s)
	if state.policy.SARIFPath == "" {
		return
	}
	type sarifResult struct {
		RuleID    string            `json:"ruleId,omitempty"`
		Level     string            `json:"level"`
		Message   map[string]string `json:"message"`
		Locations []map[string]any  `json:"locations"`
	}
	results := make([]sarifResult, 0, len(state.diags))
	for _, item := range state.diags {
		level := "warning"
		if item.Diag.Severity == SeverityError {
			level = "error"
		}
		path := s.uriToPath(item.URI)
		results = append(results, sarifResult{item.Diag.Code, level, map[string]string{"text": item.Diag.Message}, []map[string]any{{"physicalLocation": map[string]any{"artifactLocation": map[string]string{"uri": path}, "region": map[string]uint32{"startLine": item.Diag.Range.Start.Line + 1, "startColumn": item.Diag.Range.Start.Character + 1}}}}})
	}
	payload := map[string]any{"version": "2.1.0", "$schema": "https://json.schemastore.org/sarif-2.1.0.json", "runs": []any{map[string]any{"tool": map[string]any{"driver": map[string]string{"name": "lugo"}}, "results": results}}}
	path := state.policy.SARIFPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(configPath), path)
	}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(path, b, 0644)
	}
}
