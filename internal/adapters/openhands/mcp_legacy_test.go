package openhands

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// mcpSpecimens covers the cases that used to raise their own config.toml
// notes: a remote `headers` map, an sse `timeout`, and a `ws` transport.
func mcpSpecimens() []spec.Entry {
	return []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Path: "mcps/fs.yaml", Meta: map[string]any{"command": "npx"}},
		{Kind: spec.KindMCP, Name: "docs", Path: "mcps/docs.yaml", Meta: map[string]any{
			"type": "sse", "url": "https://docs.example.test/sse", "timeout": 30,
		}},
		{Kind: spec.KindMCP, Name: "search", Path: "mcps/search.yaml", Meta: map[string]any{
			"type": "http", "url": "https://search.example.test/mcp", "headers": map[string]any{"Authorization": "Bearer x"},
		}},
		{Kind: spec.KindMCP, Name: "live", Path: "mcps/live.yaml", Meta: map[string]any{"type": "ws", "url": "wss://live.example.test"}},
	}
}

// Current OpenHands releases read MCP servers only from
// ~/.openhands/mcp.json, never a project config.toml (#1252), so project
// sync writes nothing for them, wherever outputs.openhands.mcp-file
// points (#1259).
func TestEmit_MCPWritesNoProjectFile(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"default":  {},
		"mcp-file": {Outputs: map[string]config.Output{target: {MCPFile: "vendor/openhands.toml"}}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			if err := New().Emit(emit.NewSession(), spec.NewBundle(mcpSpecimens()), cfg, false); err != nil {
				t.Fatal(err)
			}
			if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
				t.Errorf("MCP specs wrote project files: %v", paths)
			}
		})
	}
}

// The servers still reach OpenHands through sync --global, so one note
// says where they went, and no unsupported warning claims they are lost.
func TestEmit_MCPNotesGlobalSyncOnce(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	emit.ResetCapabilityWarnings()
	t.Cleanup(emit.ResetCoverageNotes)
	t.Cleanup(emit.ResetCapabilityWarnings)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	if err := New().Emit(emit.NewSession(), spec.NewBundle(mcpSpecimens()), &config.Config{OnUnsupported: emit.OnUnsupportedError}, false); err != nil {
		t.Fatal(err)
	}
	if got := emit.PendingCapabilityWarningsCount(); got != 0 {
		t.Errorf("MCP specs raised %d unsupported warnings", got)
	}
	emit.FlushCoverageNotes()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want one note, got %d:\n%s", len(lines), buf.String())
	}
	for _, want := range []string{"4 mcps reach openhands", "~/.openhands/mcp.json", "~/.agnostic-ai/mcps/", "agnostic-ai sync --global"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("note lacks %q: %s", want, lines[0])
		}
	}
}
