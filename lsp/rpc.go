package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

var contentLengthPrefix = []byte("Content-Length: ")

const maxMessageSize = 100 * 1024 * 1024

// ReadMessage reads a JSON-RPC message from a buffered reader.
// It parses the Content-Length header and returns the raw message body.
func ReadMessage(r *bufio.Reader) ([]byte, error) {
	var length uint64
	var foundLength bool

	for {
		line, err := r.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			return nil, fmt.Errorf("header line too long")
		}
		if err != nil {
			return nil, err
		}

		if bytes.Equal(line, []byte("\r\n")) || bytes.Equal(line, []byte("\n")) {
			break
		}

		if bytes.HasPrefix(line, contentLengthPrefix) {
			valBytes := bytes.TrimSpace(line[len(contentLengthPrefix):])
			if len(valBytes) == 0 {
				return nil, fmt.Errorf("invalid content length")
			}
			for _, b := range valBytes {
				if b < '0' || b > '9' {
					return nil, fmt.Errorf("invalid content length")
				}
			}

			parsed, err := strconv.ParseUint(string(valBytes), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid content length: %w", err)
			}
			length = parsed
			foundLength = true
		}
	}

	if !foundLength || length == 0 {
		return nil, fmt.Errorf("missing content length")
	}

	if length > maxMessageSize { // 100MB hard limit
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}

	content := make([]byte, length)

	_, err := io.ReadFull(r, content)
	if err != nil {
		return nil, err
	}

	return content, nil
}

// WriteMessage writes a JSON-RPC message to an io.Writer.
// It marshals the message to JSON and prepends the Content-Length header.
func WriteMessage(w io.Writer, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	_, err = w.Write([]byte(header))
	if err != nil {
		return err
	}

	_, err = w.Write(body)
	if err != nil {
		return err
	}

	return nil
}

// WriteNotification sends a JSON-RPC notification (no ID) to an io.Writer.
// Notifications do not expect a response from the client.
func WriteNotification(w io.Writer, method string, params any) error {
	return WriteMessage(w, OutgoingNotification{
		RPC:    "2.0",
		Method: method,
		Params: params,
	})
}
