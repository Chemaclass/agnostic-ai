package qoder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// docs.qoder.com/cli/mcp-reference's "ws Type (TCP)" table has two
// rows, `tcp` ("TCP connection parameters (host/port)") and `type`.
// `url` belongs to the `sse` and `http` tables only, so the entry this
// adapter used to write named the one transport whose documented
// parameter it never supplied (#855).
func TestEmit_MCP_DropsWebSocketTransport(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "socket", Meta: map[string]any{"type": "ws", "url": "wss://example.test/mcp"}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "http", "url": "https://example.test/mcp"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".qoder/settings.json"))
	if strings.Contains(got, "socket") || strings.Contains(got, "wss://") {
		t.Errorf("ws entry must not reach .qoder/settings.json:\n%s", got)
	}
	if !strings.Contains(got, "https://example.test/mcp") {
		t.Errorf("documented transports must still emit:\n%s", got)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "WebSocket transport is not supported") {
		t.Errorf("expected a coverage note naming the dropped transport, got: %s", buf.String())
	}
}

// A ws-only bundle writes no settings file at all, rather than one
// holding an empty `mcpServers` map.
func TestEmit_MCP_WebSocketOnlyWritesNoSettingsFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "socket", Meta: map[string]any{"type": "ws", "url": "wss://example.test/mcp"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".qoder/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no settings file for a ws-only bundle, err=%v", err)
	}
}
