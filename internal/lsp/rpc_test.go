package lsp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRoundTrip_SimpleMessage(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"}
	if err := w.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	r := NewReader(&buf)
	got, err := r.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Method != "initialize" {
		t.Errorf("method = %q, want %q", got.Method, "initialize")
	}
}

func TestReader_MultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	for i := range 3 {
		_ = w.Send(map[string]any{"jsonrpc": "2.0", "id": i, "method": "ping"})
	}
	r := NewReader(&buf)
	for i := range 3 {
		msg, err := r.Read()
		if err != nil {
			t.Fatalf("msg %d: %v", i, err)
		}
		if msg.Method != "ping" {
			t.Errorf("msg %d method = %q", i, msg.Method)
		}
	}
}

func TestServer_Initialize(t *testing.T) {
	var in, out bytes.Buffer
	w := NewWriter(&in)
	_ = w.Send(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"rootUri": "file:///tmp/proj"},
	})
	_ = w.Send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"})
	// no exit; Run terminates via io.EOF after buffer is drained

	srv := New(&in, &out, nil)
	_ = srv.Run()

	r := NewReader(strings.NewReader(out.String()))
	resp, err := r.Read()
	if err != nil {
		t.Fatalf("read initialize response: %v", err)
	}
	raw, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(raw), "textDocumentSync") {
		t.Errorf("initialize result missing capabilities: %s", raw)
	}
}

func TestURIConversion(t *testing.T) {
	cases := []struct{ uri, path string }{
		{"file:///tmp/foo.md", "/tmp/foo.md"},
		{"file:///home/user/proj/rules/x.md", "/home/user/proj/rules/x.md"},
	}
	for _, c := range cases {
		got := uriToPath(c.uri)
		if got != c.path {
			t.Errorf("uriToPath(%q) = %q, want %q", c.uri, got, c.path)
		}
	}
}
