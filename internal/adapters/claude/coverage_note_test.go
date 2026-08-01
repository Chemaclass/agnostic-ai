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

// Regression for the target-audit finding: Claude Code has no per-server
// `disabled` key inside `.mcp.json` (code.claude.com/docs/en/mcp documents
// only the `/mcp` panel toggle and the `disabledMcpServers` /
// `enabledMcpServers` settings keys). Writing `disabled: true` into
// `.mcp.json` would let a user believe the server stopped connecting
// when Claude Code ignores the key and keeps using it, so the field must
// not reach the file, and the drop must be loud rather than silent.
func TestEmit_MCP_DisabledHasNoFileBasedEffect(t *testing.T) {
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
	if !strings.Contains(buf.String(), "`disabled` on 1 mcp has no effect on claude") {
		t.Errorf("expected a field no-op note, got: %s", buf.String())
	}
}
