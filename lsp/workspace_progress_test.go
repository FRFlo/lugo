package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestWorkspaceProgressCoversDiagnostics(t *testing.T) {
	h := newFiveMFixtureHarnessWithoutIndex(t)
	h.writeWorkspaceFile("progress.lua", "local value = 1\nprint(value)\n")
	h.server.workDoneProgressSupport = true
	h.server.refreshWorkspace()

	r := bufio.NewReader(bytes.NewReader(h.rpcOut.Bytes()))
	var events []string
	for {
		message, err := ReadMessage(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var wire struct {
			Method string `json:"method"`
			Params struct {
				Value struct {
					Kind    string `json:"kind"`
					Message string `json:"message"`
				} `json:"value"`
			} `json:"params"`
		}
		if err := json.Unmarshal(message, &wire); err != nil {
			t.Fatal(err)
		}
		if wire.Method == "$/progress" {
			events = append(events, wire.Params.Value.Kind+":"+wire.Params.Value.Message)
		}
	}
	if len(events) < 3 || events[0] != "begin:" || events[len(events)-2] != "report:Analyzing diagnostics: 1/1 files" || !strings.HasPrefix(events[len(events)-1], "end:") {
		t.Fatalf("unexpected progress sequence: %v", events)
	}
}
