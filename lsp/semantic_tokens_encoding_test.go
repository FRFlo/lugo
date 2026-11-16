package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestSemanticTokensRespectNegotiatedPositionEncoding(t *testing.T) {
	const source = "local before = 'é💸'; print(before)\n"
	for _, encoding := range []string{"utf-8", "utf-16", "utf-32"} {
		t.Run(encoding, func(t *testing.T) {
			h := newFiveMFixtureHarnessWithoutIndex(t)
			h.writeWorkspaceFile("encoding.lua", source)
			h.reindex()
			server := h.server
			initialize, err := json.Marshal(InitializeParams{Capabilities: &ClientCapabilities{General: &GeneralClientCapabilities{PositionEncodings: []string{encoding}}}})
			if err != nil {
				t.Fatal(err)
			}
			server.handleInitialize(Request{RPC: "2.0", ID: 1, Params: initialize})
			if _, err := ReadMessage(bufio.NewReader(bytes.NewReader(h.rpcOut.Bytes()))); err != nil {
				t.Fatalf("read initialize response: %v", err)
			}
			h.resetRPC()
			doc := server.Documents[server.pathToURI(h.root+"/encoding.lua")]
			if doc == nil {
				t.Fatal("test document was not indexed")
			}

			full := semanticTokenData(t, server, doc.URI, false)
			ranged := semanticTokenData(t, server, doc.URI, true)
			if len(full) != len(ranged) {
				t.Fatalf("full/range token data lengths differ: %d != %d", len(full), len(ranged))
			}
			for i := range full {
				if full[i] != ranged[i] {
					t.Fatalf("full/range token data differ at %d: %v != %v", i, full, ranged)
				}
			}

			tokens := decodeSemanticTokenRanges(full)
			wantColumn := uint32(strings.Index(source, "print"))
			if encoding != "utf-8" {
				wantColumn = uint32(len([]rune(source[:strings.Index(source, "print")])))
				if encoding == "utf-16" {
					wantColumn++ // extra unit for the astral rune
				}
			}
			if !containsSemanticToken(tokens, Position{Line: 0, Character: wantColumn}, 3, uint32(len("print"))) {
				t.Fatalf("print token at column %d missing from %v", wantColumn, tokens)
			}
			stringColumn := uint32(strings.Index(source, "'é"))
			stringLength := uint32(len("'é💸'"))
			if encoding != "utf-8" {
				stringLength = 4
				if encoding == "utf-16" {
					stringLength++
				}
			}
			if !containsSemanticToken(tokens, Position{Line: 0, Character: stringColumn}, 7, stringLength) {
				t.Fatalf("string token at column %d with length %d missing from %v", stringColumn, stringLength, tokens)
			}
		})
	}
}

func TestSemanticTokensMatchReportedUnicodePositions(t *testing.T) {
	cases := []struct {
		name, line, symbol string
	}{
		{"bmp-accent", `local value = "résumé"; print(value)`, "print"},
		{"bmp-cjk", `local value = "漢字"; print(value)`, "print"},
		{"astral-currency", `local value = "💸"; print(value)`, "print"},
		{"astral-key", `local value = "🔑"; print(value)`, "print"},
		{"mixed-unicode", `local value = "é💸漢"; print(value)`, "print"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, encoding := range []string{"utf-16", "utf-8"} {
				t.Run(encoding, func(t *testing.T) {
					h := newFiveMFixtureHarnessWithoutIndex(t)
					h.writeWorkspaceFile("unicode.lua", tc.line+"\n")
					h.reindex()
					initialize, err := json.Marshal(InitializeParams{Capabilities: &ClientCapabilities{General: &GeneralClientCapabilities{PositionEncodings: []string{encoding}}}})
					if err != nil {
						t.Fatal(err)
					}
					h.server.handleInitialize(Request{RPC: "2.0", ID: 1, Params: initialize})
					h.resetRPC()
					uri := h.server.pathToURI(h.root + "/unicode.lua")
					data := semanticTokenData(t, h.server, uri, false)
					tokens := decodeSemanticTokenRanges(data)
					ranged := decodeSemanticTokenRanges(semanticTokenData(t, h.server, uri, true))
					byteColumn := strings.Index(tc.line, tc.symbol)
					column := uint32(byteColumn)
					if encoding == "utf-16" {
						column = uint32(len(utf16.Encode([]rune(tc.line[:byteColumn]))))
					}
					if !containsSemanticToken(tokens, Position{Character: column}, 3, uint32(len(tc.symbol))) {
						t.Fatalf("%s token range missing at column %d: %v", tc.symbol, column, tokens)
					}
					if !containsSemanticToken(ranged, Position{Character: column}, 3, uint32(len(tc.symbol))) {
						t.Fatalf("%s range token missing at column %d: %v", tc.symbol, column, ranged)
					}
				})
			}
		})
	}
}

func BenchmarkSemanticTokensUnicodeEncoding(b *testing.B) {
	h := newFiveMFixtureHarnessWithoutIndex(b)
	h.writeWorkspaceFile("bench.lua", strings.Repeat("local value = 'é💸'; print(value)\n", 500))
	h.reindex()
	initialize, err := json.Marshal(InitializeParams{Capabilities: &ClientCapabilities{General: &GeneralClientCapabilities{PositionEncodings: []string{"utf-16"}}}})
	if err != nil {
		b.Fatal(err)
	}
	h.server.handleInitialize(Request{RPC: "2.0", ID: 1, Params: initialize})
	h.resetRPC()
	uri := h.server.pathToURI(h.root + "/bench.lua")
	for _, method := range []struct {
		name   string
		ranged bool
	}{{"full", false}, {"range", true}} {
		b.Run(method.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				semanticTokenData(b, h.server, uri, method.ranged)
			}
		})
	}
}

type decodedSemanticToken struct {
	line, column, length, tokenType uint32
}

func decodeSemanticTokenRanges(data []uint32) []decodedSemanticToken {
	var line, column uint32
	result := make([]decodedSemanticToken, 0, len(data)/5)
	for i := 0; i+4 < len(data); i += 5 {
		line += data[i]
		if data[i] == 0 {
			column += data[i+1]
		} else {
			column = data[i+1]
		}
		result = append(result, decodedSemanticToken{line: line, column: column, length: data[i+2], tokenType: data[i+3]})
	}
	return result
}

func containsSemanticToken(tokens []decodedSemanticToken, start Position, tokenType, length uint32) bool {
	for _, token := range tokens {
		if token.line == start.Line && token.column == start.Character && token.tokenType == tokenType && token.length == length {
			return true
		}
	}
	return false
}

func semanticTokenData(t testing.TB, server *Server, uri string, ranged bool) []uint32 {
	t.Helper()
	var out bytes.Buffer
	server.Writer = &out
	var params any = SemanticTokensParams{TextDocument: TextDocumentIdentifier{URI: uri}}
	if ranged {
		params = SemanticTokensRangeParams{TextDocument: TextDocumentIdentifier{URI: uri}, Range: Range{Start: Position{}, End: Position{Line: 1, Character: 1}}}
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if ranged {
		server.handleSemanticTokensRange(Request{RPC: "2.0", ID: 1, Params: encoded})
	} else {
		server.handleSemanticTokensFull(Request{RPC: "2.0", ID: 1, Params: encoded})
	}
	var payload SemanticTokens
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	response, err := ReadMessage(bufio.NewReader(bytes.NewReader(out.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.Result, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Data
}
