// Command workspace-perf runs Lugo's CI indexing and diagnostics over a workspace.
// It does not write to the workspace; CI output goes to stdout and progress to stderr.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/FRFlo/lugo/lsp"
)

func main() {
	os.Exit(run())
}

func run() int {
	workspace := flag.String("workspace", "", "workspace root to index (required)")
	profile := flag.String("cpuprofile", "", "write a Go CPU profile to this path")
	profileSeconds := flag.Int("profile-seconds", 0, "stop CPU collection after this many seconds; zero collects through completion")
	sarif := flag.String("sarif", "", "write diagnostics as SARIF to this path")
	flag.Parse()
	if *workspace == "" {
		fmt.Fprintln(os.Stderr, "-workspace is required")
		return 2
	}
	if journal := telemetryJournalPath(); journal != "" {
		if err := ensureOutsideWorkspace(*workspace, journal); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	root, err := filepath.Abs(*workspace)
	if fatalIf(err) != 0 {
		return 2
	}
	for _, output := range []string{*profile, *sarif} {
		if output != "" {
			if err := ensureOutsideWorkspace(root, output); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
		}
	}

	config, err := os.CreateTemp("", "lugo-workspace-perf-*.json")
	if fatalIf(err) != 0 {
		return 2
	}
	configPath := config.Name()
	defer os.Remove(configPath)
	if fatalIf(json.NewEncoder(config).Encode(makeCIConfig(root, *sarif))) != 0 {
		_ = config.Close()
		return 2
	}
	if fatalIf(config.Close()) != 0 {
		return 2
	}

	var profileFile *os.File
	var stopOnce sync.Once
	stopProfile := func() {
		stopOnce.Do(func() {
			pprof.StopCPUProfile()
			_ = profileFile.Close()
		})
	}
	if *profile != "" {
		profileFile, err = os.Create(*profile)
		if fatalIf(err) != 0 {
			return 2
		}
		if fatalIf(pprof.StartCPUProfile(profileFile)) != 0 {
			_ = profileFile.Close()
			return 2
		}
		if *profileSeconds > 0 {
			time.AfterFunc(time.Duration(*profileSeconds)*time.Second, stopProfile)
		}
	}

	start := time.Now()
	// Keep the production binary's default telemetry behavior: it initializes
	// before creating the server and closes after CI has flushed its queue.
	tel, err := lsp.InitTelemetry("workspace-perf")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Telemetry initialization failed")
	} else if tel != nil {
		defer tel.Close()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			lsp.CapturePanic(recovered, "workspace-perf")
			lsp.FlushTelemetry()
			panic(recovered)
		}
	}()

	code := lsp.NewServer("workspace-perf").RunCI(configPath)
	if tel != nil {
		_ = tel.Flush()
		tel.Close()
	}
	if profileFile != nil {
		stopProfile()
	}
	fmt.Fprintf(os.Stderr, "workspace-perf wall=%s exit=%d\n", time.Since(start), code)
	return code
}

func telemetryJournalPath() string {
	if path := os.Getenv("LUGO_TRACE_JOURNAL"); path != "" {
		return path
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "lugo", "trace.ndjson")
}

func makeCIConfig(root, sarifPath string) map[string]any {
	config := map[string]any{
		"workspaceFolders": []string{root},
		"settings":         map[string]any{},
	}
	if sarifPath != "" {
		config["ciPolicy"] = map[string]string{"sarifPath": sarifPath}
	}
	return config
}

func fatalIf(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func ensureOutsideWorkspace(root, output string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return err
	}
	resolvedOutput, err := resolveOutputPath(output)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedOutput)
	if err != nil {
		return err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return fmt.Errorf("output path %q must be outside workspace %q", resolvedOutput, resolvedRoot)
	}
	return nil
}

// resolveOutputPath resolves symlinked existing components while allowing the
// final output file (and any not-yet-created parents) to be absent.
func resolveOutputPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for candidate := filepath.Clean(absPath); ; candidate = filepath.Dir(candidate) {
		resolved, evalErr := filepath.EvalSymlinks(candidate)
		if evalErr == nil {
			relativeSuffix, relErr := filepath.Rel(candidate, filepath.Clean(absPath))
			if relErr != nil {
				return "", relErr
			}
			return filepath.Join(resolved, relativeSuffix), nil
		}
		if !os.IsNotExist(evalErr) {
			return "", evalErr
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", evalErr
		}
	}
}
