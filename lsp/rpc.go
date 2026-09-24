package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var contentLengthPrefix = []byte("Content-Length: ")

const maxMessageSize = 100 * 1024 * 1024

// JSON-RPC messages must be written as complete frames. The server performs
// workspace indexing asynchronously, so notifications can otherwise
// interleave with request responses on stdout.
var writeMu sync.Mutex
var outgoingRequestID atomic.Uint64

func nextOutgoingRequestID() uint64 {
	// Keep server-originated request IDs monotonically increasing so
	// overlapping indexing/reindex requests cannot reuse an ID.
	return outgoingRequestID.Add(1)
}

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

		line = bytes.TrimSuffix(line, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))
		separator := bytes.IndexByte(line, ':')
		if separator >= 0 && strings.EqualFold(string(bytes.TrimSpace(line[:separator])), "Content-Length") {
			valBytes := bytes.TrimSpace(line[separator+1:])
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
	err := writeMessageRaw(w, msg)
	if err != nil {
		recordTransportWriteFailure(context.Background(), "unclassified", err)
	}
	return err
}

// writeMessageRaw is used by the protocol dispatcher when it can attach a
// more useful response path to a failed write. All other writers go through
// WriteMessage so failures from feature and notification handlers are covered.
func writeMessageRaw(w io.Writer, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	frame := make([]byte, 0, len(header)+len(body))
	frame = append(frame, header...)
	frame = append(frame, body...)

	writeMu.Lock()
	defer writeMu.Unlock()
	n, err := w.Write(frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}

	return nil
}

// writeProtocolResponse writes a response emitted by the JSON-RPC dispatch
// boundary. A write failure is still returned to preserve the caller's existing
// behavior; recording it is best-effort and cannot turn it into a panic.
func writeProtocolResponse(ctx context.Context, w io.Writer, responsePath string, msg any) (err error) {
	err = writeMessageRaw(w, msg)
	if err != nil {
		recordTransportWriteFailure(ctx, responsePath, err)
	}
	return err
}

func recordTransportWriteFailure(ctx context.Context, responsePath string, writeErr error) {
	defer func() {
		// Observability must never disrupt the protocol failure path.
		_ = recover()
	}()
	RecordTelemetry(ctx, "lsp_transport_write_failed", map[string]any{
		"response_path": responsePath,
		"error_type":    fmt.Sprintf("%T", writeErr),
	})
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
