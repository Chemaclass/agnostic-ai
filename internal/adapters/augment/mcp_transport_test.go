package augment

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

// docs.augmentcode.com/cli/integrations closes the transport set at
// "-t, --transport <transport> - stdio|sse|http (default: "stdio")",
// and no Augment page names a WebSocket transport. The shared builder
// writes `{"type": "ws", "url": ...}` on Claude Code's authority, so a
// ws spec used to produce an Augment entry that looks configured and
// cannot connect (#855).
func TestEmit_MCP_DropsWebSocketTransport(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "socket", Meta: map[string]any{"type": "ws", "url": "wss://example.test/mcp"}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "http", "url": "https://example.test/mcp"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".augment/settings.json"))
	if strings.Contains(got, "socket") || strings.Contains(got, "wss://") {
		t.Errorf("ws entry must not reach .augment/settings.json:\n%s", got)
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
	dir := testutil.TempCwd(t)
	swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "socket", Meta: map[string]any{"type": "ws", "url": "wss://example.test/mcp"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".augment/settings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no settings file for a ws-only bundle, err=%v", err)
	}
}

func swapNoteWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}
