package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLSPProcessLifecycleHelper(t *testing.T) {
	mode := os.Getenv("LUGO_LIFECYCLE_HELPER")
	if mode == "" {
		return
	}
	if err := NewServer("test").Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	os.Exit(0)
}

func TestLSPProcessLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantExit   int
		wantErrors map[string]int
	}{
		{
			name: "nominal",
			input: framed(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`) +
				framed(`{"jsonrpc":"2.0","method":"initialized","params":{}}`) +
				framed(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) +
				framed(`{"jsonrpc":"2.0","method":"exit"}`),
			wantExit: 0,
		},
		{
			name: "request after shutdown",
			input: framed(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) +
				framed(`{"jsonrpc":"2.0","id":3,"method":"workspace/symbol","params":{"query":"x"}}`) +
				framed(`{"jsonrpc":"2.0","method":"exit"}`),
			wantExit:   0,
			wantErrors: map[string]int{"3": -32600},
		},
		{
			name:     "exit without shutdown",
			input:    framed(`{"jsonrpc":"2.0","method":"exit"}`),
			wantExit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLSPProcessLifecycleHelper$")
			cmd.Env = append(os.Environ(), "LUGO_LIFECYCLE_HELPER=1")
			cmd.Stdin = strings.NewReader(tt.input)
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = io.Discard
			err := cmd.Run()
			gotExit := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatalf("helper process error = %v", err)
				}
				gotExit = exitErr.ExitCode()
			}
			if gotExit != tt.wantExit {
				t.Fatalf("exit code = %d, want %d; output: %s", gotExit, tt.wantExit, stdout.String())
			}

			gotErrors := make(map[string]int)
			reader := bufio.NewReader(&stdout)
			for {
				body, readErr := ReadMessage(reader)
				if errors.Is(readErr, io.EOF) {
					break
				}
				if readErr != nil {
					t.Fatalf("ReadMessage() error = %v", readErr)
				}
				var response struct {
					ID    json.RawMessage `json:"id"`
					Error *struct {
						Code int `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(body, &response); err != nil {
					t.Fatalf("decode response %s: %v", body, err)
				}
				id := strings.Trim(string(response.ID), `"`)
				if response.Error != nil {
					gotErrors[id] = response.Error.Code
				}
			}
			if len(gotErrors) != len(tt.wantErrors) {
				t.Fatalf("error responses = %#v, want %#v", gotErrors, tt.wantErrors)
			}
			for id, code := range tt.wantErrors {
				if gotErrors[id] != code {
					t.Errorf("error for id %s = %d, want %d", id, gotErrors[id], code)
				}
			}
		})
	}
}

func TestLSPMalformedMessageAndEOF(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		code  int
	}{
		{name: "EOF", input: ""},
		{name: "malformed then EOF", input: framed("{"), code: -32700},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer("test")
			server.Reader = bufio.NewReader(strings.NewReader(tt.input))
			var output bytes.Buffer
			server.Writer = &output
			if err := server.Start(); err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			reader := bufio.NewReader(&output)
			if tt.code == 0 {
				if _, err := ReadMessage(reader); !errors.Is(err, io.EOF) {
					t.Fatalf("unexpected EOF response: %v", err)
				}
				return
			}
			body, err := ReadMessage(reader)
			if err != nil {
				t.Fatalf("ReadMessage() error = %v", err)
			}
			var response struct {
				Error struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Error.Code != tt.code {
				t.Errorf("parse error code = %d, want %d", response.Error.Code, tt.code)
			}
			if _, err := ReadMessage(reader); !errors.Is(err, io.EOF) {
				t.Errorf("trailing response error = %v, want EOF", err)
			}
		})
	}
}

func framed(body string) string {
	return rpcFrame(body)
}
