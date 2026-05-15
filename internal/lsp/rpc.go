// Package lsp implements a minimal Language Server Protocol server for
// agnostic-ai spec files. Transport is JSON-RPC 2.0 over stdio with the
// standard Content-Length framing used by all LSP clients.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Message is a JSON-RPC 2.0 envelope. Requests carry ID+Method+Params;
// responses carry ID+Result or ID+Error; notifications carry Method+Params
// only (no ID).
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// ResponseError is the JSON-RPC error object.
type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	errCodeParseError     = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInternalError  = -32603
)

// Reader reads Content-Length-framed JSON-RPC messages from r.
type Reader struct {
	r *bufio.Reader
}

// NewReader wraps r for framed JSON-RPC reading.
func NewReader(r io.Reader) *Reader { return &Reader{r: bufio.NewReader(r)} }

// Read reads one message. Returns io.EOF when the stream closes.
func (rr *Reader) Read() (*Message, error) {
	length := -1
	for {
		line, err := rr.r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("lsp: bad Content-Length %q", v)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("lsp: missing Content-Length header")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(rr.r, body); err != nil {
		return nil, fmt.Errorf("lsp: read body: %w", err)
	}
	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("lsp: parse: %w", err)
	}
	return &msg, nil
}

// Writer writes Content-Length-framed JSON-RPC messages to w.
type Writer struct {
	w io.Writer
}

// NewWriter wraps w for framed JSON-RPC writing.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Send marshals msg and writes it with a Content-Length header.
func (ww *Writer) Send(msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("lsp: marshal: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(ww.w, header); err != nil {
		return err
	}
	_, err = ww.w.Write(body)
	return err
}
