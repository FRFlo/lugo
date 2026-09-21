package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestReadMessageContentLengthValidation(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "valid", header: "5", want: "hello"},
		{name: "overflow", header: "18446744073709551616"},
		{name: "negative", header: "-1"},
		{name: "non numeric", header: "5x"},
		{name: "too large", header: "104857601"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "Content-Length: " + tt.header + "\r\n\r\nhello"
			got, err := ReadMessage(bufio.NewReader(strings.NewReader(input)))
			if tt.want == "" {
				if err == nil {
					t.Fatalf("ReadMessage() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadMessage() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("ReadMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadMessageAcceptsCaseInsensitiveContentLength(t *testing.T) {
	input := "content-length: 5\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\nhello"
	got, err := ReadMessage(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("ReadMessage() = %q, want hello", got)
	}
}

func TestWriteMessageWritesOneCompleteFrame(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMessage(&buf, Response{RPC: "2.0", ID: "request-1", Result: true}); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	frame := buf.String()
	if !strings.HasPrefix(frame, "Content-Length: ") || !strings.Contains(frame, "\r\n\r\n{\"jsonrpc\":\"2.0\"") {
		t.Fatalf("invalid JSON-RPC frame: %q", frame)
	}
	body, err := ReadMessage(bufio.NewReader(strings.NewReader(frame)))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.ID != "request-1" {
		t.Fatalf("response ID = %#v, want request-1", response.ID)
	}
}

func TestRequestPreservesLargeNumericID(t *testing.T) {
	const input = `{"jsonrpc":"2.0","id":9007199254740993,"method":"test"}`
	var req Request
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	body, err := json.Marshal(Response{RPC: "2.0", ID: req.ID, Result: true})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !bytes.Contains(body, []byte(`"id":9007199254740993`)) {
		t.Fatalf("large request ID was not preserved: %s", body)
	}
}

func TestReadMessageDoesNotPanicOnContentLengthOverflow(t *testing.T) {
	input := "Content-Length: 9223372036854775808\r\n\r\n"
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("ReadMessage panicked: %v", recovered)
		}
	}()

	_, err := ReadMessage(bufio.NewReader(bytes.NewBufferString(input)))
	if err == nil {
		t.Fatal("ReadMessage() error = nil, want error")
	}
}

func TestReadMessageRejectsOversizedHeaderLine(t *testing.T) {
	input := "X-Header: " + strings.Repeat("x", 8192) + "\r\nContent-Length: 5\r\n\r\nhello"
	if _, err := ReadMessage(bufio.NewReader(strings.NewReader(input))); err == nil {
		t.Fatal("ReadMessage() error = nil, want oversized-header error")
	}
}
