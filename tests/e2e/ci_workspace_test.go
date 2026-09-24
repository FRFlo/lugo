package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCIProcessFiveMWorkspace(t *testing.T) {
	root := copyFixture(t)
	config, err := json.Marshal(map[string]any{
		"workspaceFolders": []string{root},
		"settings": map[string]any{
			"diagFiveMUnknownExport":   true,
			"diagFiveMUnknownResource": true,
			"diagFiveMUnknownEvent":    true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "lugo-ci.json")
	if err := os.WriteFile(path, config, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryEnv(t, "LUGO_BIN"), "--ci", path)
	cmd.Env = append(os.Environ(), "LUGO_TELEMETRY=false")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CI process: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "provider") || !strings.Contains(string(output), "consumer") {
		t.Fatalf("CI output omitted FiveM workspace resources: %s", output)
	}
}
