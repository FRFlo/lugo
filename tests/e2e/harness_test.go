package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

type stdioProcess struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdoutPipe    io.ReadCloser
	stdout        *bufio.Reader
	stderr        bytes.Buffer
	mu            sync.Mutex
	notifications []map[string]any
}

func startProcess(t *testing.T, binary string, args ...string) *stdioProcess {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "LUGO_TELEMETRY=0", "LUGO_TELEMETRY_ENABLED=0")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	p := &stdioProcess{cmd: cmd, stdin: in, stdoutPipe: out, stdout: bufio.NewReader(out)}
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", binary, err)
	}
	t.Cleanup(func() {
		_ = in.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		_ = p.stdoutPipe.Close()
	})
	return p
}

func (p *stdioProcess) send(t *testing.T, message any) {
	t.Helper()
	body, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	if _, err := fmt.Fprintf(p.stdin, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		t.Fatalf("write message: %v", err)
	}
}

// MCP's StdioTransport uses newline-delimited JSON, while the LSP uses
// Content-Length framing. Keeping both writers in the harness makes the
// transport boundary explicit rather than accidentally relying on one format.
func (p *stdioProcess) sendLine(t *testing.T, message any) {
	t.Helper()
	body, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	if _, err := fmt.Fprintf(p.stdin, "%s\n", body); err != nil {
		t.Fatalf("write line message: %v", err)
	}
}

func (p *stdioProcess) read(ctx context.Context) ([]byte, error) {
	result := make(chan struct {
		body []byte
		err  error
	}, 1)
	go func() {
		var length int
		for {
			line, err := p.stdout.ReadString('\n')
			if err != nil {
				result <- struct {
					body []byte
					err  error
				}{err: err}
				return
			}
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "content-length:") {
				length, err = strconv.Atoi(strings.TrimSpace(strings.SplitN(line, ":", 2)[1]))
				if err != nil {
					result <- struct {
						body []byte
						err  error
					}{err: err}
					return
				}
			}
			if line == "\r\n" || line == "\n" {
				break
			}
		}
		body := make([]byte, length)
		_, err := io.ReadFull(p.stdout, body)
		result <- struct {
			body []byte
			err  error
		}{body: body, err: err}
	}()
	select {
	case r := <-result:
		return r.body, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *stdioProcess) notification(t *testing.T, ctx context.Context, method string, uri string) map[string]any {
	t.Helper()
	matches := func(msg map[string]any) bool {
		if msg["method"] != method {
			return false
		}
		if uri == "" {
			return true
		}
		params, _ := msg["params"].(map[string]any)
		gotURI, _ := params["uri"].(string)
		if strings.EqualFold(gotURI, uri) {
			return true
		}
		got, gotErr := url.Parse(gotURI)
		want, wantErr := url.Parse(uri)
		if gotErr != nil || wantErr != nil || got.Scheme != "file" || want.Scheme != "file" {
			return false
		}
		if runtime.GOOS == "windows" {
			return strings.EqualFold(strings.TrimPrefix(got.Path, "/"), strings.TrimPrefix(want.Path, "/"))
		}
		return got.Path == want.Path
	}
	p.mu.Lock()
	for i, msg := range p.notifications {
		if matches(msg) {
			p.notifications = append(p.notifications[:i], p.notifications[i+1:]...)
			p.mu.Unlock()
			return msg
		}
	}
	p.mu.Unlock()
	for {
		body, err := p.read(ctx)
		if err != nil {
			t.Fatalf("read notification %s for %s: %v (buffered: %v, stderr: %s)", method, uri, err, p.notifications, p.stderr.String())
		}
		var msg map[string]any
		if err := json.Unmarshal(body, &msg); err != nil {
			t.Fatalf("decode notification: %v", err)
		}
		if matches(msg) {
			return msg
		}
	}
}

func (p *stdioProcess) response(t *testing.T, ctx context.Context, id int) map[string]any {
	return p.responseWith(t, ctx, id, false)
}

func (p *stdioProcess) responseLine(t *testing.T, ctx context.Context, id int) map[string]any {
	return p.responseWith(t, ctx, id, true)
}

func (p *stdioProcess) responseWith(t *testing.T, ctx context.Context, id int, line bool) map[string]any {
	t.Helper()
	for {
		var body []byte
		var err error
		if line {
			body, err = p.readLine(ctx)
		} else {
			body, err = p.read(ctx)
		}
		if err != nil {
			t.Fatalf("read response %d: %v (stderr: %s)", id, err, p.stderr.String())
		}
		var msg map[string]any
		if err := json.Unmarshal(body, &msg); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		got, ok := msg["id"].(float64)
		if method, ok := msg["method"].(string); ok {
			p.mu.Lock()
			p.notifications = append(p.notifications, map[string]any{"method": method, "params": msg["params"]})
			p.mu.Unlock()
		}
		if ok && int(got) == id {
			return msg
		}
	}
}

func (p *stdioProcess) readLine(ctx context.Context) ([]byte, error) {
	result := make(chan struct {
		body []byte
		err  error
	}, 1)
	go func() {
		line, err := p.stdout.ReadBytes('\n')
		result <- struct {
			body []byte
			err  error
		}{body: bytes.TrimSpace(line), err: err}
	}()
	select {
	case r := <-result:
		return r.body, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "fixtures", "fivem-static")
}

func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	root := fixtureRoot(t)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

var builtBinaries struct {
	lugo string
	mcp  string
}

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "lugo-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	builtBinaries.lugo = os.Getenv("LUGO_BIN")
	builtBinaries.mcp = os.Getenv("LUGO_MCP_BIN")
	if builtBinaries.lugo == "" {
		builtBinaries.lugo = filepath.Join(tmp, "lugo-e2e")
	}
	if builtBinaries.mcp == "" {
		builtBinaries.mcp = filepath.Join(tmp, "lugo-mcp-e2e")
	}
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(strings.ToLower(builtBinaries.lugo), ".exe") {
			builtBinaries.lugo += ".exe"
		}
		if !strings.HasSuffix(strings.ToLower(builtBinaries.mcp), ".exe") {
			builtBinaries.mcp += ".exe"
		}
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(sourceFile)))
	for _, build := range [][2]string{{builtBinaries.lugo, "."}, {builtBinaries.mcp, "./cmd/lugo-mcp"}} {
		if _, err := os.Stat(build[0]); err == nil {
			continue
		}
		cmd := exec.Command("go", "build", "-o", build[0], build[1])
		cmd.Dir = moduleRoot
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n", build[1], err)
			os.Exit(1)
		}
	}
	code := m.Run()
	if err := os.RemoveAll(tmp); err != nil {
		fmt.Fprintf(os.Stderr, "remove temporary E2E binaries: %v\n", err)
	}
	os.Exit(code)
}

func binaryEnv(t *testing.T, name string) string {
	t.Helper()
	path := builtBinaries.lugo
	if name == "LUGO_MCP_BIN" {
		path = builtBinaries.mcp
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("E2E binary %s unavailable at %q: %v", name, path, err)
	}
	return path
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if runtime.GOOS == "windows" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}
