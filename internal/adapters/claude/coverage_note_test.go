package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

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

// Project rejections belong in settings, not in each .mcp.json server.
func TestEmit_MCP_DisabledUsesProjectRejection(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	fs := parsed["mcpServers"].(map[string]any)["fs"].(map[string]any)
	if _, ok := fs["disabled"]; ok {
		t.Errorf("claude has no file-based disable key; must not emit one: %s", raw)
	}

	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "`disabled`") {
		t.Errorf("unexpected field no-op note: %s", buf.String())
	}
	raw, err = os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"disabledMcpjsonServers"`) || !strings.Contains(string(raw), `"fs"`) {
		t.Errorf("missing rejection: %s", raw)
	}
}
