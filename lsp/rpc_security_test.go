package lsp

import (
	"bufio"
	"bytes"
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
